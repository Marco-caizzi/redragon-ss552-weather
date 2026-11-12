// Package weather fetches and models forecasts from Open-Meteo (https://open-meteo.com).
package weather

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Current is the weather right now.
type Current struct {
	Time      time.Time
	Temp      float64
	FeelsLike float64
	Humidity  int
	WindKmh   float64
	Code      int
	IsDay     bool
}

// Hour is one hourly forecast point. RainProb is -1 when unknown.
type Hour struct {
	Time     time.Time
	Temp     float64
	RainProb int
	Code     int
	IsDay    bool
}

// Day is one daily forecast. RainProb is -1 when unknown.
type Day struct {
	Date     time.Time
	Code     int
	Max      float64
	Min      float64
	RainProb int
}

// Forecast is a parsed Open-Meteo response. Times are in the location's own time zone.
type Forecast struct {
	Current  Current
	Hours    []Hour
	Days     []Day
	Location *time.Location
	// Fahrenheit is true when temperatures are in °F.
	Fahrenheit bool
}

// Condition of the current weather.
func (f *Forecast) Condition() Condition { return ConditionFromCode(f.Current.Code) }

// HoursFrom returns up to n hourly points starting at the hour containing t.
func (f *Forecast) HoursFrom(t time.Time, n int) []Hour {
	start := t.In(f.Location).Truncate(time.Hour)
	for i, h := range f.Hours {
		if !h.Time.Before(start) {
			end := min(i+n, len(f.Hours))
			return f.Hours[i:end]
		}
	}
	return nil
}

// DaysFrom returns up to n days starting at the day containing t.
func (f *Forecast) DaysFrom(t time.Time, n int) []Day {
	local := t.In(f.Location)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, f.Location)
	for i, d := range f.Days {
		if !d.Date.Before(today) {
			end := min(i+n, len(f.Days))
			return f.Days[i:end]
		}
	}
	return nil
}

// Today returns the forecast for the day containing t, if present.
func (f *Forecast) Today(t time.Time) (Day, bool) {
	days := f.DaysFrom(t, 1)
	if len(days) == 0 {
		return Day{}, false
	}
	local := t.In(f.Location)
	if days[0].Date.Year() != local.Year() || days[0].Date.YearDay() != local.YearDay() {
		return Day{}, false
	}
	return days[0], true
}

// Raw response shape. Pointer slices tolerate nulls Open-Meteo returns for missing values.
type apiResponse struct {
	UTCOffsetSeconds int    `json:"utc_offset_seconds"`
	TimezoneAbbr     string `json:"timezone_abbreviation"`
	CurrentUnits     struct {
		Temperature string `json:"temperature_2m"`
	} `json:"current_units"`
	Current struct {
		Time        string   `json:"time"`
		Temperature *float64 `json:"temperature_2m"`
		Apparent    *float64 `json:"apparent_temperature"`
		Humidity    *float64 `json:"relative_humidity_2m"`
		IsDay       *int     `json:"is_day"`
		WeatherCode *int     `json:"weather_code"`
		WindSpeed   *float64 `json:"wind_speed_10m"`
	} `json:"current"`
	Hourly struct {
		Time        []string   `json:"time"`
		Temperature []*float64 `json:"temperature_2m"`
		RainProb    []*float64 `json:"precipitation_probability"`
		WeatherCode []*int     `json:"weather_code"`
		IsDay       []*int     `json:"is_day"`
	} `json:"hourly"`
	Daily struct {
		Time        []string   `json:"time"`
		WeatherCode []*int     `json:"weather_code"`
		Max         []*float64 `json:"temperature_2m_max"`
		Min         []*float64 `json:"temperature_2m_min"`
		RainProb    []*float64 `json:"precipitation_probability_max"`
	} `json:"daily"`
	Error  bool   `json:"error"`
	Reason string `json:"reason"`
}

// Parse decodes an Open-Meteo forecast response.
func Parse(data []byte) (*Forecast, error) {
	var r apiResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("decoding forecast: %w", err)
	}
	if r.Error {
		return nil, fmt.Errorf("open-meteo error: %s", r.Reason)
	}
	if r.Current.Time == "" || r.Current.Temperature == nil {
		return nil, errors.New("forecast has no current weather")
	}

	loc := time.FixedZone(r.TimezoneAbbr, r.UTCOffsetSeconds)
	f := &Forecast{Location: loc, Fahrenheit: r.CurrentUnits.Temperature == "°F"}

	now, err := time.ParseInLocation("2006-01-02T15:04", r.Current.Time, loc)
	if err != nil {
		return nil, fmt.Errorf("current time: %w", err)
	}
	f.Current = Current{
		Time:      now,
		Temp:      *r.Current.Temperature,
		FeelsLike: valueOr(r.Current.Apparent, *r.Current.Temperature),
		Humidity:  int(valueOr(r.Current.Humidity, -1)),
		WindKmh:   valueOr(r.Current.WindSpeed, -1),
		Code:      intOr(r.Current.WeatherCode, -1),
		IsDay:     intOr(r.Current.IsDay, 1) == 1,
	}

	for i, ts := range r.Hourly.Time {
		t, err := time.ParseInLocation("2006-01-02T15:04", ts, loc)
		if err != nil {
			return nil, fmt.Errorf("hourly time %q: %w", ts, err)
		}
		temp := at(r.Hourly.Temperature, i)
		if temp == nil {
			continue // An hour without temperature is useless on the display.
		}
		f.Hours = append(f.Hours, Hour{
			Time:     t,
			Temp:     *temp,
			RainProb: int(valueOr(at(r.Hourly.RainProb, i), -1)),
			Code:     intOr(at(r.Hourly.WeatherCode, i), -1),
			IsDay:    intOr(at(r.Hourly.IsDay, i), 1) == 1,
		})
	}

	for i, ds := range r.Daily.Time {
		d, err := time.ParseInLocation("2006-01-02", ds, loc)
		if err != nil {
			return nil, fmt.Errorf("daily time %q: %w", ds, err)
		}
		hi, lo := at(r.Daily.Max, i), at(r.Daily.Min, i)
		if hi == nil || lo == nil {
			continue
		}
		f.Days = append(f.Days, Day{
			Date:     d,
			Code:     intOr(at(r.Daily.WeatherCode, i), -1),
			Max:      *hi,
			Min:      *lo,
			RainProb: int(valueOr(at(r.Daily.RainProb, i), -1)),
		})
	}
	return f, nil
}

func at[T any](values []*T, i int) *T {
	if i < len(values) {
		return values[i]
	}
	return nil
}

func valueOr(v *float64, fallback float64) float64 {
	if v == nil {
		return fallback
	}
	return *v
}

func intOr(v *int, fallback int) int {
	if v == nil {
		return fallback
	}
	return *v
}
