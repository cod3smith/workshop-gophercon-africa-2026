package weather

// Describe is the plain-English phrase for an Open-Meteo weather_code.
//
// It exists because the Condition.Description stored with each record is
// wrong. internal/clients/openmeteo/codes.go numbers WMO table 4677, the
// present-weather table for manned stations, but Open-Meteo's weather_code
// is a different, much shorter table of its own, covering only what a
// forecast model can produce. The two roughly agree for the precipitation
// codes and disagree exactly where the data is densest: under table 4677, code 1
// is "Clouds dissolving", 2 is "State of sky unchanged" and 3 is "Clouds
// forming", while Open-Meteo means "Mainly clear", "Partly cloudy" and
// "Overcast". The stored descriptions were generated from the wrong table,
// so most of the record's weather reads as something it was not.
//
// Condition.Code is correct, so readers describe the code with this rather
// than trusting Condition.Description. The stored column is deliberately
// left alone: re-describing at read time fixes every page without a
// migration.
//
// See https://open-meteo.com/en/docs#weather_variables ("WMO Weather
// interpretation codes").
func Describe(code int) string {
	if phrase, ok := descriptions[code]; ok {
		return phrase
	}
	return unknown
}

// unknown reads as weather without claiming a condition that was never
// recorded.
const unknown = "Unsettled"

// descriptions is every code Open-Meteo publishes for weather_code.
var descriptions = map[int]string{
	0:  "Clear sky",
	1:  "Mainly clear",
	2:  "Partly cloudy",
	3:  "Overcast",
	45: "Fog",
	48: "Freezing fog",
	51: "Light drizzle",
	53: "Moderate drizzle",
	55: "Dense drizzle",
	56: "Light freezing drizzle",
	57: "Dense freezing drizzle",
	61: "Slight rain",
	63: "Moderate rain",
	65: "Heavy rain",
	66: "Light freezing rain",
	67: "Heavy freezing rain",
	71: "Slight snow",
	73: "Moderate snow",
	75: "Heavy snow",
	77: "Snow grains",
	80: "Slight rain showers",
	81: "Moderate rain showers",
	82: "Violent rain showers",
	85: "Slight snow showers",
	86: "Heavy snow showers",
	95: "Thunderstorm",
	96: "Thunderstorm with slight hail",
	99: "Thunderstorm with heavy hail",
}
