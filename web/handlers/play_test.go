package handlers_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"uuid"
	"workshop/internal/common/logs"
	"workshop/internal/domain/game"
	"workshop/internal/domain/sighting"
	"workshop/web/handlers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGames stands in for the game service, so the handlers can be driven
// without a database behind them.
type fakeGames struct {
	sightings []sighting.Sighting
	newErr    error
	roundErr  error
}

func (f fakeGames) New(context.Context) (game.Game, error) {
	if f.newErr != nil {
		return game.Game{}, f.newErr
	}

	ids := make([]uuid.UUID, 0, len(f.sightings))
	for _, s := range f.sightings {
		ids = append(ids, s.ID)
	}

	return game.New(ids), nil
}

func (f fakeGames) Round(_ context.Context, g game.Game) (sighting.Sighting, error) {
	if f.roundErr != nil {
		return sighting.Sighting{}, f.roundErr
	}

	id, ok := g.Current()
	if !ok {
		return sighting.Sighting{}, game.ErrOver
	}

	for _, s := range f.sightings {
		if s.ID == id {
			return s, nil
		}
	}

	return sighting.Sighting{}, sighting.ErrNotFound
}

func (f fakeGames) Played(_ context.Context, g game.Game) (map[uuid.UUID]sighting.Sighting, error) {
	out := make(map[uuid.UUID]sighting.Sighting, len(g.SightingIDs))
	for _, s := range f.sightings {
		out[s.ID] = s
	}
	return out, nil
}

// gameOf returns a fake service holding Rounds sightings off Nairobi.
func gameOf() fakeGames {
	out := make([]sighting.Sighting, 0, game.Rounds)
	for range game.Rounds {
		out = append(out, sighting.Sighting{
			ID:       uuid.New(),
			Location: sighting.Location{Coordinates: sighting.Coordinates{Latitude: -1.29, Longitude: 36.82}},
		})
	}
	return fakeGames{sightings: out}
}

func quietLogger() *slog.Logger {
	return logs.NewProduction(io.Discard, slog.LevelError)
}

