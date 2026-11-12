// Package plugin holds the per-key state of the weather plugin: settings, forecast cache and views.
package plugin

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Marco-caizzi/redragon-ss552-weather/internal/weather"
)

const (
	defaultRefresh = 15 * time.Minute
	minRefresh     = 5 * time.Minute
	maxRefresh     = 3 * time.Hour
)

// Settings is the per-key configuration saved by the property inspector.
type Settings struct {
	City       string
	Latitude   float64
	Longitude  float64
	Fahrenheit bool
	Refresh    time.Duration
	Configured bool
}

// Query returns the forecast request for these settings.
func (s Settings) Query() weather.Query {
	// Rounding keeps keys configured for the same city sharing one cached forecast.
	round := func(v float64) float64 { return math.Round(v*1e4) / 1e4 }
	return weather.Query{Latitude: round(s.Latitude), Longitude: round(s.Longitude), Fahrenheit: s.Fahrenheit}
}

// ParseSettings reads settings JSON written by the property inspector.
// Numbers may arrive as JSON numbers or strings; anything invalid falls back to defaults.
func ParseSettings(raw json.RawMessage) Settings {
	s := Settings{Refresh: defaultRefresh}
	var m map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return s
	}

	s.City = strings.TrimSpace(asString(m["city"]))
	lat, latOK := asFloat(m["latitude"])
	lon, lonOK := asFloat(m["longitude"])
	s.Fahrenheit = strings.EqualFold(asString(m["units"]), "f")
	if minutes, ok := asFloat(m["refresh"]); ok {
		d := time.Duration(minutes * float64(time.Minute))
		s.Refresh = max(minRefresh, min(d, maxRefresh))
	}

	validCoords := latOK && lonOK && math.Abs(lat) <= 90 && math.Abs(lon) <= 180 && !(lat == 0 && lon == 0)
	if validCoords {
		s.Latitude, s.Longitude = lat, lon
		s.Configured = true
		if s.City == "" {
			s.City = strconv.FormatFloat(lat, 'f', 2, 64) + ", " + strconv.FormatFloat(lon, 'f', 2, 64)
		}
	}
	return s
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, !math.IsNaN(t) && !math.IsInf(t, 0)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(strings.Replace(t, ",", ".", 1)), 64)
		return f, err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
	default:
		return 0, false
	}
}
