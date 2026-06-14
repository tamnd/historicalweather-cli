package historicalweather

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring, which need no network. The client's HTTP behaviour is
// covered in historicalweather_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "historicalweather" {
		t.Errorf("Scheme = %q, want historicalweather", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "historicalweather" {
		t.Errorf("Identity.Binary = %q, want historicalweather", info.Identity.Binary)
	}
}

func TestClassifyLatLon(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
	}{
		{"52.52,13.41", "location", "52.52,13.41"},
		{"-33.87,151.21", "location", "-33.87,151.21"},
		{"0,0", "location", "0,0"},
		{"48.8,2.3", "location", "48.8,2.3"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestClassifyCity(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
	}{
		{"london", "city", "london"},
		{"Berlin", "city", "Berlin"},
		{"New York", "city", "New York"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocateLocation(t *testing.T) {
	got, err := Domain{}.Locate("location", "48.8,2.3")
	if err != nil {
		t.Fatalf("Locate error: %v", err)
	}
	want := "https://archive-api.open-meteo.com/v1/archive?latitude=48.8&longitude=2.3"
	if got != want {
		t.Errorf("Locate = %q, want %q", got, want)
	}
}

func TestLocateCity(t *testing.T) {
	got, err := Domain{}.Locate("city", "london")
	if err != nil {
		t.Fatalf("Locate error: %v", err)
	}
	want := "https://archive-api.open-meteo.com/v1/archive?q=london"
	if got != want {
		t.Errorf("Locate = %q, want %q", got, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "foo")
	if err == nil {
		t.Error("expected error for unknown type, got nil")
	}
}

func TestIsLatLon(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"52.52,13.41", true},
		{"-90,180", true},
		{"0,0", true},
		{"notfloat,13.41", false},
		{"52.52", false},
		{"Berlin", false},
		{"", false},
	}
	for _, tc := range cases {
		got := isLatLon(tc.s)
		if got != tc.want {
			t.Errorf("isLatLon(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

// TestHostWiring mounts the driver in a kit Host and verifies the domain is
// registered and its ops are available.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}
	info, ok := h.Domain("historicalweather")
	if !ok {
		t.Fatal("historicalweather domain not mounted on host")
	}
	if info.Scheme != "historicalweather" {
		t.Errorf("scheme = %q, want historicalweather", info.Scheme)
	}
}
