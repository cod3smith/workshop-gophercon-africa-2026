package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
	"uuid"
	"workshop/internal/domain/sighting"
	"workshop/web/views/pages"
)

// RecordFinder is what the record page needs from the sighting domain.
type RecordFinder interface {
	Find(ctx context.Context, id uuid.UUID) (sighting.Sighting, error)
}

// recordSoundWait is how long the record page waits to learn whether the
// animal has a recording. It is shorter than the reveal's wait, because
// nobody is waiting on a guess to be scored here: a lookup that has not
// finished is left to finish for the next visit, and the page goes out
// without it.
const recordSoundWait = 500 * time.Millisecond

// Record renders the page for one sighting. An id that is not a uuid is a
// wrong address, not a fault, so it is answered as a sighting that is not
// there. A nil sounds is a site with no recordings.
func Record(logger *slog.Logger, sightings RecordFinder, sounds SoundService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			recordNotFound(ctx, w, logger)
			return
		}

		sight, err := sightings.Find(ctx, id)
		if errors.Is(err, sighting.ErrNotFound) {
			recordNotFound(ctx, w, logger)
			return
		} else if err != nil {
			logger.ErrorContext(ctx, "handlers: reading a sighting: "+err.Error())
			renderError(ctx, w, logger, http.StatusInternalServerError, "Unable to load this sighting.")
			return
		}

		// One lookup answers both: by the time recording() returns, the
		// answer is cached, so asking again costs nothing.
		render(ctx, w, logger, pages.Record(pages.RecordProps{
			Sighting:       sight,
			Recording:      recording(ctx, sounds, sight, recordSoundWait),
			RecordingKnown: recordingKnown(ctx, sounds, sight, recordSoundWait),
		}))
	}
}

// recordNotFound says there is no such sighting, which is true of an id
// that is not one as much as of one that was never recorded.
func recordNotFound(ctx context.Context, w http.ResponseWriter, logger *slog.Logger) {
	renderError(ctx, w, logger, http.StatusNotFound, "There is no sighting at that address. Browse the record, or play a round.")
}
