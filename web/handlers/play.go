package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"
	"uuid"
	"workshop/internal/domain/game"
	"workshop/internal/domain/sighting"
	"workshop/internal/domain/species"
	"workshop/web/views/pages"
)

// GameService is what the play handlers need from the game domain.
type GameService interface {
	New(ctx context.Context) (game.Game, error)
	Round(ctx context.Context, g game.Game) (sighting.Sighting, error)
	Played(ctx context.Context, g game.Game) (map[uuid.UUID]sighting.Sighting, error)
}

// SoundService is what the reveal needs to offer the animal's sound. Sound
// answers false for a species with no recording, and for a lookup that
// failed or ran out of time: the reveal reads the same either way. Warm
// starts a lookup without waiting for it.
type SoundService interface {
	Sound(ctx context.Context, scientificName string) (species.Recording, bool)
	Warm(ctx context.Context, scientificName string)
}

// soundTimeout is how long the reveal waits to learn whether there is a
// recording. The lookup is a call to somebody else's server, and the
// player has a guess to be told about, so a slow one is given up on.
const soundTimeout = 1500 * time.Millisecond

// Play renders the round the player is on, and starts them a game when
// they arrive without one. While the question is on screen it also starts
// finding the animal's recording, so the reveal has it ready; nothing of
// that is on the question itself.
func Play(logger *slog.Logger, games GameService, session Session, sounds SoundService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		g, err := session.Read(r)
		if err != nil {
			if g, err = start(ctx, w, games, session); err != nil {
				startFailed(ctx, w, logger, err)
				return
			}
		}

		if g.Complete() {
			// The animals are only named once the game is over, so this is
			// the first time they can be shown together.
			played, err := games.Played(ctx, g)
			if err != nil {
				// The score and the distances are all in the cookie, so the
				// screen is still worth showing without the animals' names.
				logger.ErrorContext(ctx, "handlers: reading the finished game: "+err.Error())
				played = nil
			}

			render(ctx, w, logger, pages.Over(pages.OverProps{Game: g, Sightings: played, URL: shareURL(r)}))
			return
		}

		sight, err := games.Round(ctx, g)
		if errors.Is(err, sighting.ErrNotFound) {
			// The sighting this round asks about has gone, which a reseeded
			// database does, so the player gets a new game rather than an
			// error page about a row they never knew existed.
			logger.WarnContext(ctx, "handlers: the round's sighting has gone, starting again")
			http.Redirect(w, r, "/play/new", http.StatusSeeOther)
			return
		} else if err != nil {
			// Anything else is the store being unwell, and throwing the
			// player's game away would be the wrong way to say so.
			logger.ErrorContext(ctx, "handlers: reading the round: "+err.Error())
			renderError(ctx, w, logger, http.StatusInternalServerError, "Unable to load this round.")
			return
		}

		warm(ctx, sounds, sight)

		render(ctx, w, logger, pages.Question(pages.QuestionProps{Game: g, Sighting: sight}))
	}
}

// shareURL is where a shared result sends people: the game, on the host the
// player reached it by. The scheme comes from the proxy's header when there
// is one, because a TLS terminator in front of the app makes every request
// look like plain http to it. Both values are only ever echoed back to the
// player who sent them.
func shareURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/play"
}

// NewGame throws away whatever the player had and starts them a fresh game.
func NewGame(logger *slog.Logger, games GameService, session Session) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if _, err := start(ctx, w, games, session); err != nil {
			startFailed(ctx, w, logger, err)
			return
		}

		http.Redirect(w, r, "/play", http.StatusSeeOther)
	}
}

