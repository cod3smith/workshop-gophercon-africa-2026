package pages

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"uuid"
	"workshop/internal/domain/sighting"
	"workshop/internal/domain/species"
	"workshop/internal/domain/weather"
)

// RecordProps is one sighting, with everything the page needs to show of
// it already found.
type RecordProps struct {
	Sighting sighting.Sighting
	// Recording is the sound of the animal, already found by the handler,
	// or nil when there is none to offer: the species has no recording, or
	// the lookup failed or had not finished. The view never goes and looks.
	Recording *species.Recording
}

// hasPosition reports whether the sighting says where it was. The record
// stores a position for every sighting, and (0, 0) is what one that never
// had a position comes back as: a spot in the Gulf of Guinea that no
// animal on this record was seen at.
func hasPosition(l sighting.Location) bool {
	return l.Latitude != 0 || l.Longitude != 0
}

// recordMap is the one place the sighting was recorded.
func recordMap(s sighting.Sighting) MapData {
	return MapData{
		Mode:   "record",
		Answer: &MapPoint{Lat: s.Location.Latitude, Lng: s.Location.Longitude, Label: place(s.Location)},
	}
}

// scientificLine is the Latin name as recorded, or the plain fact that
// there is none.
func scientificLine(s sighting.Sighting) string {
	if name := strings.TrimSpace(s.Species.ScientificName); name != "" {
		return name
	}
	return "No scientific name recorded"
}

// recordDescription is the line a pasted link unfurls with.
func recordDescription(s sighting.Sighting) string {
	return fmt.Sprintf("%s, seen at %s. %s.", speciesName(s), place(s.Location), dateLabel(s))
}

// dateLabel is the day it was seen, or the fact that nobody wrote it down.
func dateLabel(s sighting.Sighting) string {
	if s.HappenedAt.IsZero() {
		return "Date not recorded"
	}
	return s.HappenedAt.Format("2 January 2006")
}

// positionLabel is the coordinates to the precision the record can stand
// behind, and how sure the observer was where it was, when they said.
func positionLabel(l sighting.Location) string {
	if !hasPosition(l) {
		return "Not recorded"
	}

	out := strconv.FormatFloat(l.Latitude, 'f', 4, 64) + ", " + strconv.FormatFloat(l.Longitude, 'f', 4, 64)
	if u := l.CoordinateUncertaintyInMetres; u != nil && *u > 0 {
		if *u >= 1000 {
			out += fmt.Sprintf(" (within %.1f km)", *u/1000)
		} else {
			out += fmt.Sprintf(" (within %.0f m)", *u)
		}
	}
	return out
}

// countLabel is how many animals the observer saw.
func countLabel(s sighting.Sighting) string {
	switch n := s.IndividualCount; {
	case n == nil || *n < 1:
		return "Count not recorded"
	case *n == 1:
		return "1 animal"
	default:
		return strconv.Itoa(*n) + " together"
	}
}

// observerLabel is who saw it.
func observerLabel(s sighting.Sighting) string {
	if s.RecordedBy == "" {
		return "Observer not recorded"
	}
	return s.RecordedBy
}

// facts are the rows of what the record says about the sighting. Each one
// answers even when the record does not, so a missing field reads as
// missing rather than as a gap in the page.
func facts(s sighting.Sighting) []Clue {
	group := Clue{Label: "Group", Value: Taxon(s), Title: taxonLatin(s)}

	out := []Clue{
		{Label: "Seen at", Value: place(s.Location)},
		{Label: "On", Value: dateLabel(s)},
		group,
		{Label: "Seen", Value: countLabel(s)},
		{Label: "Observer", Value: observerLabel(s)},
	}

	// Elevation is a nicety few observers give, so it is left out rather
	// than stated missing.
	if e := s.Location.ElevationInMetres; e != nil {
		out = append(out, Clue{Label: "Elevation", Value: fmt.Sprintf("%.0f m", *e)})
	}

	return out
}

// hasWeather reports whether a reading came with the sighting.
func hasWeather(s sighting.Sighting) bool {
	return s.Weather.ID != uuid.Nil()
}

// conditions are the weather that day, in words and units a person uses.
func conditions(s sighting.Sighting) []Clue {
	w := s.Weather

	return []Clue{
		{Label: "Sky", Value: weather.Describe(w.Condition.Code)},
		{Label: "Temperature", Value: temperatureLabel(w.Temperature)},
		{Label: "Wind", Value: windLabel(w.Wind)},
		{Label: "Rain", Value: rainLabel(w.Precipitation)},
		{Label: "Cloud cover", Value: fmt.Sprintf("%d%%", w.CloudCover)},
		{Label: "Humidity", Value: fmt.Sprintf("%d%%", w.Humidity)},
	}
}

// temperatureLabel is the air temperature, and what it felt like when that
// is different enough to be worth saying.
func temperatureLabel(t weather.Temperature) string {
	out := fmt.Sprintf("%.0f°C", t.Actual)
	if math.Abs(t.Apparent-t.Actual) >= 1 {
		out += fmt.Sprintf(", feels like %.0f°C", t.Apparent)
	}
	return out
}

// windLabel is the wind's strength and where it blew from.
func windLabel(w weather.Wind) string {
	if w.Speed < 1 {
		return "Calm"
	}

	out := fmt.Sprintf("%.0f km/h from the %s", w.Speed, compass(w.Direction))
	if w.Gusts >= w.Speed+5 {
		out += fmt.Sprintf(", gusting %.0f", w.Gusts)
	}
	return out
}

// rainLabel is what fell in the hour.
func rainLabel(p weather.Precipitation) string {
	if p.Total <= 0 {
		return "None"
	}
	return fmt.Sprintf("%.1f mm", p.Total)
}

// compass names the direction of degrees clockwise from north, to the
// nearest of eight.
func compass(degrees int) string {
	points := [...]string{"north", "north-east", "east", "south-east", "south", "south-west", "west", "north-west"}

	normal := ((degrees % 360) + 360) % 360
	return points[int(math.Round(float64(normal)/45))%len(points)]
}
