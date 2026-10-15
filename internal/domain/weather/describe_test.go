package weather_test

import (
	"testing"
	"workshop/internal/domain/weather"

	"github.com/stretchr/testify/assert"
)

func TestDescribe(t *testing.T) {
	t.Parallel()

	tt := map[string]struct {
		code int
		want string
	}{
		// The three commonest codes in the data. The stored descriptions
		// for these came from the wrong WMO table and said, respectively,
		// "Clouds dissolving", "State of sky unchanged" and "Clouds forming".
		"clear sky":     {code: 0, want: "Clear sky"},
		"mainly clear":  {code: 1, want: "Mainly clear"},
		"partly cloudy": {code: 2, want: "Partly cloudy"},
		"overcast":      {code: 3, want: "Overcast"},

		"drizzle":         {code: 51, want: "Light drizzle"},
		"slight rain":     {code: 61, want: "Slight rain"},
		"moderate rain":   {code: 63, want: "Moderate rain"},
		"heavy rain":      {code: 65, want: "Heavy rain"},
		"slight snow":     {code: 71, want: "Slight snow"},
		"moderate snow":   {code: 73, want: "Moderate snow"},
		"heavy snow":      {code: 75, want: "Heavy snow"},
		"rain showers":    {code: 80, want: "Slight rain showers"},
		"heavier showers": {code: 81, want: "Moderate rain showers"},
		"violent showers": {code: 82, want: "Violent rain showers"},
		"thunderstorm":    {code: 95, want: "Thunderstorm"},
		"slight hail":     {code: 96, want: "Thunderstorm with slight hail"},
		"heavy hail":      {code: 99, want: "Thunderstorm with heavy hail"},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, weather.Describe(test.code))
		})
	}
}

func TestDescribe_ReadsEvenWhenTheCodeIsUnknown(t *testing.T) {
	t.Parallel()

	// A code Open-Meteo adds later, or a corrupt row, must not leave the
	// clue panel with a blank or a number where the weather should be.
	assert.NotEmpty(t, weather.Describe(999))
	assert.NotEmpty(t, weather.Describe(-1))
}
