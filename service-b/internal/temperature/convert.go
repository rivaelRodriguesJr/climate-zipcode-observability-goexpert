// Package temperature provides temperature unit conversions.
package temperature

import "math"

func round(v float64, places int) float64 {
	p := math.Pow10(places)
	return math.Round(v*p) / p
}

// CelsiusToFahrenheit converts C to F (F = C*1.8 + 32), rounded to 1 decimal.
func CelsiusToFahrenheit(c float64) float64 { return round(c*1.8+32, 1) }

// CelsiusToKelvin converts C to K (K = C + 273), rounded to 2 decimals.
func CelsiusToKelvin(c float64) float64 { return round(c+273, 2) }
