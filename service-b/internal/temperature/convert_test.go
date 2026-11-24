package temperature

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCelsiusToFahrenheit(t *testing.T) {
	tests := []struct{ c, want float64 }{
		{0, 32},
		{100, 212},
		{-40, -40},
		{28.5, 83.3},
		{25.55, 78},
	}
	for _, tt := range tests {
		assert.InDelta(t, tt.want, CelsiusToFahrenheit(tt.c), 1e-9, "C=%v", tt.c)
	}
}

func TestCelsiusToKelvin(t *testing.T) {
	tests := []struct{ c, want float64 }{
		{0, 273},
		{25, 298},
		{-273, 0},
		{28.55, 301.55},
	}
	for _, tt := range tests {
		assert.InDelta(t, tt.want, CelsiusToKelvin(tt.c), 1e-9, "C=%v", tt.c)
	}
}
