package handlers

import (
	"log/slog"
	"net/http"
)

// NotFound answers anything the other patterns didn't claim. It's
// registered on "GET /", the pattern every unmatched path falls back to,
// so a mistyped URL gets the site's own page rather than Go's bare
// "404 page not found".
func NotFound(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderError(r.Context(), w, logger, http.StatusNotFound, "That address isn't part of the record. Browse the sightings, or play a round.")
	}
}
