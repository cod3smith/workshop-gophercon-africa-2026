// Package game is the guessing game played over recorded sightings: the
// player is shown a photograph of an animal and says where on earth they
// think it was seen, and the game says how close they were.
//
// Nothing in here knows about HTTP, cookies or SQL. A game is a value, so
// it can be handed to a player, stored however the caller likes and scored
// again later with the same answer.
package game

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"uuid"
	"workshop/internal/domain/sighting"
)

const (
	// Rounds is how many sightings one game asks about.
	Rounds = 5

	// MaxRoundScore is the most a single round can award, which is what a
	// guess on top of the answer earns.
	MaxRoundScore = 5000

	// MaxScore is the most a whole game can award.
	MaxScore = Rounds * MaxRoundScore

	// Name is what the game calls itself, on the page and in a shared result.
	Name = "Sightings Safari"
)

// earthRadiusKm is the mean radius of the earth, which is the sphere the
// distance between two guesses is measured on.
const earthRadiusKm = 6371.0088

// decayKm is how quickly the points fall away from the answer: the
// distance at which a guess keeps about a third of the round's points,
// which is gentle enough that a player who knows the continent still
// scores. It is the curve's e-folding distance, not its half-life, which
// falls around 1,040km.
const decayKm = 1500

// Result is one round, once the player has guessed: how far out they were
// and what it earned them.
type Result struct {
	DistanceKm float64 `json:"distanceKm"`
	Points     int     `json:"points"`

	// Guess is where the player put their pin, kept so the finished game
	// can show all five guesses beside the five answers. It is nil on a
	// result recorded without one, such as a game begun before guesses
	// were kept, which the ending draws without a guess rather than with
	// one at 0,0.
	Guess *sighting.Coordinates `json:"guess,omitempty"`
}

// Game is a run of Rounds sightings and how the player has done so far.
// The sightings are chosen up front so the game can be put down and picked
// up again without the answers changing underneath the player.
type Game struct {
	SightingIDs []uuid.UUID `json:"sightingIDs"`
	Round       int         `json:"round"`
	Results     []Result    `json:"results"`
}

// New returns a game that will ask about the given sightings, in order.
func New(sightingIDs []uuid.UUID) Game {
	return Game{SightingIDs: sightingIDs}
}

// Current returns the sighting the player is being asked about, and false
// once there are no rounds left to play.
func (g Game) Current() (uuid.UUID, bool) {
	if g.Round < 0 || g.Round >= len(g.SightingIDs) {
		return uuid.Nil(), false
	}
	return g.SightingIDs[g.Round], true
}

// Complete reports whether every round has been scored.
func (g Game) Complete() bool {
	return g.Round >= len(g.SightingIDs)
}

// Total is the points scored across every round so far.
func (g Game) Total() int {
	var total int
	for _, r := range g.Results {
		total += r.Points
	}
	return total
}

// Best is the round the player did best at, and its zero-based index, so
// the ending can name a good moment rather than only a total. When rounds
// tie the earliest wins. It is false when no round has been scored.
func (g Game) Best() (Result, int, bool) {
	if len(g.Results) == 0 {
		return Result{}, 0, false
	}

	best := 0
	for i, r := range g.Results {
		if r.Points > g.Results[best].Points {
			best = i
		}
	}
	return g.Results[best], best, true
}

// Record scores the round the player is on and moves to the next one.
// A game with nothing left to play is left alone, so a replayed form
// submission can't score a sixth round.
func (g *Game) Record(r Result) {
	if g.Complete() {
		return
	}

	g.Results = append(g.Results, r)
	g.Round++
}

// Score returns what a guess earns against the place the animal was
// actually seen.
func Score(guess, answer sighting.Coordinates) Result {
	km := Distance(guess, answer)
	return Result{DistanceKm: km, Points: ScoreFor(km), Guess: &guess}
}

// ScoreFor returns the points a guess that far from the answer earns.
// The points decay smoothly rather than in bands, so there is always
// something to be gained by guessing a little better.
func ScoreFor(distanceKm float64) int {
	if distanceKm <= 0 {
		return MaxRoundScore
	}

	points := int(math.Round(MaxRoundScore * math.Exp(-distanceKm/decayKm)))
	return min(max(points, 0), MaxRoundScore)
}

// Distance returns the great-circle distance in kilometres between two
// positions, by the haversine formula.
func Distance(a, b sighting.Coordinates) float64 {
	var (
		lat1 = radians(a.Latitude)
		lat2 = radians(b.Latitude)
		dLat = lat2 - lat1
		dLng = radians(b.Longitude - a.Longitude)
	)

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)

	// Rounding can push h a hair over 1 for antipodal points, and Asin of
	// anything over 1 is NaN, so it's clamped before the root.
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(min(h, 1)))
}

// radians converts degrees to radians.
func radians(degrees float64) float64 {
	return degrees * math.Pi / 180
}

// Rank is a title for the total, so a score means something without the
// player working out what 18,000 out of 25,000 was worth.
func (g Game) Rank() string {
	switch total := g.Total(); {
	case total >= 22500:
		return "Legendary Tracker"
	case total >= 18000:
		return "Safari Guide"
	case total >= 12000:
		return "Field Naturalist"
	case total >= 6000:
		return "Keen Amateur"
	case total >= 1500:
		return "Getting Warmer"
	default:
		return "Scenic Route Specialist"
	}
}

// Medal is the square that stands for a round's points in a shared result.
func Medal(points int) string {
	switch {
	case points >= 4000:
		return "🟩"
	case points >= 2500:
		return "🟨"
	case points >= 1000:
		return "🟧"
	default:
		return "⬛"
	}
}

// Share is the result as a few lines the player can paste anywhere: the
// score, then a square per round, which gives away how they did without
// giving away a single answer, then the address to play it at.
//
// The address is the point of sharing: pasted into a chat with no link, a
// result is something nobody who reads it can act on. It sits on its own
// line so a chat client links all of it and nothing else.
func (g Game) Share(url string) string {
	var squares strings.Builder
	for _, r := range g.Results {
		squares.WriteString(Medal(r.Points))
	}

	return fmt.Sprintf("%s %s/%s — %s\n%s\n%s", Name, group(g.Total()), group(MaxScore), g.Rank(), squares.String(), url)
}

// group puts separators in a number, so a score pasted into a chat reads
// the same way it did on the page.
func group(n int) string {
	digits := strconv.Itoa(n)
	if len(digits) <= 3 {
		return digits
	}

	var out []byte
	for i, digit := range []byte(digits) {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, digit)
	}

	return string(out)
}
