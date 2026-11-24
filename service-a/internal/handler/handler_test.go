package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"climate-zipcode/internal/orchestrator"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
)

type fakeClient struct {
	res   string
	err   error
	calls int
	cep   string
}

func (f *fakeClient) Lookup(_ context.Context, cep string) (string, error) {
	f.calls++
	f.cep = cep
	return f.res, f.err
}

func post(h http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/weather", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newHandler(c OrchestratorClient) http.Handler {
	return New(c, noop.NewTracerProvider().Tracer("test"))
}

func TestInvalidInputRejectedWithoutCallingServiceB(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"malformed json", `{"cep":`, http.StatusBadRequest},
		{"empty body", ``, http.StatusBadRequest},
		{"unknown field", `{"cep":"01001001","x":1}`, http.StatusBadRequest},
		{"missing cep", `{}`, http.StatusUnprocessableEntity},
		{"cep as number", `{"cep":1001001}`, http.StatusUnprocessableEntity},
		{"cep as null", `{"cep":null}`, http.StatusUnprocessableEntity},
		{"cep as bool", `{"cep":true}`, http.StatusUnprocessableEntity},
		{"too short", `{"cep":"0100100"}`, http.StatusUnprocessableEntity},
		{"too long", `{"cep":"010010011"}`, http.StatusUnprocessableEntity},
		{"with hyphen", `{"cep":"01001-001"}`, http.StatusUnprocessableEntity},
		{"letters", `{"cep":"abcdefgh"}`, http.StatusUnprocessableEntity},
		{"empty string", `{"cep":""}`, http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeClient{}
			rec := post(newHandler(c), tt.body)
			assert.Equal(t, tt.want, rec.Code, rec.Body.String())
			assert.Zero(t, c.calls, "service B must not be called for invalid input")
		})
	}
}

func TestInvalidZipcodeMessage(t *testing.T) {
	rec := post(newHandler(&fakeClient{}), `{"cep":"123"}`)
	assert.Contains(t, rec.Body.String(), "invalid zipcode")
}

func TestNonexistentZipcodeReturns404(t *testing.T) {
	// Regression: the orchestrator's error must be recognised by the handler.
	for name, err := range map[string]error{
		"direct":  orchestrator.ErrNotFound,
		"wrapped": fmt.Errorf("lookup: %w", orchestrator.ErrNotFound),
	} {
		t.Run(name, func(t *testing.T) {
			rec := post(newHandler(&fakeClient{err: err}), `{"cep":"00000000"}`)
			assert.Equal(t, http.StatusNotFound, rec.Code)
			assert.Contains(t, rec.Body.String(), "can not find zipcode")
		})
	}
}

func TestOtherErrorsReturn500(t *testing.T) {
	rec := post(newHandler(&fakeClient{err: errors.New("boom")}), `{"cep":"01001001"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestSuccessRelaysPayloadVerbatim(t *testing.T) {
	payload := `{"city":"São Paulo","temp_C":28.5,"temp_F":83.3,"temp_K":301.5}`
	c := &fakeClient{res: payload}

	rec := post(newHandler(c), `{"cep":" 01001001 "}`) // surrounding spaces are trimmed

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, payload, rec.Body.String())
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "01001001", c.cep)
}
