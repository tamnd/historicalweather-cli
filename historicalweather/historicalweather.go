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
	Date          string  `kit:"id" json:"date"`
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	Timezone      string  `json:"timezone"`
	TempMax       float64 `json:"temp_max_c"`
	TempMin       float64 `json:"temp_min_c"`
	Precipitation float64 `json:"precipitation_mm"`
	WindspeedMax  float64 `json:"windspeed_max_kmh"`
	Elevation     float64 `json:"elevation_m"`
}

// HourlyRecord holds one hour of historical weather observations.
type HourlyRecord struct {
	Time          string  `kit:"id" json:"time"`
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	Timezone      string  `json:"timezone"`
	Temperature   float64 `json:"temp_c"`
	Precipitation float64 `json:"precipitation_mm"`
	Windspeed     float64 `json:"windspeed_kmh"`
}

// wireResponse is the raw JSON shape returned by the archive API.
type wireResponse struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone"`
	Elevation float64 `json:"elevation"`
	Daily     *struct {
		Time    []string  `json:"time"`
		TempMax []float64 `json:"temperature_2m_max"`
		TempMin []float64 `json:"temperature_2m_min"`
		Precip  []float64 `json:"precipitation_sum"`
		WindMax []float64 `json:"windspeed_10m_max"`
	} `json:"daily"`
	Hourly *struct {
		Time   []string  `json:"time"`
		Temp   []float64 `json:"temperature_2m"`
		Precip []float64 `json:"precipitation"`
		Wind   []float64 `json:"windspeed_10m"`
	} `json:"hourly"`
}

// safeGet returns slice[i] or 0.0 if i is out of bounds.
func safeGet(slice []float64, i int) float64 {
	if i < 0 || i >= len(slice) {
		return 0.0
	}
	return slice[i]
}

// DailyHistory fetches daily historical weather records for the given location
// and date range (YYYY-MM-DD). It returns one DailyRecord per day.
func (c *Client) DailyHistory(ctx context.Context, lat, lon float64, start, end string) ([]DailyRecord, error) {
	url := fmt.Sprintf(
		"%s/v1/archive?latitude=%g&longitude=%g&start_date=%s&end_date=%s&daily=temperature_2m_max,temperature_2m_min,precipitation_sum,windspeed_10m_max&timezone=auto",
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
	if wr.Daily == nil {
		return nil, fmt.Errorf("no daily data in response")
	}
	out := make([]DailyRecord, len(wr.Daily.Time))
	for i, t := range wr.Daily.Time {
		out[i] = DailyRecord{
			Date:          t,
			Latitude:      wr.Latitude,
			Longitude:     wr.Longitude,
			Timezone:      wr.Timezone,
			Elevation:     wr.Elevation,
			TempMax:       safeGet(wr.Daily.TempMax, i),
			TempMin:       safeGet(wr.Daily.TempMin, i),
			Precipitation: safeGet(wr.Daily.Precip, i),
			WindspeedMax:  safeGet(wr.Daily.WindMax, i),
		}
	}
	return out, nil
}

// HourlyHistory fetches hourly historical weather records for the given
// location and date range (YYYY-MM-DD). It returns one HourlyRecord per hour.
func (c *Client) HourlyHistory(ctx context.Context, lat, lon float64, start, end string) ([]HourlyRecord, error) {
	url := fmt.Sprintf(
		"%s/v1/archive?latitude=%g&longitude=%g&start_date=%s&end_date=%s&hourly=temperature_2m,precipitation,windspeed_10m&timezone=auto",
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
	if wr.Hourly == nil {
		return nil, fmt.Errorf("no hourly data in response")
	}
	out := make([]HourlyRecord, len(wr.Hourly.Time))
	for i, t := range wr.Hourly.Time {
		out[i] = HourlyRecord{
			Time:          t,
			Latitude:      wr.Latitude,
			Longitude:     wr.Longitude,
			Timezone:      wr.Timezone,
			Temperature:   safeGet(wr.Hourly.Temp, i),
			Precipitation: safeGet(wr.Hourly.Precip, i),
			Windspeed:     safeGet(wr.Hourly.Wind, i),
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
