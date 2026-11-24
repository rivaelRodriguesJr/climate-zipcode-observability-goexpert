package internal_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"climate-zipcode/internal/handler"
	"climate-zipcode/internal/orchestrator"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

// End-to-end inside service A: HTTP handler + real orchestrator client
// talking to a fake service B.
func TestServiceAThroughOrchestrator(t *testing.T) {
	b := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/weather/01001001":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"city":"São Paulo","temp_C":28.5,"temp_F":83.3,"temp_K":301.5}`))
		case "/weather/00000000":
			http.Error(w, "can not find zipcode", http.StatusNotFound)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	})

	client := orchestrator.NewClient("http://service-b.test")
	client.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		b.ServeHTTP(rec, r)
		return rec.Result(), nil
	})}
	a := handler.New(client, noop.NewTracerProvider().Tracer("t"))

	call := func(body string) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodPost, "/weather", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		a.ServeHTTP(rec, req)
		var m map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &m))
		return rec.Code, m
	}

	t.Run("200 with weather", func(t *testing.T) {
		code, body := call(`{"cep":"01001001"}`)
		assert.Equal(t, 200, code)
		assert.Equal(t, map[string]any{"city": "São Paulo", "temp_C": 28.5, "temp_F": 83.3, "temp_K": 301.5}, body)
	})
	t.Run("404 for nonexistent zipcode", func(t *testing.T) {
		code, _ := call(`{"cep":"00000000"}`)
		assert.Equal(t, 404, code)
	})
	t.Run("422 for invalid zipcode", func(t *testing.T) {
		code, _ := call(`{"cep":"123"}`)
		assert.Equal(t, 422, code)
	})
	t.Run("500 when service B fails", func(t *testing.T) {
		code, _ := call(`{"cep":"12345678"}`)
		assert.Equal(t, 500, code)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
