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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "k" || r.URL.Query().Get("q") != "São Paulo" {
			w.WriteHeader(400)
			return
		}
		w.Write([]byte(`{"current":{"temp_c":28.5}}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, APIKey: "k", HTTP: srv.Client()}

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
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

			_, err := c.CurrentCelsius(context.Background(), "x")
			assert.Error(t, err)
		})
	}
}
