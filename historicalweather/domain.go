package historicalweather

import (
	"context"
	"fmt"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes historicalweather as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/historicalweather-cli/historicalweather"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// historicalweather:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone historicalweather binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the historicalweather driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "historicalweather",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "historicalweather",
			Short:  "Historical weather data via Open Meteo Archive API.",
			Long: `historicalweather reads public historical weather data from the Open Meteo
Archive API (archive-api.open-meteo.com), shapes it into clean records, and
prints output that pipes into the rest of your tools. No API key required.`,
			Site: Host,
			Repo: "https://github.com/tamnd/historicalweather-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// daily: get daily historical weather records
	kit.Handle(app, kit.OpMeta{
		Name:    "daily",
		Group:   "read",
		List:    true,
		Summary: "Get daily historical weather records for a location and date range",
		Args: []kit.Arg{
			{Name: "lat", Help: "latitude"},
			{Name: "lon", Help: "longitude"},
			{Name: "start", Help: "start date YYYY-MM-DD"},
			{Name: "end", Help: "end date YYYY-MM-DD"},
		},
	}, dailyOp)

	// hourly: get hourly historical weather records
	kit.Handle(app, kit.OpMeta{
		Name:    "hourly",
		Group:   "read",
		List:    true,
		Summary: "Get hourly historical weather records for a location and date range",
		Args: []kit.Arg{
			{Name: "lat", Help: "latitude"},
			{Name: "lon", Help: "longitude"},
			{Name: "start", Help: "start date YYYY-MM-DD"},
			{Name: "end", Help: "end date YYYY-MM-DD"},
		},
	}, hourlyOp)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type dailyInput struct {
	Lat    float64 `kit:"arg" help:"latitude"`
	Lon    float64 `kit:"arg" help:"longitude"`
	Start  string  `kit:"arg" help:"start date YYYY-MM-DD"`
	End    string  `kit:"arg" help:"end date YYYY-MM-DD"`
	Client *Client `kit:"inject"`
}

type hourlyInput struct {
	Lat    float64 `kit:"arg" help:"latitude"`
	Lon    float64 `kit:"arg" help:"longitude"`
	Start  string  `kit:"arg" help:"start date YYYY-MM-DD"`
	End    string  `kit:"arg" help:"end date YYYY-MM-DD"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func dailyOp(ctx context.Context, in dailyInput, emit func(*DailyRecord) error) error {
	records, err := in.Client.DailyHistory(ctx, in.Lat, in.Lon, in.Start, in.End)
	if err != nil {
		return mapErr(err)
	}
	for i := range records {
		if err := emit(&records[i]); err != nil {
			return err
		}
	}
	return nil
}

func hourlyOp(ctx context.Context, in hourlyInput, emit func(*HourlyRecord) error) error {
	records, err := in.Client.HourlyHistory(ctx, in.Lat, in.Lon, in.Start, in.End)
	if err != nil {
		return mapErr(err)
	}
	for i := range records {
		if err := emit(&records[i]); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: URI-native string functions, pure and network-free ---

// Classify turns any accepted input into the canonical (type, id).
// Recognizes "lat,lon" patterns as "location" resources; everything else is "query".
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty historicalweather reference")
	}
	// "lat,lon" pattern: two floats comma-separated
	if isLatLon(input) {
		return "location", input, nil
	}
	return "query", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "location":
		parts := strings.SplitN(id, ",", 2)
		if len(parts) != 2 {
			return "", errs.Usage("location id must be lat,lon: %q", id)
		}
		return fmt.Sprintf(
			"https://%s/v1/archive?latitude=%s&longitude=%s&start_date=2024-01-01&end_date=2024-01-07&daily=temperature_2m_max",
			Host, strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]),
		), nil
	case "query":
		return fmt.Sprintf("https://%s/v1/archive?%s", Host, id), nil
	default:
		return "", errs.Usage("historicalweather has no resource type %q", uriType)
	}
}

// --- helpers ---

// isLatLon checks if the input looks like "float,float".
func isLatLon(s string) bool {
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return false
	}
	var a, b float64
	_, errA := fmt.Sscanf(strings.TrimSpace(parts[0]), "%g", &a)
	_, errB := fmt.Sscanf(strings.TrimSpace(parts[1]), "%g", &b)
	return errA == nil && errB == nil
}

// mapErr converts a library error into the kit error kind that carries the
// right exit code.
func mapErr(err error) error {
	return err
}
