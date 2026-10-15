package game

import (
	"context"
	"errors"
	"math/rand/v2"
	"uuid"
	"workshop/internal/domain/sighting"
)

var (
	// ErrNotEnoughSightings represents an error where there aren't enough
	// sightings with a photograph and a position to fill a game.
	ErrNotEnoughSightings = errors.New("game: not enough sightings with a photograph and a position to play")

	// ErrOver represents an error where a round was asked for of a game
	// that has none left to play.
	ErrOver = errors.New("game: the game is over")
)

// SightingSource is what the game needs from the sighting domain: a pool
// to draw its rounds from, and a way to read back the one a round asks
// about. The sighting service satisfies it.
type SightingSource interface {
	List(ctx context.Context, filter sighting.ListFilter) ([]sighting.Sighting, error)
	Find(ctx context.Context, id uuid.UUID) (sighting.Sighting, error)
}

// Service starts games and reads back the sightings they ask about.
type Service struct {
	source SightingSource
}

// NewService returns a Service drawing its rounds from source.
func NewService(source SightingSource) *Service {
	return &Service{source: source}
}

// New starts a game over Rounds sightings drawn at random from every one
// that can be guessed at.
//
// The draw happens once, up front, so the answers are fixed before the
// player sees the first photograph.
func (s Service) New(ctx context.Context) (Game, error) {
	all, err := s.source.List(ctx, sighting.ListFilter{})
	if err != nil {
		return Game{}, err
	}

	ids := make([]uuid.UUID, 0, len(all))
	for _, sight := range all {
		if Playable(sight) {
			ids = append(ids, sight.ID)
		}
	}

	if len(ids) < Rounds {
		return Game{}, ErrNotEnoughSightings
	}

	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })

	return New(ids[:Rounds]), nil
}

// Round returns the sighting the game is currently asking about, or ErrOver
// when there are no rounds left.
func (s Service) Round(ctx context.Context, g Game) (sighting.Sighting, error) {
	id, ok := g.Current()
	if !ok {
		return sighting.Sighting{}, ErrOver
	}

	return s.source.Find(ctx, id)
}

// Played returns the sightings the game asked about, in the order they
// were asked, so a finished game can be shown back round by round. A
// sighting that has since gone is left out rather than failing the lot.
func (s Service) Played(ctx context.Context, g Game) ([]sighting.Sighting, error) {
	out := make([]sighting.Sighting, 0, len(g.SightingIDs))
	for _, id := range g.SightingIDs {
		sight, err := s.source.Find(ctx, id)
		if errors.Is(err, sighting.ErrNotFound) {
			continue
		} else if err != nil {
			return nil, err
		}

		out = append(out, sight)
	}

	return out, nil
}

// Playable reports whether a sighting can be guessed at. It needs a
// photograph for the player to look at and a position to measure their
// guess against; GBIF supplies neither reliably.
func Playable(s sighting.Sighting) bool {
	if _, ok := Photo(s); !ok {
		return false
	}

	return s.Location.Latitude != 0 || s.Location.Longitude != 0
}

// Photo returns the image to show for a sighting, and false when it has
// none. GBIF records sound and video alongside photographs, and a round
// with nothing to look at can't be played.
func Photo(s sighting.Sighting) (sighting.Media, bool) {
	for _, m := range s.Media {
		if m.Type == "StillImage" && m.URL != "" {
			return m, true
		}
	}

	return sighting.Media{}, false
}
