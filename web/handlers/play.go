package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"workshop/internal/domain/game"
	"workshop/internal/domain/sighting"
	"workshop/web/views/pages"
)

// GameService is what the play handlers need from the game domain.
type GameService interface {
	New(ctx context.Context) (game.Game, error)
	Round(ctx context.Context, g game.Game) (sighting.Sighting, error)
	Played(ctx context.Context, g game.Game) ([]sighting.Sighting, error)
}

// Play renders the round the player is on, and starts them a game when
// they arrive without one.
func Play(logger *slog.Logger, games GameService, session Session) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		g, err := session.Read(r)
		if err != nil {
			if g, err = start(ctx, w, games, session); err != nil {
				logger.ErrorContext(ctx, "handlers: starting a game: "+err.Error())
				renderError(ctx, w, logger, http.StatusInternalServerError, "Unable to start a game.")
				return
			}
		}

		if g.Complete() {
			// The animals are only named once the game is over, so this is
			// the first time they can be shown together.
			played, err := games.Played(ctx, g)
			if err != nil {
				logger.ErrorContext(ctx, "handlers: reading the finished game: "+err.Error())
				played = nil
			}

			render(ctx, w, logger, pages.Over(pages.OverProps{Game: g, Sightings: played}))
			return
		}

		sight, err := games.Round(ctx, g)
		if err != nil {
			// The sighting a round asks about has gone, which a reseeded
			// database does, so the player gets a new game rather than an
			// error page about a row they never knew existed.
			logger.WarnContext(ctx, "handlers: reading the round, starting again: "+err.Error())
			http.Redirect(w, r, "/play/new", http.StatusSeeOther)
			return
		}

		render(ctx, w, logger, pages.Question(pages.QuestionProps{Game: g, Sighting: sight}))
	}
}

// NewGame throws away whatever the player had and starts them a fresh game.
func NewGame(logger *slog.Logger, games GameService, session Session) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if _, err := start(ctx, w, games, session); err != nil {
			logger.ErrorContext(ctx, "handlers: starting a game: "+err.Error())
			renderError(ctx, w, logger, http.StatusInternalServerError, "Unable to start a game.")
			return
		}

		http.Redirect(w, r, "/play", http.StatusSeeOther)
	}
}

// Guess scores the player's guess against where the animal was really seen.
func Guess(logger *slog.Logger, games GameService, session Session) http.HandlerFunc {
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
			renderError(ctx, w, logger, http.StatusBadRequest, "That guess isn't anywhere on earth.")
			return
		}

		sight, err := games.Round(ctx, g)
		if err != nil {
			logger.WarnContext(ctx, "handlers: reading the round, starting again: "+err.Error())
			http.Redirect(w, r, "/play/new", http.StatusSeeOther)
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
		}))
	}
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
