package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func parseFixture(t *testing.T) *Forecast {
	t.Helper()
	f, err := Parse(loadFixture(t, "cordoba_forecast.json"))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestParseRealResponse(t *testing.T) {
	f := parseFixture(t)
	if f.Current.Temp != 29.8 || f.Current.FeelsLike != 32.7 || f.Current.Humidity != 36 || f.Current.Code != 0 || !f.Current.IsDay {
		t.Fatalf("current = %+v", f.Current)
	}
	if got := f.Current.Time.Format(time.RFC3339); got != "2026-11-11T15:00:00-03:00" {
		t.Fatalf("current time = %s", got)
	}
	if len(f.Hours) != 168 || len(f.Days) != 7 {
		t.Fatalf("hours=%d days=%d", len(f.Hours), len(f.Days))
	}
	if d := f.Days[2]; d.Max != 35.3 || d.Min != 21.3 || d.RainProb != 75 || d.Code != 96 {
		t.Fatalf("day 2 = %+v", d)
	}
	if f.Fahrenheit {
		t.Fatal("units should be Celsius")
	}
	if f.Condition() != Clear {
		t.Fatalf("condition = %v", f.Condition())
	}
}

func TestHoursFromAndDaysFrom(t *testing.T) {
	f := parseFixture(t)
	// 15:40 local time belongs to the 15:00 slot.
	now := time.Date(2026, 11, 11, 18, 40, 0, 0, time.UTC)
	hours := f.HoursFrom(now, 4)
	if len(hours) != 4 || hours[0].Time.Hour() != 15 || hours[0].Temp != 29.8 || hours[3].Time.Hour() != 18 {
		t.Fatalf("hours = %+v", hours)
	}
	// Near the end the slice is shorter instead of panicking.
	if late := f.HoursFrom(time.Date(2026, 11, 18, 1, 30, 0, 0, time.UTC), 5); len(late) != 2 {
		t.Fatalf("late hours len = %d", len(late))
	}
	days := f.DaysFrom(now, 3)
	if len(days) != 3 || days[0].Date.Day() != 11 || days[2].Date.Day() != 13 {
		t.Fatalf("days = %+v", days)
	}
	today, ok := f.Today(now)
	if !ok || today.Max != 30.0 || today.Min != 14.7 {
		t.Fatalf("today = %+v ok=%v", today, ok)
	}
	// 23:30 local on the 11th is still the 11th in Córdoba even though it is the 12th in UTC.
	if d, _ := f.Today(time.Date(2026, 11, 12, 2, 30, 0, 0, time.UTC)); d.Date.Day() != 11 {
		t.Fatalf("today across UTC midnight = %v", d.Date)
	}
	if _, ok := f.Today(time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)); ok {
		t.Fatal("today must be missing past the forecast range")
	}
}

func TestParseToleratesNulls(t *testing.T) {
	data := `{"utc_offset_seconds":0,"timezone_abbreviation":"GMT","current_units":{"temperature_2m":"°F"},
	"current":{"time":"2026-11-11T15:00","temperature_2m":70.1,"apparent_temperature":null,"weather_code":null},
	"hourly":{"time":["2026-11-11T15:00","2026-11-11T16:00"],"temperature_2m":[70.1,null],"precipitation_probability":[null,10]},
	"daily":{"time":["2026-11-11"],"temperature_2m_max":[null],"temperature_2m_min":[50]}}`
	f, err := Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if !f.Fahrenheit || f.Current.FeelsLike != 70.1 || f.Current.Code != -1 {
		t.Fatalf("current = %+v", f.Current)
	}
	if len(f.Hours) != 1 || f.Hours[0].RainProb != -1 {
		t.Fatalf("hours = %+v", f.Hours)
	}
	if len(f.Days) != 0 {
		t.Fatalf("days with null max should be skipped: %+v", f.Days)
	}
}

func TestParseErrors(t *testing.T) {
	for name, data := range map[string]string{
		"invalid json": `{`,
		"api error":    `{"error":true,"reason":"Latitude must be in range"}`,
		"no current":   `{"hourly":{}}`,
	} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestConditionFromCode(t *testing.T) {
	cases := map[int]Condition{0: Clear, 2: PartlyCloudy, 3: Cloudy, 45: Fog, 53: Drizzle, 63: Rain, 81: Rain, 75: Snow, 86: Snow, 95: Thunderstorm, 99: Thunderstorm, -1: Unknown, 42: Unknown}
	for code, want := range cases {
		if got := ConditionFromCode(code); got != want {
			t.Errorf("code %d = %v, want %v", code, got, want)
		}
	}
}

func TestParsePlaces(t *testing.T) {
	places, err := ParsePlaces(loadFixture(t, "cordoba_search.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(places) != 3 {
		t.Fatalf("len = %d", len(places))
	}
	if got := places[0].Label(); got != "Córdoba, Provincia de Córdoba, Argentina" {
		t.Fatalf("label = %q", got)
	}
	if got := places[1].ShortLabel(); got != "Córdoba, España" {
		t.Fatalf("short label = %q", got)
	}
	if places[0].Latitude != -31.40648 || places[0].Longitude != -64.18853 {
		t.Fatalf("coordinates = %+v", places[0])
	}
	empty, err := ParsePlaces(loadFixture(t, "empty_search.json"))
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty = %v, %v", empty, err)
	}
}

func TestClientRequestsAndErrors(t *testing.T) {
	forecast := loadFixture(t, "cordoba_forecast.json")
	search := loadFixture(t, "cordoba_search.json")
	var lastQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastQuery = r.URL.Query()
		switch r.URL.Path {
		case "/forecast":
			w.Write(forecast)
		case "/search":
			w.Write(search)
		default:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":true,"reason":"Cannot initialize WeatherVariable from invalid String value"}`))
		}
	}))
	defer srv.Close()

	c := &Client{ForecastURL: srv.URL + "/forecast", GeocodeURL: srv.URL + "/search"}
	f, err := c.Fetch(context.Background(), Query{Latitude: -31.4065, Longitude: -64.1885, Fahrenheit: true})
	if err != nil || f.Current.Temp != 29.8 {
		t.Fatalf("fetch = %v, %v", f, err)
	}
	if lastQuery.Get("latitude") != "-31.4065" || lastQuery.Get("temperature_unit") != "fahrenheit" || lastQuery.Get("timezone") != "auto" {
		t.Fatalf("forecast query = %v", lastQuery)
	}
	places, err := c.Search(context.Background(), "Córdoba", "es")
	if err != nil || len(places) != 3 || lastQuery.Get("name") != "Córdoba" || lastQuery.Get("language") != "es" {
		t.Fatalf("search = %v, %v, query %v", places, err, lastQuery)
	}

	bad := &Client{ForecastURL: srv.URL + "/bad"}
	if _, err := bad.Fetch(context.Background(), Query{}); err == nil || !strings.Contains(err.Error(), "HTTP 400: Cannot initialize") {
		t.Fatalf("error = %v", err)
	}
}
