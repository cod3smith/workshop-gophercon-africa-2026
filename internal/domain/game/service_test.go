package game_test

import (
	"context"
	"errors"
	"testing"
	"uuid"
	"workshop/internal/domain/game"
	"workshop/internal/domain/sighting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sourceStub stands in for the sighting service.
type sourceStub struct {
	sightings []sighting.Sighting
	err       error
}

func (s sourceStub) List(context.Context, sighting.ListFilter) ([]sighting.Sighting, error) {
	return s.sightings, s.err
}

func (s sourceStub) Find(_ context.Context, id uuid.UUID) (sighting.Sighting, error) {
	for _, sight := range s.sightings {
		if sight.ID == id {
			return sight, nil
		}
	}
	return sighting.Sighting{}, sighting.ErrNotFound
}

// playable returns a sighting the game can ask about: a photograph to show
// and a position to measure a guess against.
func playable() sighting.Sighting {
	return sighting.Sighting{
		ID:       uuid.New(),
		Location: sighting.Location{Coordinates: sighting.Coordinates{Latitude: -1.29, Longitude: 36.82}},
		Media:    []sighting.Media{{Type: "StillImage", URL: "https://example.test/lion.jpg"}},
	}
}

// pool returns n playable sightings.
func pool(n int) []sighting.Sighting {
	out := make([]sighting.Sighting, 0, n)
	for range n {
		out = append(out, playable())
	}
	return out
}

func TestPlayable(t *testing.T) {
	noPhoto := playable()
	noPhoto.Media = nil

	soundOnly := playable()
	soundOnly.Media = []sighting.Media{{Type: "Sound", URL: "https://example.test/roar.mp3"}}

	noURL := playable()
	noURL.Media = []sighting.Media{{Type: "StillImage"}}

	noPosition := playable()
	noPosition.Location.Coordinates = sighting.Coordinates{}

	tests := map[string]struct {
		in   sighting.Sighting
		want bool
	}{
		"A photograph and a position":   {playable(), true},
		"No media at all":               {noPhoto, false},
		"Media, but nothing to look at": {soundOnly, false},
		"A photograph with no address":  {noURL, false},
		"No position to measure from":   {noPosition, false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, game.Playable(test.in))
		})
	}
}

func TestService_New(t *testing.T) {
	ctx := context.Background()

	t.Run("Draws a full game of distinct sightings", func(t *testing.T) {
		svc := game.NewService(sourceStub{sightings: pool(50)})

		g, err := svc.New(ctx)
		require.NoError(t, err)
		require.Len(t, g.SightingIDs, game.Rounds)

		seen := make(map[uuid.UUID]bool, game.Rounds)
		for _, id := range g.SightingIDs {
			assert.False(t, seen[id], "the same sighting came up twice")
			seen[id] = true
		}
	})

	t.Run("Leaves out sightings with nothing to show", func(t *testing.T) {
		unplayable := playable()
		unplayable.Media = nil

		svc := game.NewService(sourceStub{sightings: append(pool(game.Rounds), unplayable)})

		g, err := svc.New(ctx)
		require.NoError(t, err)
		assert.NotContains(t, g.SightingIDs, unplayable.ID)
	})

	t.Run("Refuses to start without enough to ask about", func(t *testing.T) {
		svc := game.NewService(sourceStub{sightings: pool(game.Rounds - 1)})

		_, err := svc.New(ctx)
		assert.ErrorIs(t, err, game.ErrNotEnoughSightings)
	})

	t.Run("Passes the store's error back", func(t *testing.T) {
		errInternal := errors.New("internal error")
		svc := game.NewService(sourceStub{err: errInternal})

		_, err := svc.New(ctx)
		assert.ErrorIs(t, err, errInternal)
	})
}

func TestService_Round(t *testing.T) {
	ctx := context.Background()
	sightings := pool(50)
	svc := game.NewService(sourceStub{sightings: sightings})

	t.Run("Returns the sighting the game is asking about", func(t *testing.T) {
		g, err := svc.New(ctx)
		require.NoError(t, err)

		id, ok := g.Current()
		require.True(t, ok)

		got, err := svc.Round(ctx, g)
		require.NoError(t, err)
		assert.Equal(t, id, got.ID)
	})

	t.Run("A finished game has no round to show", func(t *testing.T) {
		g, err := svc.New(ctx)
		require.NoError(t, err)
		for range game.Rounds {
			g.Record(game.Result{})
		}

		_, err = svc.Round(ctx, g)
		assert.ErrorIs(t, err, game.ErrOver)
	})
}

func TestPhoto(t *testing.T) {
	t.Run("Picks the first still image", func(t *testing.T) {
		s := playable()
		s.Media = []sighting.Media{
			{Type: "Sound", URL: "https://example.test/roar.mp3"},
			{Type: "StillImage", URL: "https://example.test/lion.jpg"},
		}

		got, ok := game.Photo(s)
		assert.True(t, ok)
		assert.Equal(t, "https://example.test/lion.jpg", got.URL)
	})

	t.Run("Says so when there is none", func(t *testing.T) {
		s := playable()
		s.Media = nil

		_, ok := game.Photo(s)
		assert.False(t, ok)
	})
}

func TestService_Played(t *testing.T) {
	ctx := context.Background()

	t.Run("Returns every sighting the game asked about", func(t *testing.T) {
		sightings := pool(50)
		svc := game.NewService(sourceStub{sightings: sightings})

		g, err := svc.New(ctx)
		require.NoError(t, err)

		played, err := svc.Played(ctx, g)
		require.NoError(t, err)
		assert.Len(t, played, game.Rounds)

		for _, id := range g.SightingIDs {
			assert.Equal(t, id, played[id].ID)
		}
	})

	t.Run("A sighting that has gone leaves the others on their own round", func(t *testing.T) {
		sightings := pool(50)
		svc := game.NewService(sourceStub{sightings: sightings})

		g, err := svc.New(ctx)
		require.NoError(t, err)

		// Drop the sighting the second round asks about, as a reseeded
		// database would.
		missing := g.SightingIDs[1]
		remaining := make([]sighting.Sighting, 0, len(sightings)-1)
		for _, s := range sightings {
			if s.ID != missing {
				remaining = append(remaining, s)
			}
		}

		played, err := game.NewService(sourceStub{sightings: remaining}).Played(ctx, g)
		require.NoError(t, err)

		_, found := played[missing]
		assert.False(t, found, "the missing sighting should be absent")

		// Every other round still resolves to the sighting it was drawn
		// against, rather than shifting up into the gap.
		for i, id := range g.SightingIDs {
			if i == 1 {
				continue
			}
			assert.Equal(t, id, played[id].ID, "round %d moved onto another sighting", i+1)
		}
	})
}