// begin starts a game and returns the cookie carrying it.
func begin(t *testing.T, session handlers.Session, games handlers.GameService) *http.Cookie {
	t.Helper()

	rec := httptest.NewRecorder()
	handlers.Play(quietLogger(), games, session)(rec, httptest.NewRequest(http.MethodGet, "/play", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1, "starting a game should set the game cookie")

	return cookies[0]
}

// guess posts a guess for round, carrying cookie.
func guess(t *testing.T, session handlers.Session, games handlers.GameService, cookie *http.Cookie, round, lat, lng string) *httptest.ResponseRecorder {
	t.Helper()

	form := url.Values{"round": {round}, "lat": {lat}, "lng": {lng}}
	r := httptest.NewRequest(http.MethodPost, "/play/guess", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		r.AddCookie(cookie)
	}

	rec := httptest.NewRecorder()
	handlers.Guess(quietLogger(), games, session)(rec, r)

	return rec
}

// gameIn returns the game a response's cookie carries.
func gameIn(t *testing.T, session handlers.Session, rec *httptest.ResponseRecorder) game.Game {
	t.Helper()

	cookies := rec.Result().Cookies()
	require.NotEmpty(t, cookies)

	r := httptest.NewRequest(http.MethodGet, "/play", nil)
	r.AddCookie(cookies[0])

	g, err := session.Read(r)
	require.NoError(t, err)

	return g
}

func TestPlay(t *testing.T) {
	t.Parallel()

	t.Run("Starts a game for a player who arrives without one", func(t *testing.T) {
		t.Parallel()

		session := handlers.NewSession()
		cookie := begin(t, session, gameOf())
		assert.NotEmpty(t, cookie.Value)
	})

	t.Run("Explains an empty database rather than failing", func(t *testing.T) {
		t.Parallel()

		games := fakeGames{newErr: game.ErrNotEnoughSightings}

		rec := httptest.NewRecorder()
		handlers.Play(quietLogger(), games, handlers.NewSession())(rec, httptest.NewRequest(http.MethodGet, "/play", nil))

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Contains(t, rec.Body.String(), "db seed")
	})
}

func TestGuess(t *testing.T) {
	t.Parallel()

	t.Run("Scores a guess and advances the round", func(t *testing.T) {
		t.Parallel()

		session, games := handlers.NewSession(), gameOf()
		cookie := begin(t, session, games)

		rec := guess(t, session, games, cookie, "0", "-1.29", "36.82")
		require.Equal(t, http.StatusOK, rec.Code)

		g := gameIn(t, session, rec)
		require.Len(t, g.Results, 1)
		assert.Equal(t, 1, g.Round)
		assert.Equal(t, game.MaxRoundScore, g.Results[0].Points, "a guess on the answer takes the round")
	})

	t.Run("Ignores a resubmitted round", func(t *testing.T) {
		t.Parallel()

		session, games := handlers.NewSession(), gameOf()
		cookie := begin(t, session, games)

		scored := guess(t, session, games, cookie, "0", "-1.29", "36.82")
		require.Equal(t, http.StatusOK, scored.Code)
		advanced := scored.Result().Cookies()[0]

		// The same form again, with the cookie as it now stands.
		again := guess(t, session, games, advanced, "0", "51.5", "-0.12")
		assert.Equal(t, http.StatusSeeOther, again.Code)
		assert.Equal(t, "/play", again.Header().Get("Location"))

		g := gameIn(t, session, scored)
		assert.Len(t, g.Results, 1, "the round must not be scored twice")
	})

	t.Run("Returns the player to their round when the guess is not a position", func(t *testing.T) {
		t.Parallel()

		session, games := handlers.NewSession(), gameOf()
		cookie := begin(t, session, games)

		// A typed guess can be anything, so a bad one has to land the player
		// back on their round rather than on a dead-end error page.
		for name, pos := range map[string][2]string{
			"Not a number":        {"not-a-number", "36.82"},
			"NaN":                 {"NaN", "NaN"},
			"Latitude off world":  {"999", "0"},
			"Longitude off world": {"0", "999"},
			"Infinite":            {"Inf", "0"},
			"Empty":               {"", ""},
		} {
			t.Run(name, func(t *testing.T) {
				rec := guess(t, session, games, cookie, "0", pos[0], pos[1])

				assert.Equal(t, http.StatusSeeOther, rec.Code)
				assert.Equal(t, "/play", rec.Header().Get("Location"))
				assert.Empty(t, rec.Result().Cookies(), "the round is unscored, so the game must be left as it was")
			})
		}
	})

	t.Run("Sends a player with no game to start one", func(t *testing.T) {
		t.Parallel()

		rec := guess(t, handlers.NewSession(), gameOf(), nil, "0", "-1.29", "36.82")

		assert.Equal(t, http.StatusSeeOther, rec.Code)
		assert.Equal(t, "/play/new", rec.Header().Get("Location"))
	})
}

// finishedGame plays every round of games and returns the cookie holding
// the finished game.
func finishedGame(t *testing.T, session handlers.Session, games fakeGames) *http.Cookie {
	t.Helper()

	cookie := begin(t, session, games)
	for round := range game.Rounds {
		rec := guess(t, session, games, cookie, strconv.Itoa(round), "-1.29", "36.82")
		require.Equal(t, http.StatusOK, rec.Code)
		cookie = rec.Result().Cookies()[0]
	}
	return cookie
}

func TestPlay_SharesTheAddressItWasReachedAt(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		header map[string]string
		want   string
	}{
		"plain http":              {want: "http://safari.test/play"},
		"behind a TLS terminator": {header: map[string]string{"X-Forwarded-Proto": "https"}, want: "https://safari.test/play"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			session, games := handlers.NewSession(), gameOf()
			cookie := finishedGame(t, session, games)

			r := httptest.NewRequest(http.MethodGet, "http://safari.test/play", nil)
			r.AddCookie(cookie)
			for k, v := range test.header {
				r.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			handlers.Play(quietLogger(), games, session)(rec, r)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), test.want, "the share line should carry the host the player used, not a hardcoded one")
		})
	}
}
