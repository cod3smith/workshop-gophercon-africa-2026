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
	"time"
	"uuid"
	"workshop/internal/common/logs"
	"workshop/internal/domain/game"
	"workshop/internal/domain/sighting"
	"workshop/internal/domain/species"
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
	handlers.Play(quietLogger(), games, session, silence{})(rec, httptest.NewRequest(http.MethodGet, "/play", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1, "starting a game should set the game cookie")

	return cookies[0]
}

// guess posts a guess for round, carrying cookie, to a game with no sounds.
func guess(t *testing.T, session handlers.Session, games handlers.GameService, cookie *http.Cookie, round, lat, lng string) *httptest.ResponseRecorder {
	t.Helper()

	return guessHearing(t, session, games, silence{}, cookie, round, lat, lng)
}

// silence is a sound service that has no recording of anything.
type silence struct{}

func (silence) Sound(context.Context, string) (species.Recording, bool) {
	return species.Recording{}, false
}

func (silence) Warm(context.Context, string) {}

// guessHearing posts a guess for round, carrying cookie, with sounds to
// consult for the reveal.
func guessHearing(t *testing.T, session handlers.Session, games handlers.GameService, sounds handlers.SoundService, cookie *http.Cookie, round, lat, lng string) *httptest.ResponseRecorder {
	t.Helper()

	form := url.Values{"round": {round}, "lat": {lat}, "lng": {lng}}
	r := httptest.NewRequest(http.MethodPost, "/play/guess", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		r.AddCookie(cookie)
	}

	rec := httptest.NewRecorder()
	handlers.Guess(quietLogger(), games, session, sounds)(rec, r)

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
		handlers.Play(quietLogger(), games, handlers.NewSession(), silence{})(rec, httptest.NewRequest(http.MethodGet, "/play", nil))

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
			handlers.Play(quietLogger(), games, session, silence{})(rec, r)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), test.want, "the share line should carry the host the player used, not a hardcoded one")
		})
	}
}

// soundsFor is a sound service that knows one species and records what it
// was asked, and under what deadline.
type soundsFor struct {
	name      string
	recording species.Recording
	block     bool

	asked    string
	warmed   []string
	deadline time.Time
	hasLimit bool
}

func (s *soundsFor) Warm(_ context.Context, name string) {
	s.warmed = append(s.warmed, name)
}

func (s *soundsFor) Sound(ctx context.Context, name string) (species.Recording, bool) {
	s.asked = name
	s.deadline, s.hasLimit = ctx.Deadline()

	if s.block {
		<-ctx.Done()
		return species.Recording{}, false
	}
	return s.recording, name == s.name
}

// gameOfLions is gameOf, with a species the reveal can name.
func gameOfLions() fakeGames {
	games := gameOf()
	for i := range games.sightings {
		games.sightings[i].Species = species.Species{
			ScientificName: "Panthera leo (Linnaeus, 1758)",
			CanonicalName:  "Panthera leo",
			VernacularName: "lion",
		}
	}
	return games
}

var lionRoar = species.Recording{
	URL:         "https://static.inaturalist.org/sounds/2160774.wav?1789563327",
	ContentType: "audio/x-wav",
	Licence:     "cc-by-nc",
	Attribution: "(c) Africa Gomez, some rights reserved (CC BY-NC)",
}

func TestGuess_TheRevealOffersTheSoundWhenThereIsOne(t *testing.T) {
	t.Parallel()

	session, games := handlers.NewSession(), gameOfLions()
	cookie := begin(t, session, games)

	sounds := &soundsFor{name: "Panthera leo", recording: lionRoar}
	rec := guessHearing(t, session, games, sounds, cookie, "0", "-1.29", "36.82")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "Panthera leo", sounds.asked, "the species is looked up by its name without the authority")
	assert.Contains(t, rec.Body.String(), "<audio")
	assert.Contains(t, rec.Body.String(), "Africa Gomez")
}

func TestGuess_TheRevealIsSilentWhenThereIsNoRecording(t *testing.T) {
	t.Parallel()

	session, games := handlers.NewSession(), gameOfLions()
	cookie := begin(t, session, games)

	rec := guessHearing(t, session, games, &soundsFor{name: "Acinonyx jubatus", recording: lionRoar}, cookie, "0", "-1.29", "36.82")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "<audio")
}

func TestGuess_RunsWithoutASoundService(t *testing.T) {
	t.Parallel()

	session, games := handlers.NewSession(), gameOfLions()
	cookie := begin(t, session, games)

	rec := guessHearing(t, session, games, nil, cookie, "0", "-1.29", "36.82")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "<audio")
}

// A slow iNaturalist must not hold up the reveal, and a reveal that gave up
// waiting reads exactly as one for a species with no recording.
func TestGuess_GivesTheSoundLookupItsOwnShortDeadline(t *testing.T) {
	t.Parallel()

	session, games := handlers.NewSession(), gameOfLions()
	cookie := begin(t, session, games)

	sounds := &soundsFor{block: true}
	start := time.Now()
	rec := guessHearing(t, session, games, sounds, cookie, "0", "-1.29", "36.82")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, sounds.hasLimit, "the lookup is not allowed to wait for as long as the request does")
	assert.LessOrEqual(t, time.Until(sounds.deadline), 3*time.Second, "a second or two, not the transport's thirty")
	assert.Less(t, time.Since(start), 10*time.Second)
	assert.NotContains(t, rec.Body.String(), "<audio")
	assert.Contains(t, rec.Body.String(), "reveal__name", "the guess is still answered")
}

// The question is on screen for a while before the guess, and finding a
// recording takes iNaturalist several seconds, so the lookup is started
// when the question is shown and is ready for the reveal. Nothing about it
// reaches the page.
func TestPlay_StartsLookingForTheSoundWhileTheQuestionIsOnScreen(t *testing.T) {
	t.Parallel()

	session, games := handlers.NewSession(), gameOfLions()
	sounds := &soundsFor{name: "Panthera leo", recording: lionRoar}

	rec := httptest.NewRecorder()
	handlers.Play(quietLogger(), games, session, sounds)(rec, httptest.NewRequest(http.MethodGet, "/play", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"Panthera leo"}, sounds.warmed)
	assert.NotContains(t, rec.Body.String(), "<audio", "the question must stay silent")
	assert.NotContains(t, rec.Body.String(), "Panthera", "and name nothing")
}
