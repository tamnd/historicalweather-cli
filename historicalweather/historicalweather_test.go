package historicalweather_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamnd/historicalweather-cli/historicalweather"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := historicalweather.NewClient()
	c.Rate = 0 // no pacing in the test

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := historicalweather.NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestDailyHistory(t *testing.T) {
	payload := map[string]any{
		"latitude":  52.54833,
		"longitude": 13.407822,
		"timezone":  "Europe/Berlin",
		"elevation": 38.0,
		"daily": map[string]any{
			"time":                  []string{"2024-01-01", "2024-01-02"},
			"temperature_2m_max":    []float64{7.3, 7.9},
			"temperature_2m_min":    []float64{3.2, 4.1},
			"temperature_2m_mean":   []float64{5.2, 6.0},
			"precipitation_sum":     []float64{1.9, 10.2},
			"wind_speed_10m_max":    []float64{12.3, 18.7},
			"weathercode":           []int{3, 61},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := historicalweather.NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL

	records, err := c.DailyHistory(context.Background(), 52.52, 13.41, "2024-01-01", "2024-01-02")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	r := records[0]
	if r.Date != "2024-01-01" {
		t.Errorf("Date = %q, want 2024-01-01", r.Date)
	}
	if r.TempMax != 7.3 {
		t.Errorf("TempMax = %g, want 7.3", r.TempMax)
	}
	if r.TempMin != 3.2 {
		t.Errorf("TempMin = %g, want 3.2", r.TempMin)
	}
	if r.TempMean != 5.2 {
		t.Errorf("TempMean = %g, want 5.2", r.TempMean)
	}
	if r.WindSpeedMax != 12.3 {
		t.Errorf("WindSpeedMax = %g, want 12.3", r.WindSpeedMax)
	}
	if r.WeatherCode != 3 {
		t.Errorf("WeatherCode = %d, want 3", r.WeatherCode)
	}
}

func TestHourlyHistory(t *testing.T) {
	payload := map[string]any{
		"latitude":  52.54833,
		"longitude": 13.407822,
		"timezone":  "Europe/Berlin",
		"elevation": 38.0,
		"hourly": map[string]any{
			"time":                   []string{"2024-01-01T00:00", "2024-01-01T01:00"},
			"temperature_2m":         []float64{4.5, 4.2},
			"precipitation":          []float64{0.1, 0.0},
			"wind_speed_10m":         []float64{8.2, 7.9},
			"relative_humidity_2m":   []float64{85.0, 82.0},
			"weathercode":            []int{2, 1},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := historicalweather.NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL

	records, err := c.HourlyHistory(context.Background(), 52.52, 13.41, "2024-01-01", "2024-01-01", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	r := records[0]
	if r.Time != "2024-01-01T00:00" {
		t.Errorf("Time = %q, want 2024-01-01T00:00", r.Time)
	}
	if r.Temperature != 4.5 {
		t.Errorf("Temperature = %g, want 4.5", r.Temperature)
	}
	if r.WindSpeed != 8.2 {
		t.Errorf("WindSpeed = %g, want 8.2", r.WindSpeed)
	}
	if r.Humidity != 85.0 {
		t.Errorf("Humidity = %g, want 85.0", r.Humidity)
	}
	if r.WeatherCode != 2 {
		t.Errorf("WeatherCode = %d, want 2", r.WeatherCode)
	}
}

func TestHourlyHistoryLimit(t *testing.T) {
	times := make([]string, 24)
	temps := make([]float64, 24)
	for i := range times {
		times[i] = "2024-01-01T00:00"
		temps[i] = float64(i)
	}
	payload := map[string]any{
		"latitude":  1.0,
		"longitude": 1.0,
		"timezone":  "UTC",
		"hourly": map[string]any{
			"time":                 times,
			"temperature_2m":       temps,
			"precipitation":        temps,
			"wind_speed_10m":       temps,
			"relative_humidity_2m": temps,
			"weathercode":          make([]int, 24),
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := historicalweather.NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL

	records, err := c.HourlyHistory(context.Background(), 1.0, 1.0, "2024-01-01", "2024-01-01", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 5 {
		t.Fatalf("got %d records with limit 5, want 5", len(records))
	}
}

func TestSafeF(t *testing.T) {
	// tested indirectly via DailyHistory with short slices - ensure no panic
	payload := map[string]any{
		"latitude":  1.0,
		"longitude": 1.0,
		"timezone":  "UTC",
		"elevation": 0.0,
		"daily": map[string]any{
			"time":                  []string{"2024-01-01"},
			"temperature_2m_max":    []float64{}, // intentionally shorter
			"temperature_2m_min":    []float64{},
			"temperature_2m_mean":   []float64{},
			"precipitation_sum":     []float64{},
			"wind_speed_10m_max":    []float64{},
			"weathercode":           []int{},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := historicalweather.NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL

	records, err := c.DailyHistory(context.Background(), 1.0, 1.0, "2024-01-01", "2024-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].TempMax != 0.0 {
		t.Errorf("safeF on empty slice should return 0.0, got %g", records[0].TempMax)
	}
	if records[0].WeatherCode != 0 {
		t.Errorf("safeI on empty slice should return 0, got %d", records[0].WeatherCode)
	}
}
