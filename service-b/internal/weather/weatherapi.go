// Package weather fetches the current temperature from WeatherAPI.
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	BaseURL string // e.g. https://api.weatherapi.com
	APIKey  string
	HTTP    *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{BaseURL: "https://api.weatherapi.com", APIKey: apiKey, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

// CurrentCelsius returns the current temperature in Celsius for a city.
func (c *Client) CurrentCelsius(ctx context.Context, city string) (float64, error) {
	q := url.Values{"key": {c.APIKey}, "q": {city}, "aqi": {"no"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/current.json?"+q.Encode(), nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("weatherapi: unexpected status %d", resp.StatusCode)
	}
	var body struct {
		Current struct {
			TempC *float64 `json:"temp_c"`
		} `json:"current"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, err
	}
	if body.Current.TempC == nil {
		return 0, fmt.Errorf("weatherapi: missing temp_c")
	}
	return *body.Current.TempC, nil
}
