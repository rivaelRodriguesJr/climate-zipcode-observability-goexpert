package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"climate-zipcode/internal/location"
	"climate-zipcode/internal/weather"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

type fakeLoc struct {
	city  string
	err   error
	calls int
}

func (f *fakeLoc) Lookup(context.Context, string) (string, error) {
	f.calls++
	return f.city, f.err
}

type fakeWeather struct {
	c     float64
	err   error
	calls int
}

func (f *fakeWeather) CurrentCelsius(context.Context, string) (float64, error) {
	f.calls++
	return f.c, f.err
}

func do(h http.Handler, cep string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/weather/"+cep, nil))
	return rec
}

func newHandler(l LocationFinder, w WeatherFetcher) http.Handler {
	return New(l, w, noop.NewTracerProvider().Tracer("test"))
}

func TestInvalidZipcodeReturns422(t *testing.T) {
	for _, cep := range []string{"123", "0100100", "010010011", "abcdefgh", "0100100a", "01001-00"} {
		t.Run(cep, func(t *testing.T) {
			l, w := &fakeLoc{}, &fakeWeather{}
			rec := do(newHandler(l, w), cep)

			assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
			assert.Contains(t, rec.Body.String(), "invalid zipcode")
			assert.Zero(t, l.calls, "must not call ViaCEP for an invalid zipcode")
			assert.Zero(t, w.calls)
		})
	}
}

func TestZipcodeNotFoundReturns404(t *testing.T) {
	l, w := &fakeLoc{err: location.ErrNotFound}, &fakeWeather{}
	rec := do(newHandler(l, w), "00000000")

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "can not find zipcode")
	assert.Zero(t, w.calls, "must not call WeatherAPI when the zipcode does not exist")
}

func TestWrappedNotFoundReturns404(t *testing.T) {
	l := &fakeLoc{err: errors.Join(errors.New("ctx"), location.ErrNotFound)}
	rec := do(newHandler(l, &fakeWeather{}), "00000000")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDependencyFailuresReturn500(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name string
		l    *fakeLoc
		w    *fakeWeather
	}{
		{"viacep failure", &fakeLoc{err: boom}, &fakeWeather{}},
		{"weatherapi failure", &fakeLoc{city: "São Paulo"}, &fakeWeather{err: boom}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(newHandler(tt.l, tt.w), "01001000")
			assert.Equal(t, http.StatusInternalServerError, rec.Code)
			assert.Contains(t, rec.Body.String(), "internal error")
		})
	}
}

func TestSuccessReturnsAllUnits(t *testing.T) {
	rec := do(newHandler(&fakeLoc{city: "São Paulo"}, &fakeWeather{c: 28.5}), "01001000")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var got Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "São Paulo", got.City)
	assert.InDelta(t, 28.5, got.TempC, 1e-9)
	assert.InDelta(t, 83.3, got.TempF, 1e-9)
	assert.InDelta(t, 301.5, got.TempK, 1e-9)
}

func TestSuccessJSONFieldNames(t *testing.T) {
	rec := do(newHandler(&fakeLoc{city: "X"}, &fakeWeather{c: 0}), "01001000")

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	for _, k := range []string{"city", "temp_C", "temp_F", "temp_K"} {
		assert.Contains(t, raw, k)
	}
}

// Integration: real handler + real ViaCEP/WeatherAPI clients against fake upstreams.
func TestIntegrationWithUpstreams(t *testing.T) {
	var gotCity string
	via := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws/01001000/json/":
			_, _ = w.Write([]byte(`{"localidade":"São Paulo"}`))
		case "/ws/99999999/json/":
			_, _ = w.Write([]byte(`{"erro":true}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	wx := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCity = r.URL.Query().Get("q")
		_, _ = w.Write([]byte(`{"current":{"temp_c":10}}`))
	})

	h := newHandler(
		&location.Client{BaseURL: "http://viacep.test", HTTP: inMemory(via)},
		&weather.Client{BaseURL: "http://weather.test", APIKey: "k", HTTP: inMemory(wx)},
	)

	t.Run("success", func(t *testing.T) {
		rec := do(h, "01001000")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "São Paulo", gotCity, "WeatherAPI must be queried with the city returned by ViaCEP")

		var got Response
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, Response{City: "São Paulo", TempC: 10, TempF: 50, TempK: 283}, got)
	})
	t.Run("nonexistent zipcode", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, do(h, "99999999").Code)
	})
	t.Run("malformed zipcode", func(t *testing.T) {
		assert.Equal(t, http.StatusUnprocessableEntity, do(h, "1234").Code)
	})
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
