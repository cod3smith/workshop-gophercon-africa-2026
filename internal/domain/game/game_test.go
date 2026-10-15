package game_test

import (
	"math"
	"testing"
	"uuid"
	"workshop/internal/domain/game"
	"workshop/internal/domain/sighting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// at is a Coordinates built from numbers the test knows are in range.
func at(t *testing.T, lat, lng float64) sighting.Coordinates {
	t.Helper()

	c, err := sighting.ParseCoordinates(lat, lng)
	require.NoError(t, err)
	return c
}

func TestDistance(t *testing.T) {
	var (
		london    = at(t, 51.5074, -0.1278)
		paris     = at(t, 48.8566, 2.3522)
		nullIsle  = at(t, 0, 0)
		quarter   = at(t, 0, 90)
		northPole = at(t, 90, 0)
		southPole = at(t, -90, 0)
	)

	tests := map[string]struct {
		a, b   sighting.Coordinates
		want   float64
		within float64
	}{
		"Same place is zero":       {london, london, 0, 0.001},
		"London to Paris":          {london, paris, 344, 5},
		"Paris to London is same":  {paris, london, 344, 5},
		"A quarter of the equator": {nullIsle, quarter, 10018, 20},
		"Pole to pole":             {northPole, southPole, 20015, 20},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.InDelta(t, test.want, game.Distance(test.a, test.b), test.within)
		})
	}
}

func TestScoreFor(t *testing.T) {
	t.Run("A perfect guess scores the maximum", func(t *testing.T) {
		assert.Equal(t, game.MaxRoundScore, game.ScoreFor(0))
	})

	t.Run("Never negative, however far out", func(t *testing.T) {
		assert.GreaterOrEqual(t, game.ScoreFor(20015), 0)
	})

	t.Run("Never over the maximum", func(t *testing.T) {
		assert.LessOrEqual(t, game.ScoreFor(0), game.MaxRoundScore)
	})

	t.Run("Falls as the guess gets worse", func(t *testing.T) {
		last := math.MaxInt
		for _, km := range []float64{0, 10, 100, 500, 1500, 5000, 15000} {
			got := game.ScoreFor(km)
			assert.Less(t, got, last, "score at %vkm should beat the one before", km)
			last = got
		}
	})
}

func TestScore(t *testing.T) {
	answer := at(t, -1.2921, 36.8219)

	t.Run("Reports the distance and the points together", func(t *testing.T) {
		got := game.Score(answer, answer)
		assert.InDelta(t, 0, got.DistanceKm, 0.001)
		assert.Equal(t, game.MaxRoundScore, got.Points)
	})
}

// newGame returns a game over Rounds throwaway sightings.
func newGame() game.Game {
	ids := make([]uuid.UUID, game.Rounds)
	for i := range ids {
		ids[i] = uuid.New()
	}
	return game.New(ids)
}

func TestGame(t *testing.T) {
	t.Run("Starts on the first round with nothing scored", func(t *testing.T) {
		g := newGame()

		assert.Equal(t, 0, g.Round)
		assert.Equal(t, 0, g.Total())
		assert.False(t, g.Complete())

		id, ok := g.Current()
		assert.True(t, ok)
		assert.Equal(t, g.SightingIDs[0], id)
	})

	t.Run("Recording a round advances it and adds the points", func(t *testing.T) {
		g := newGame()
		g.Record(game.Result{DistanceKm: 10, Points: 1000})
		g.Record(game.Result{DistanceKm: 20, Points: 500})

		assert.Equal(t, 2, g.Round)
		assert.Equal(t, 1500, g.Total())
		assert.Len(t, g.Results, 2)

		id, ok := g.Current()
		assert.True(t, ok)
		assert.Equal(t, g.SightingIDs[2], id)
	})

	t.Run("Is complete once every round is scored", func(t *testing.T) {
		g := newGame()
		for range game.Rounds {
			g.Record(game.Result{Points: 100})
		}

		assert.True(t, g.Complete())
		assert.Equal(t, game.Rounds*100, g.Total())

		_, ok := g.Current()
		assert.False(t, ok, "a finished game has no round to play")
	})

	t.Run("Recording past the last round is ignored", func(t *testing.T) {
		g := newGame()
		for range game.Rounds + 3 {
			g.Record(game.Result{Points: 100})
		}

		assert.Len(t, g.Results, game.Rounds)
		assert.Equal(t, game.Rounds*100, g.Total())
	})

	t.Run("A game with no sightings is complete and unplayable", func(t *testing.T) {
		g := game.New(nil)

		assert.True(t, g.Complete())
		_, ok := g.Current()
		assert.False(t, ok)
	})
}

func TestGame_Share(t *testing.T) {
	g := newGame()
	for range game.Rounds {
		g.Record(game.Result{Points: game.MaxRoundScore})
	}

	share := g.Share()

	assert.Contains(t, share, "25,000", "the share line carries the total, grouped")
	assert.Contains(t, share, game.Name)
	assert.Contains(t, share, g.Rank())
	for _, r := range g.Results {
		assert.NotEmpty(t, game.Medal(r.Points))
	}
}

func TestGame_Rank(t *testing.T) {
	t.Run("Every total has a rank", func(t *testing.T) {
		for _, points := range []int{0, 1, 6000, 12000, 18000, 22500, game.Rounds * game.MaxRoundScore} {
			g := game.Game{Results: []game.Result{{Points: points}}}
			assert.NotEmpty(t, g.Rank(), "no rank for %d points", points)
		}
	})

	t.Run("A better total never ranks lower", func(t *testing.T) {
		perfect := game.Game{Results: []game.Result{{Points: game.Rounds * game.MaxRoundScore}}}
		hopeless := game.Game{Results: []game.Result{{Points: 0}}}
		assert.NotEqual(t, hopeless.Rank(), perfect.Rank())
	})
}
