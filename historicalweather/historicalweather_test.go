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
			"time":                 []string{"2024-01-01", "2024-01-02"},
			"temperature_2m_max":   []float64{7.3, 7.9},
			"temperature_2m_min":   []float64{3.2, 4.1},
			"precipitation_sum":    []float64{1.9, 10.2},
			"windspeed_10m_max":    []float64{12.3, 18.7},
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
	if r.Elevation != 38.0 {
		t.Errorf("Elevation = %g, want 38.0", r.Elevation)
	}
}

func TestHourlyHistory(t *testing.T) {
	payload := map[string]any{
		"latitude":  52.54833,
		"longitude": 13.407822,
		"timezone":  "Europe/Berlin",
		"elevation": 38.0,
		"hourly": map[string]any{
			"time":             []string{"2024-01-01T00:00", "2024-01-01T01:00"},
			"temperature_2m":   []float64{4.5, 4.2},
			"precipitation":    []float64{0.1, 0.0},
			"windspeed_10m":    []float64{8.2, 7.9},
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

	records, err := c.HourlyHistory(context.Background(), 52.52, 13.41, "2024-01-01", "2024-01-01")
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
}

func TestSafeGet(t *testing.T) {
	// tested indirectly via DailyHistory with short slices — just ensure no panic
	payload := map[string]any{
		"latitude":  1.0,
		"longitude": 1.0,
		"timezone":  "UTC",
		"elevation": 0.0,
		"daily": map[string]any{
			"time":               []string{"2024-01-01"},
			"temperature_2m_max": []float64{}, // intentionally shorter
			"temperature_2m_min": []float64{},
			"precipitation_sum":  []float64{},
			"windspeed_10m_max":  []float64{},
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
		t.Errorf("safeGet on empty slice should return 0.0, got %g", records[0].TempMax)
	}
}
