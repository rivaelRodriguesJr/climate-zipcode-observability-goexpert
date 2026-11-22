// Package location resolves a Brazilian CEP to a city using ViaCEP.
package location

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrNotFound is returned when the CEP does not exist.
var ErrNotFound = errors.New("zipcode not found")

type Client struct {
	BaseURL string // e.g. https://viacep.com.br
	HTTP    *http.Client
}

func NewClient() *Client {
	return &Client{BaseURL: "https://viacep.com.br", HTTP: &http.Client{Timeout: 10 * time.Second}}
}

// Lookup returns the city name for the given 8-digit CEP.
func (c *Client) Lookup(ctx context.Context, cep string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/ws/%s/json/", c.BaseURL, cep), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// ViaCEP answers 400 for malformed CEPs; treat as not found.
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest {
		return "", ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("viacep: unexpected status %d", resp.StatusCode)
	}
	var body struct {
		Localidade string `json:"localidade"`
		Erro       any    `json:"erro"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.Erro != nil && body.Erro != false && body.Erro != "false" || body.Localidade == "" {
		return "", ErrNotFound
	}
	return body.Localidade, nil
}
