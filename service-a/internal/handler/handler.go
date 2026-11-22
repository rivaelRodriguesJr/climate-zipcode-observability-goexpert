// Package handler exposes the HTTP API.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	httpSwagger "github.com/swaggo/http-swagger"
)

// OrchestratorClient is the downstream service that resolves a CEP
// and returns the weather payload as raw JSON.
type OrchestratorClient interface {
	Lookup(ctx context.Context, cep string) (string, error)
}

// ErrZipNotFound is returned by the orchestrator when the CEP does not exist.
var ErrZipNotFound = errors.New("zipcode not found")

// WeatherResponse is the JSON returned on success.
type WeatherResponse struct {
	City  string  `json:"city" example:"São Paulo"`
	TempC float64 `json:"temp_C" example:"28.5"`
	TempF float64 `json:"temp_F" example:"83.3"`
	TempK float64 `json:"temp_K" example:"301.5"`
}

// WeatherRequest is the JSON expected in the POST body.
// Cep is kept as json.RawMessage so we can distinguish
// "not a string" from "malformed JSON" and map both to 422.
type WeatherRequest struct {
	Cep json.RawMessage `json:"cep" swaggertype:"string" example:"01001001"`
}

var cepRe = regexp.MustCompile(`^\d{8}$`)

const maxBodyBytes = 1 << 20 // 1 MiB

// HandleWeather godoc
//
// @Summary      Get current weather by ZIP code
// @Description  Receives a Brazilian ZIP code (CEP) as a JSON body, resolves the
// @Description  corresponding city via ViaCEP and retrieves its current temperature
// @Description  via WeatherAPI. Temperatures are returned in Celsius, Fahrenheit and Kelvin.
// @Tags         Weather
// @Accept       json
// @Produce      json
// @Param        request body WeatherRequest true "Brazilian ZIP code (8 digits)"
// @Success      200 {object} WeatherResponse "Current temperature"
// @Failure      400 {string} string "invalid request body"
// @Failure      404 {string} string "can not find zipcode"
// @Failure      422 {string} string "invalid zipcode"
// @Failure      500 {string} string "internal error"
// @Router       /weather [post]
func HandleWeather(client OrchestratorClient, tracer trace.Tracer) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		carrier := propagation.HeaderCarrier(r.Header)
		ctx := r.Context()
		ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
		ctx, span := tracer.Start(ctx, "service-a-request")
		defer span.End()

		// 1. Cap request body to avoid abuse.
		r.Body = http.MaxBytesReader(rw, r.Body, maxBodyBytes)

		// 2. Decode JSON strictly.
		var req WeatherRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeJSONError(rw, http.StatusBadRequest, "invalid request body")
			return
		}

		// 3. Extract CEP. Must be a JSON string with exactly 8 digits.
		//    Any type mismatch or format problem → 422 "invalid zipcode".
		cep, err := parseCEP(req.Cep)
		if err != nil {
			writeJSONError(rw, http.StatusUnprocessableEntity, "invalid zipcode")
			return
		}

		// 4. Forward to the orchestrator.
		res, err := client.Lookup(r.Context(), cep)
		if err != nil {
			fmt.Println(err)
			if errors.Is(err, ErrZipNotFound) {
				writeJSONError(rw, http.StatusNotFound, "can not find zipcode")
				return
			}
			log.Printf("handler: lookup failed for cep=%q: %v", cep, err)
			writeJSONError(rw, http.StatusInternalServerError, "internal error")
			return
		}

		// 5. Success — relay the orchestrator's JSON verbatim.
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte(res))
	}
}

// parseCEP decodes raw into a string and validates it is exactly 8 digits.
// Returns an error for any non-string type or invalid format.
func parseCEP(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("missing cep")
	}

	var cep string
	if err := json.Unmarshal(raw, &cep); err != nil {
		// Value was not a JSON string (e.g. number, bool, null).
		return "", errors.New("cep must be a string")
	}

	cep = strings.TrimSpace(cep)
	if !cepRe.MatchString(cep) {
		return "", errors.New("cep must be 8 digits")
	}

	return cep, nil
}

// writeJSONError writes a JSON error body: {"error": "<msg>"}.
func writeJSONError(rw http.ResponseWriter, status int, msg string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(map[string]string{"error": msg})
}

func New(client OrchestratorClient, tracer trace.Tracer) http.Handler {
	mux := http.NewServeMux()

	// Redirect root to Swagger UI.
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) {
		http.Redirect(rw, r, "/swagger/index.html", http.StatusTemporaryRedirect)
	})

	// Swagger UI.
	mux.Handle("GET /swagger/", httpSwagger.WrapHandler)

	// Weather API — POST /weather with JSON body {"cep":"..."}.
	mux.HandleFunc("POST /weather", HandleWeather(client, tracer))

	return mux
}
