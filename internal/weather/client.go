package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Open-Meteo endpoints. Both are free and need no API key.
const (
	DefaultForecastURL = "https://api.open-meteo.com/v1/forecast"
	DefaultGeocodeURL  = "https://geocoding-api.open-meteo.com/v1/search"
)

const (
	currentFields = "temperature_2m,apparent_temperature,relative_humidity_2m,is_day,weather_code,wind_speed_10m"
	hourlyFields  = "temperature_2m,precipitation_probability,weather_code,is_day"
	dailyFields   = "weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max"
	forecastDays  = 7
	maxBodySize   = 2 << 20
)

// Query identifies one forecast request. It is comparable, so it can key a cache.
type Query struct {
	Latitude   float64
	Longitude  float64
	Fahrenheit bool
}

// Client calls the Open-Meteo APIs. The zero value is ready to use.
type Client struct {
	HTTP        *http.Client
	ForecastURL string
	GeocodeURL  string
	UserAgent   string
}

// ForecastRequestURL builds the forecast URL for q.
func (c *Client) ForecastRequestURL(q Query) string {
	v := url.Values{}
	v.Set("latitude", strconv.FormatFloat(q.Latitude, 'f', 4, 64))
	v.Set("longitude", strconv.FormatFloat(q.Longitude, 'f', 4, 64))
	v.Set("current", currentFields)
	v.Set("hourly", hourlyFields)
	v.Set("daily", dailyFields)
	v.Set("timezone", "auto")
	v.Set("forecast_days", strconv.Itoa(forecastDays))
	if q.Fahrenheit {
		v.Set("temperature_unit", "fahrenheit")
	}
	return orDefault(c.ForecastURL, DefaultForecastURL) + "?" + v.Encode()
}

// Fetch downloads and parses the forecast for q.
func (c *Client) Fetch(ctx context.Context, q Query) (*Forecast, error) {
	body, err := c.get(ctx, c.ForecastRequestURL(q))
	if err != nil {
		return nil, err
	}
	return Parse(body)
}

// Search looks up places by name. lang is a two-letter language code for the result names.
func (c *Client) Search(ctx context.Context, name, lang string) ([]Place, error) {
	v := url.Values{}
	v.Set("name", name)
	v.Set("count", "10")
	v.Set("format", "json")
	if lang != "" {
		v.Set("language", lang)
	}
	body, err := c.get(ctx, orDefault(c.GeocodeURL, DefaultGeocodeURL)+"?"+v.Encode())
	if err != nil {
		return nil, err
	}
	return ParsePlaces(body)
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		// Open-Meteo explains client errors in a JSON "reason" field.
		var e struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal(body, &e) == nil && e.Reason != "" {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, e.Reason)
		}
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return body, nil
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
