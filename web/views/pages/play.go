package pages

import (
	"fmt"
	"math"
	"strconv"
	"unicode"
	"workshop/internal/domain/game"
	"workshop/internal/domain/sighting"
	"workshop/web/views/components"
)

type (
	// QuestionProps is a round waiting for a guess.
	QuestionProps struct {
		Game     game.Game
		Sighting sighting.Sighting
	}
	// AnswerProps is the round the player has just guessed at, with the
	// game as it stands after it was scored.
	AnswerProps struct {
		Game     game.Game
		Sighting sighting.Sighting
		Guess    sighting.Coordinates
		Result   game.Result
	}
	// OverProps is a finished game, with the sightings it asked about so
	// each round can be named.
	OverProps struct {
		Game      game.Game
		Sightings []sighting.Sighting
	}
)

type (
	// MapData is what safari.js reads out of the page to draw the map:
	// an empty world to guess on, or a guess and an answer to compare.
	MapData struct {
		Mode   string    `json:"mode"`
		Guess  *MapPoint `json:"guess,omitempty"`
		Answer *MapPoint `json:"answer,omitempty"`
	}
	// MapPoint is one marker on that map.
	MapPoint struct {
		Lat   float64 `json:"lat"`
		Lng   float64 `json:"lng"`
		Label string  `json:"label"`
	}
	// Clue is one hint shown beside the photograph.
	Clue struct {
		Label string
		Value string
	}
)

// guessMap is the world, with nothing on it yet.
func guessMap() MapData {
	return MapData{Mode: "guess"}
}

// answerMap is the guess and the true position, so the player can see how
// far out they were.
func answerMap(props AnswerProps) MapData {
	return MapData{
		Mode:   "answer",
		Guess:  &MapPoint{Lat: props.Guess.Latitude, Lng: props.Guess.Longitude, Label: "Your guess"},
		Answer: &MapPoint{Lat: props.Sighting.Location.Latitude, Lng: props.Sighting.Location.Longitude, Label: place(props.Sighting.Location)},
	}
}

// photo is the image a round is played on.
func photo(s sighting.Sighting) sighting.Media {
	media, _ := game.Photo(s)
	return media
}

// credit names whoever took the photograph, which their licence asks for.
func credit(m sighting.Media) string {
	if m.Creator == "" && m.RightsHolder == "" {
		return "Photographer unrecorded"
	}
	if m.Creator == "" {
		return m.RightsHolder
	}
	return m.Creator
}

// clues are the hints shown with the photograph. The species and the place
// are deliberately missing: working those out is the game.
func clues(s sighting.Sighting) []Clue {
	out := []Clue{
		{Label: "Recorded", Value: s.HappenedAt.Format("January 2006")},
		{Label: "Group", Value: taxon(s)},
	}

	if s.Weather.Temperature.Actual != 0 {
		out = append(out, Clue{Label: "That day", Value: fmt.Sprintf("%.0f°C, %s", s.Weather.Temperature.Actual, condition(s))})
	}

	if s.IndividualCount != nil && *s.IndividualCount > 1 {
		out = append(out, Clue{Label: "Seen", Value: strconv.Itoa(*s.IndividualCount) + " together"})
	}

	return out
}

// taxon is the broadest name for what kind of animal it is, which hints
// without naming the species.
func taxon(s sighting.Sighting) string {
	if s.Species.Class != "" {
		return s.Species.Class
	}
	if s.Species.Phylum != "" {
		return s.Species.Phylum
	}
	return "Unrecorded"
}

// condition is the weather in words, lowercased to sit in a sentence.
func condition(s sighting.Sighting) string {
	if s.Weather.Condition.Description == "" {
		return "conditions unrecorded"
	}
	return s.Weather.Condition.Description
}

// playedName names the animal in round i, or an empty string when that
// sighting is no longer there to name.
func (p OverProps) playedName(i int) string {
	if i < 0 || i >= len(p.Sightings) {
		return ""
	}
	return speciesName(p.Sightings[i])
}

// playedPlace is where round i's animal really was.
func (p OverProps) playedPlace(i int) string {
	if i < 0 || i >= len(p.Sightings) {
		return ""
	}
	return place(p.Sightings[i].Location)
}

// roundLabel says where in the game the player is.
func roundLabel(g game.Game) string {
	return fmt.Sprintf("Round %d of %d", min(g.Round+1, game.Rounds), game.Rounds)
}

// distanceLabel is how far out a guess was, in units that stay readable
// from a few metres to half the planet.
func distanceLabel(km float64) string {
	switch {
	case km < 1:
		return fmt.Sprintf("%.0f m out", math.Round(km*1000))
	case km < 10:
		return fmt.Sprintf("%.1f km out", km)
	default:
		return fmt.Sprintf("%s km out", thousands(int(math.Round(km))))
	}
}

// verdict is a word for how good a guess was, so the number means
// something at a glance.
func verdict(km float64) string {
	switch {
	case km < 25:
		return "Spot on"
	case km < 150:
		return "Very close"
	case km < 500:
		return "Close"
	case km < 1500:
		return "The right part of the world"
	case km < 5000:
		return "Wrong neighbourhood"
	default:
		return "Wrong planet, almost"
	}
}

// thousands groups a number so a score reads at a glance.
func thousands(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}

	var out []byte
	for i, digit := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, digit)
	}
	return string(out)
}

// barWidth is a result's points as a percentage, for the bar under it.
func barWidth(points int) string {
	return fmt.Sprintf("%d%%", points*100/game.MaxRoundScore)
}

// homeTiles is how many sightings the home page shows. The figures above
// them count the whole record; this is just what fits on a page.
const homeTiles = 60

// tiles is the slice of the record the home page renders.
func tiles(sightings []sighting.Sighting) []sighting.Sighting {
	if len(sightings) > homeTiles {
		return sightings[:homeTiles]
	}
	return sightings
}

// speciesName is what the animal is called, once the round is over.
//
// GBIF takes vernacular names as its contributors typed them, so "spotted
// hyaena" arrives beside "African buffalo". Only the first letter is
// touched: the rest may well be a proper noun.
func speciesName(s sighting.Sighting) string {
	name := components.DisplayName(s.Species)
	if name == "" {
		return name
	}

	runes := []rune(name)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// place is where the animal was seen, once the round is over.
func place(l sighting.Location) string {
	return components.Place(l)
}
