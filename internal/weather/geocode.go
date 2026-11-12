package weather

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Place is a geocoding search result.
type Place struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Admin1      string  `json:"admin1"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

// Label returns a name that tells apart places sharing a name, e.g. "Córdoba, Provincia de Córdoba, Argentina".
func (p Place) Label() string {
	parts := []string{p.Name}
	if p.Admin1 != "" && !strings.EqualFold(p.Admin1, p.Name) {
		parts = append(parts, p.Admin1)
	}
	if p.Country != "" {
		parts = append(parts, p.Country)
	} else if p.CountryCode != "" {
		parts = append(parts, p.CountryCode)
	}
	return strings.Join(parts, ", ")
}

// ShortLabel returns "Name, Country" for the key display and saved settings.
func (p Place) ShortLabel() string {
	country := p.Country
	if country == "" {
		country = p.CountryCode
	}
	if country == "" {
		return p.Name
	}
	return p.Name + ", " + country
}

// ParsePlaces decodes an Open-Meteo geocoding response. No matches yields an empty slice.
func ParsePlaces(data []byte) ([]Place, error) {
	var r struct {
		Results []struct {
			ID          int64   `json:"id"`
			Name        string  `json:"name"`
			Latitude    float64 `json:"latitude"`
			Longitude   float64 `json:"longitude"`
			CountryCode string  `json:"country_code"`
			Country     string  `json:"country"`
			Admin1      string  `json:"admin1"`
		} `json:"results"`
		Error  bool   `json:"error"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("decoding search results: %w", err)
	}
	if r.Error {
		return nil, fmt.Errorf("open-meteo error: %s", r.Reason)
	}
	places := make([]Place, 0, len(r.Results))
	for _, res := range r.Results {
		places = append(places, Place{
			ID:          res.ID,
			Name:        res.Name,
			Admin1:      res.Admin1,
			Country:     res.Country,
			CountryCode: res.CountryCode,
			Latitude:    res.Latitude,
			Longitude:   res.Longitude,
		})
	}
	return places, nil
}
