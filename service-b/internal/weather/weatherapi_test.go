package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentCelsius(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "k" || r.URL.Query().Get("q") != "São Paulo" {
			w.WriteHeader(400)
			return
		}
		_, _ = w.Write([]byte(`{"current":{"temp_c":28.5}}`))
	})
	c := &Client{BaseURL: "http://weather.test", APIKey: "k", HTTP: inMemory(h)}

	got, err := c.CurrentCelsius(context.Background(), "São Paulo")
	require.NoError(t, err)
	assert.Equal(t, 28.5, got)
}

func TestCurrentCelsiusErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"unauthorized", 401, `{}`},
		{"missing temp", 200, `{}`},
		{"invalid json", 200, `not json`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			c := &Client{BaseURL: "http://weather.test", HTTP: inMemory(h)}

			_, err := c.CurrentCelsius(context.Background(), "x")
			assert.Error(t, err)
		})
	}
}

// inMemory returns an http.Client that serves requests with h directly,
// without opening any network socket.
func inMemory(h http.Handler) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Result(), nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
