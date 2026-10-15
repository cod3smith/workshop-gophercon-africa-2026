package pages

import (
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"
	"unicode"
	"uuid"
	"workshop/internal/domain/game"
	"workshop/internal/domain/sighting"
	"workshop/internal/domain/weather"
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
		Game game.Game
		// Sightings is keyed by sighting id, so a round names the animal it
		// was actually drawn against rather than whatever is at its index.
		Sightings map[uuid.UUID]sighting.Sighting
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
		// Title is the Latin behind a plain-English value, shown on hover.
		Title string
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

// originalName is the file iNaturalist keeps the uploaded photograph under,
// before its extension. It keeps scaled copies beside it, named by size.
const originalName = "original"

// PhotoURL is the photograph at the given size ("medium", "large"), for a
// page that has no use for a 2048-pixel original.
//
// Only iNaturalist keeps sized copies, so a URL whose final path segment is
// not "original.<ext>" is returned as it was: guessing at another host's
// layout would hand the page a broken image.
func PhotoURL(m sighting.Media, size string) string {
	dir, file := path.Split(m.URL)
	extension, ok := strings.CutPrefix(file, originalName+".")
	if !ok {
		return m.URL
	}
	return dir + size + "." + extension
}

// Credit names whoever took the photograph, which their licence asks for.
//
// Some records carry the species' own genus where a photographer should be
// (a tortoise's, over a photograph of an impala). That is no attribution,
// and on a round that has not been guessed it names an animal, so a credit
// that is the scientific name or its genus, in any case, counts as none.
func Credit(m sighting.Media, scientificName string) string {
	name := m.Creator
	if name == "" {
		name = m.RightsHolder
	}

	if name == "" || namesTheSpecies(name, scientificName) {
		return "Photographer unrecorded"
	}
	return name
}

// namesTheSpecies reports whether credit is the scientific name or just its
// genus, ignoring case and surrounding space.
func namesTheSpecies(credit, scientificName string) bool {
	credit = strings.TrimSpace(credit)
	if credit == "" || scientificName == "" {
		return false
	}

	genus, _, _ := strings.Cut(strings.TrimSpace(scientificName), " ")
	return strings.EqualFold(credit, genus) || strings.EqualFold(credit, strings.TrimSpace(scientificName))
}

// clues are the hints shown with the photograph. The species and the place
// are deliberately missing: working those out is the game.
func clues(s sighting.Sighting) []Clue {
	group := Clue{Label: "Group", Value: Taxon(s)}
	if latin := taxonLatin(s); latin != "" {
		group.Title = latin
	}

	out := []Clue{
		{Label: "Recorded", Value: s.HappenedAt.Format("January 2006")},
		group,
	}

	if s.Weather.Temperature.Actual != 0 {
		out = append(out, Clue{Label: "That day", Value: fmt.Sprintf("%.0f°C, %s", s.Weather.Temperature.Actual, condition(s))})
	}

	if s.IndividualCount != nil && *s.IndividualCount > 1 {
		out = append(out, Clue{Label: "Seen", Value: strconv.Itoa(*s.IndividualCount) + " together"})
	}

	return out
}

// groups are the taxonomic classes in the record, in the words a player
// would use. A class says what kind of animal it is, but only to someone
// who already knows what a class is.
var groups = map[string]string{
	"Mammalia":       "Mammals",
	"Aves":           "Birds",
	"Testudines":     "Turtles",
	"Squamata":       "Lizards and snakes",
	"Crocodylia":     "Crocodiles",
	"Elasmobranchii": "Sharks and rays",
}

// Taxon is the broadest name for what kind of animal it is, which hints
// without naming the species. It is plain English where the class is one
// the record is known to hold, and the Latin as recorded otherwise.
func Taxon(s sighting.Sighting) string {
	if s.Species.Class != "" {
		if name, ok := groups[s.Species.Class]; ok {
			return name
		}
		return s.Species.Class
	}
	if s.Species.Phylum != "" {
		return s.Species.Phylum
	}
	return "Unrecorded"
}

// taxonLatin is the recorded class behind a plain-English group name, so a
// player who wants the Latin can hover for it. It is empty when Taxon
// already showed the Latin.
func taxonLatin(s sighting.Sighting) string {
	if _, ok := groups[s.Species.Class]; ok {
		return s.Species.Class
	}
	return ""
}

// condition is the weather in words, lowercased to sit in a sentence.
//
// It describes the stored code, not the stored description: the description
// came from the wrong WMO table (see weather.Describe).
func condition(s sighting.Sighting) string {
	phrase := weather.Describe(s.Weather.Condition.Code)

	runes := []rune(phrase)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// played returns the sighting round i asked about, and false when it is no
// longer there to name.
func (p OverProps) played(i int) (sighting.Sighting, bool) {
	if i < 0 || i >= len(p.Game.SightingIDs) {
		return sighting.Sighting{}, false
	}

	s, ok := p.Sightings[p.Game.SightingIDs[i]]
	return s, ok
}

// playedName names the animal in round i, or an empty string when that
// sighting is no longer there to name.
func (p OverProps) playedName(i int) string {
	s, ok := p.played(i)
	if !ok {
		return ""
	}
	return speciesName(s)
}

// playedPlace is where round i's animal really was.
func (p OverProps) playedPlace(i int) string {
	s, ok := p.played(i)
	if !ok {
		return ""
	}
	return place(s.Location)
}

// roundLabel names the round being shown. The caller says which, because
// the answer page is headed by the round just played while the question
// page is headed by the one being asked.
func roundLabel(n int) string {
	return fmt.Sprintf("Round %d of %d", min(max(n, 1), game.Rounds), game.Rounds)
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

// tone names how a round went, so the colour the page uses agrees with
// the distance and the medal already stating it rather than being the only
// thing that says so.
func tone(points int) string {
	switch {
	case points >= 4000:
		return "is-near"
	case points >= 1500:
		return "is-mid"
	default:
		return "is-far"
	}
}

// gameTone is the same judgement over a whole game, by what an average
// round of it was worth.
func gameTone(g game.Game) string {
	return tone(g.Total() * game.MaxRoundScore / game.MaxScore)
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

// barScale is a result's points as a fraction of a round's maximum, which
// the bar under it is scaled by.
func barScale(points int) string {
	return fmt.Sprintf("%.3f", float64(points)/float64(game.MaxRoundScore))
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
