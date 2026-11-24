// Package orchestrator forwards weather lookups to Service B.
package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
)

// ErrNotFound is returned when Service B responds 404.
// The handler maps it to its own 404 "can not find zipcode".
var ErrNotFound = errors.New("zipcode not found")

// Client is a thin HTTP proxy to Service B.
type Client struct {
	BaseURL string // e.g. http://service-b:8080
	HTTP    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Lookup forwards the CEP to Service B via GET /weather/{cep}/ and returns
// the raw JSON response body as a string.
//
// It is intentionally a proxy: it does NOT parse Service B's payload.
// Only the status code is interpreted:
//   - 2xx         → return body, nil
//   - 404         → ErrNotFound
//   - anything    → wrapped error (handler maps to 500)
func (c *Client) Lookup(ctx context.Context, cep string) (string, error) {
	url := fmt.Sprintf("%s/weather/%s", c.BaseURL, cep)

	ctx, span := otel.Tracer("microservice-tracer").Start(ctx, "service-b-call")
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("orchestrator: read body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("orchestrator: upstream status %d: %s", resp.StatusCode, string(body))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	return string(body), nil
}
