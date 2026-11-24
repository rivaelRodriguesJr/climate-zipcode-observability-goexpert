package orchestrator

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func serverB(t *testing.T, fn http.HandlerFunc) *Client {
	t.Helper()
	c := NewClient("http://service-b.test")
	c.HTTP = inMemory(fn)
	return c
}

func TestLookupSuccessReturnsBodyVerbatim(t *testing.T) {
	const payload = `{"city":"São Paulo","temp_C":28.5,"temp_F":83.3,"temp_K":301.5}`
	var path string
	c := serverB(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(payload))
	})

	got, err := c.Lookup(context.Background(), "01001001")
	require.NoError(t, err)
	assert.Equal(t, payload, got)
	assert.Equal(t, "/weather/01001001", path)
}

func TestLookup404MapsToErrNotFound(t *testing.T) {
	c := serverB(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "can not find zipcode", http.StatusNotFound)
	})
	_, err := c.Lookup(context.Background(), "00000000")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestLookupOtherStatusesAreErrors(t *testing.T) {
	for _, status := range []int{400, 422, 500, 503} {
		c := serverB(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })
		_, err := c.Lookup(context.Background(), "01001001")
		require.Error(t, err, "status %d", status)
		assert.NotErrorIs(t, err, ErrNotFound, "status %d", status)
	}
}

func TestLookupUnreachableServiceB(t *testing.T) {
	c := NewClient("http://service-b.test")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})}

	_, err := c.Lookup(context.Background(), "01001001")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound)
}

// The trace context must reach service B as a child of service A's span.
func TestLookupPropagatesTraceContext(t *testing.T) {
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	ctx, parent := tp.Tracer("test").Start(context.Background(), "parent")
	defer parent.End()

	var header string
	c := serverB(t, func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("traceparent")
		_, _ = w.Write([]byte(`{}`))
	})
	_, err := c.Lookup(ctx, "01001001")
	require.NoError(t, err)

	require.NotEmpty(t, header, "traceparent header not sent to service B")
	assert.Contains(t, header, parent.SpanContext().TraceID().String(),
		"traceparent must belong to the caller's trace")
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
