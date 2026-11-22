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

	"climate-zipcode/internal/location"
	"climate-zipcode/internal/temperature"

	httpSwagger "github.com/swaggo/http-swagger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type LocationFinder interface {
	Lookup(ctx context.Context, cep string) (string, error)
}

type WeatherFetcher interface {
	CurrentCelsius(ctx context.Context, city string) (float64, error)
}

type Response struct {
	City  string  `json:"city" example:"São Paulo"`
	TempC float64 `json:"temp_C" example:"28.5"`
	TempF float64 `json:"temp_F" example:"83.3"`
	TempK float64 `json:"temp_K" example:"301.5"`
}

var cepRe = regexp.MustCompile(`^\d{8}$`)

// GetWeather godoc
//
// @Summary      Get current weather by ZIP code
// @Description  Receives a Brazilian ZIP code (CEP), identifies the corresponding city using ViaCEP and retrieves its current temperature using WeatherAPI.
// @Description  Temperatures are returned in Celsius, Fahrenheit and Kelvin.
// @Tags         Weather
// @Produce      json
// @Param        zipcode path string true "Brazilian ZIP code (8 digits)" example(01001000)
// @Success      200 {object} Response "Current temperature"
// @Failure      404 {string} string "can not find zipcode"
// @Failure      422 {string} string "invalid zipcode"
// @Failure      500 {string} string "internal error"
// @Router       /weather/{zipcode} [get]
func GetWeather(loc LocationFinder, w WeatherFetcher, tracer trace.Tracer) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		carrier := propagation.HeaderCarrier(r.Header)
		ctx := r.Context()
		ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
		ctx, span := tracer.Start(ctx, "service-b-request")
		defer span.End()

		cep := r.PathValue("zipcode")

		fmt.Println(cep)

		if !cepRe.MatchString(cep) {
			http.Error(rw, "invalid zipcode", http.StatusUnprocessableEntity)
			return
		}

		city, err := loc.Lookup(r.Context(), cep)
		if errors.Is(err, location.ErrNotFound) {
			http.Error(rw, "can not find zipcode", http.StatusNotFound)
			return
		}

		if err != nil {
			log.Printf("location lookup: %v", err)
			http.Error(rw, "internal error", http.StatusInternalServerError)
			return
		}

		c, err := w.CurrentCelsius(r.Context(), city)
		if err != nil {
			log.Printf("weather fetch: %v", err)
			http.Error(rw, "internal error", http.StatusInternalServerError)
			return
		}

		rw.Header().Set("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(Response{
			City:  city,
			TempC: c,
			TempF: temperature.CelsiusToFahrenheit(c),
			TempK: temperature.CelsiusToKelvin(c),
		})
	}
}

// New returns the router serving the API and Swagger documentation.
func New(loc LocationFinder, w WeatherFetcher, tracer trace.Tracer) http.Handler {
	mux := http.NewServeMux()

	// Redirect root to Swagger UI.
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) {
		http.Redirect(rw, r, "/swagger/index.html", http.StatusTemporaryRedirect)
	})

	// Swagger UI.
	mux.Handle("GET /swagger/", httpSwagger.WrapHandler)

	// Weather API.
	mux.HandleFunc("GET /weather/{zipcode}", GetWeather(loc, w, tracer))

	return mux
}
