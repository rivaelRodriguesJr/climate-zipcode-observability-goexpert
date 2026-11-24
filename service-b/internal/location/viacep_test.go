package location

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, status int, body string) (*Client, *string) {
	t.Helper()
	var path string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	return &Client{BaseURL: "http://viacep.test", HTTP: inMemory(h)}, &path
}

func TestLookupSuccess(t *testing.T) {
	c, path := newTestClient(t, 200, `{"cep":"01001-000","localidade":"São Paulo"}`)

	city, err := c.Lookup(context.Background(), "01001000")
	require.NoError(t, err)
	assert.Equal(t, "São Paulo", city)
	assert.Equal(t, "/ws/01001000/json/", *path)
}

func TestLookupNotFound(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"viacep erro true (bool)", 200, `{"erro": true}`},
		{"viacep erro true (string)", 200, `{"erro": "true"}`},
		{"empty city", 200, `{}`},
		{"http 404", 404, ``},
		{"http 400", 400, ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestClient(t, tt.status, tt.body)
			_, err := c.Lookup(context.Background(), "00000000")
			assert.ErrorIs(t, err, ErrNotFound)
		})
	}
}

func TestLookupInternalErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"upstream 500", 500, ``},
		{"invalid json", 200, `not json`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestClient(t, tt.status, tt.body)
			_, err := c.Lookup(context.Background(), "01001000")
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrNotFound)
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