// Guess scores the player's guess against where the animal was really seen,
// and, where a recording of it exists, lets them hear it. A nil sounds is a
// game with none.
func Guess(logger *slog.Logger, games GameService, session Session, sounds SoundService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		g, err := session.Read(r)
		if err != nil {
			http.Redirect(w, r, "/play/new", http.StatusSeeOther)
			return
		}

		if err = r.ParseForm(); err != nil {
			renderError(ctx, w, logger, http.StatusBadRequest, "That guess didn't arrive in one piece.")
			return
		}

		// The round the form was rendered for. A guess for a round already
		// played is a resubmitted form, which would otherwise score the
		// next photograph with the last one's answer.
		if round, _ := strconv.Atoi(r.PostFormValue("round")); round != g.Round {
			http.Redirect(w, r, "/play", http.StatusSeeOther)
			return
		}

		guess, err := coordinates(r.PostFormValue("lat"), r.PostFormValue("lng"))
		if err != nil {
			// The fields can be typed into, so this is a slip a person makes
			// rather than a request nobody should send. The round is unscored
			// and the cookie untouched, so back to it they go with their game.
			http.Redirect(w, r, "/play", http.StatusSeeOther)
			return
		}

		sight, err := games.Round(ctx, g)
		if errors.Is(err, sighting.ErrNotFound) {
			logger.WarnContext(ctx, "handlers: the round's sighting has gone, starting again")
			http.Redirect(w, r, "/play/new", http.StatusSeeOther)
			return
		} else if err != nil {
			logger.ErrorContext(ctx, "handlers: reading the round: "+err.Error())
			renderError(ctx, w, logger, http.StatusInternalServerError, "Unable to score this round.")
			return
		}

		result := game.Score(guess, sight.Location.Coordinates)
		g.Record(result)

		if err = session.Write(w, g); err != nil {
			logger.ErrorContext(ctx, "handlers: saving the game: "+err.Error())
			renderError(ctx, w, logger, http.StatusInternalServerError, "Unable to save your game.")
			return
		}

		render(ctx, w, logger, pages.Answer(pages.AnswerProps{
			Game:     g,
			Sighting: sight,
			Guess:    guess,
			Result:   result,
			// Only now, with the guess in and the cookie written: the sound
			// names the animal, so it is looked for after the round is
			// scored and never on the question.
			Recording: recording(ctx, sounds, sight, soundTimeout),
		}))
	}
}

// soundName is the name a recording is looked up by. The canonical name is
// the one iNaturalist knows the species by; the scientific name carries
// the authority ("Panthera leo (Linnaeus, 1758)").
func soundName(s sighting.Sighting) string {
	if s.Species.CanonicalName != "" {
		return s.Species.CanonicalName
	}
	return s.Species.ScientificName
}

// warm starts finding the sound of the animal in sight, for the reveal
// that follows the question. It never waits, and never reaches the page.
func warm(ctx context.Context, sounds SoundService, s sighting.Sighting) {
	if sounds == nil {
		return
	}
	if name := soundName(s); name != "" {
		sounds.Warm(ctx, name)
	}
}

// recording finds the sound of the animal in sight, or returns nil. It
// waits at most wait, so a slow service cannot hold the page up, and
// nothing it does wrong reaches the player.
func recording(ctx context.Context, sounds SoundService, s sighting.Sighting, wait time.Duration) *species.Recording {
	if sounds == nil {
		return nil
	}

	name := soundName(s)
	if name == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	found, ok := sounds.Sound(ctx, name)
	if !ok {
		return nil
	}
	return &found
}

// startFailed explains why a game couldn't begin. An empty database is the
// likely cause on a fresh clone, and it has a fix the player can run.
func startFailed(ctx context.Context, w http.ResponseWriter, logger *slog.Logger, err error) {
	if errors.Is(err, game.ErrNotEnoughSightings) {
		logger.WarnContext(ctx, "handlers: no sightings to play with")
		renderError(ctx, w, logger, http.StatusServiceUnavailable,
			"There are no sightings to play with yet. Run 'go run . db seed' and reload this page.")
		return
	}

	logger.ErrorContext(ctx, "handlers: starting a game: "+err.Error())
	renderError(ctx, w, logger, http.StatusInternalServerError, "Unable to start a game.")
}

// start draws a new game and puts it on the response.
func start(ctx context.Context, w http.ResponseWriter, games GameService, session Session) (game.Game, error) {
	g, err := games.New(ctx)
	if err != nil {
		return game.Game{}, err
	}

	return g, session.Write(w, g)
}

// coordinates parses a guess the form arrived with.
func coordinates(lat, lng string) (sighting.Coordinates, error) {
	latitude, err := strconv.ParseFloat(lat, 64)
	if err != nil {
		return sighting.Coordinates{}, err
	}

	longitude, err := strconv.ParseFloat(lng, 64)
	if err != nil {
		return sighting.Coordinates{}, err
	}

	// The domain decides whether those numbers are a place on earth.
	return sighting.ParseCoordinates(latitude, longitude)
}
