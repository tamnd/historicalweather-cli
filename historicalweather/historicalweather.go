// Package historicalweather is the library behind the historicalweather command line:
// the HTTP client, request shaping, and the typed data models for historical
// weather data via the Open Meteo Archive API.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
package historicalweather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultUserAgent identifies the client to Open Meteo.
const DefaultUserAgent = "historicalweather/dev (+https://github.com/tamnd/historicalweather-cli)"

// Host is the API host this client talks to.
const Host = "archive-api.open-meteo.com"

// Config holds the tunable knobs for a Client.
type Config struct {
	BaseURL string
	Rate    time.Duration
	Retries int
	Timeout time.Duration
}

// DefaultConfig returns the recommended defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL: "https://archive-api.open-meteo.com",
		Rate:    200 * time.Millisecond,
		Retries: 3,
		Timeout: 30 * time.Second,
	}
}

// Client talks to the Open Meteo Archive API over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	BaseURL   string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: DefaultUserAgent,
		BaseURL:   cfg.BaseURL,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

// DailyRecord holds one day of historical weather observations.
type DailyRecord struct {
	Date          string  `json:"date" kit:"id"`
	TempMax       float64 `json:"temp_max_c"`
	TempMin       float64 `json:"temp_min_c"`
	TempMean      float64 `json:"temp_mean_c"`
	Precipitation float64 `json:"precipitation_mm"`
	WindSpeedMax  float64 `json:"wind_speed_max_kmh"`
	WeatherCode   int     `json:"weather_code"`
}

// HourlyRecord holds one hour of historical weather observations.
type HourlyRecord struct {
	Time          string  `json:"time" kit:"id"`
	Temperature   float64 `json:"temperature_c"`
	Precipitation float64 `json:"precipitation_mm"`
	WindSpeed     float64 `json:"wind_speed_kmh"`
	Humidity      float64 `json:"humidity_pct"`
	WeatherCode   int     `json:"weather_code"`
}

// wireResponse is the raw JSON shape returned by the archive API.
type wireResponse struct {
	Daily  *wireDailyData  `json:"daily"`
	Hourly *wireHourlyData `json:"hourly"`
	Error  string          `json:"reason"`
}

type wireDailyData struct {
	Time          []string  `json:"time"`
	TempMax       []float64 `json:"temperature_2m_max"`
	TempMin       []float64 `json:"temperature_2m_min"`
	TempMean      []float64 `json:"temperature_2m_mean"`
	Precipitation []float64 `json:"precipitation_sum"`
	WindSpeedMax  []float64 `json:"wind_speed_10m_max"`
	WeatherCode   []int     `json:"weathercode"`
}

type wireHourlyData struct {
	Time          []string  `json:"time"`
	Temperature   []float64 `json:"temperature_2m"`
	Precipitation []float64 `json:"precipitation"`
	WindSpeed     []float64 `json:"wind_speed_10m"`
	Humidity      []float64 `json:"relative_humidity_2m"`
	WeatherCode   []int     `json:"weathercode"`
}

// safeF returns arr[i] if i is in-bounds, 0 otherwise.
func safeF(arr []float64, i int) float64 {
	if i < 0 || i >= len(arr) {
		return 0.0
	}
	return arr[i]
}

// safeI returns arr[i] if i is in-bounds, 0 otherwise.
func safeI(arr []int, i int) int {
	if i < 0 || i >= len(arr) {
		return 0
	}
	return arr[i]
}

// DailyHistory fetches daily historical weather records for the given location
// and date range (YYYY-MM-DD). It returns one DailyRecord per day.
func (c *Client) DailyHistory(ctx context.Context, lat, lon float64, start, end string) ([]DailyRecord, error) {
	url := fmt.Sprintf(
		"%s/v1/archive?latitude=%g&longitude=%g&start_date=%s&end_date=%s&daily=temperature_2m_max,temperature_2m_min,temperature_2m_mean,precipitation_sum,wind_speed_10m_max,weathercode&timezone=auto",
		c.BaseURL, lat, lon, start, end,
	)
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var wr wireResponse
	if err := json.Unmarshal(body, &wr); err != nil {
		return nil, fmt.Errorf("decode daily response: %w", err)
	}
	if wr.Error != "" {
		return nil, fmt.Errorf("api error: %s", wr.Error)
	}
	if wr.Daily == nil {
		return nil, fmt.Errorf("no daily data in response")
	}
	out := make([]DailyRecord, len(wr.Daily.Time))
	for i, t := range wr.Daily.Time {
		out[i] = DailyRecord{
			Date:          t,
			TempMax:       safeF(wr.Daily.TempMax, i),
			TempMin:       safeF(wr.Daily.TempMin, i),
			TempMean:      safeF(wr.Daily.TempMean, i),
			Precipitation: safeF(wr.Daily.Precipitation, i),
			WindSpeedMax:  safeF(wr.Daily.WindSpeedMax, i),
			WeatherCode:   safeI(wr.Daily.WeatherCode, i),
		}
	}
	return out, nil
}

// HourlyHistory fetches hourly historical weather records for the given
// location and date range (YYYY-MM-DD). It returns one HourlyRecord per hour.
func (c *Client) HourlyHistory(ctx context.Context, lat, lon float64, start, end string, limit int) ([]HourlyRecord, error) {
	url := fmt.Sprintf(
		"%s/v1/archive?latitude=%g&longitude=%g&start_date=%s&end_date=%s&hourly=temperature_2m,precipitation,wind_speed_10m,relative_humidity_2m,weathercode&timezone=auto",
		c.BaseURL, lat, lon, start, end,
	)
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var wr wireResponse
	if err := json.Unmarshal(body, &wr); err != nil {
		return nil, fmt.Errorf("decode hourly response: %w", err)
	}
	if wr.Error != "" {
		return nil, fmt.Errorf("api error: %s", wr.Error)
	}
	if wr.Hourly == nil {
		return nil, fmt.Errorf("no hourly data in response")
	}
	n := len(wr.Hourly.Time)
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]HourlyRecord, n)
	for i := 0; i < n; i++ {
		out[i] = HourlyRecord{
			Time:          wr.Hourly.Time[i],
			Temperature:   safeF(wr.Hourly.Temperature, i),
			Precipitation: safeF(wr.Hourly.Precipitation, i),
			WindSpeed:     safeF(wr.Hourly.WindSpeed, i),
			Humidity:      safeF(wr.Hourly.Humidity, i),
			WeatherCode:   safeI(wr.Hourly.WeatherCode, i),
		}
	}
	return out, nil
}

// Get fetches url and returns the response body. It paces and retries according
// to the client's settings. The caller owns nothing extra; the body is read
// fully and closed here.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func (c *Client) do(ctx context.Context, url string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}
