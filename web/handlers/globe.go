package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"workshop/internal/domain/sighting"
	"workshop/web/views/pages"
)

// Globe renders the whole record on one sphere.
//
// The page lists every sighting as a link in the HTML it is served as, so
// the record stays browsable where WebGL is missing, the CDN is blocked or
// JavaScript never runs at all. The sphere is what the list becomes when
// all three are there.
func Globe(logger *slog.Logger, sightings SightingsLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// No limit: this page is the whole record, and a globe showing a
		// slice of it would quietly misstate how much of the world is here.
		items, err := sightings.List(ctx, sighting.ListFilter{})
		if err != nil {
			logger.ErrorContext(ctx, "handlers: listing sightings for the globe: "+err.Error())
			renderError(ctx, w, logger, http.StatusInternalServerError, "Unable to load the globe.")
			return
		}

		render(ctx, w, logger, pages.Globe(pages.GlobeProps{Sightings: items}))
	}
}

// GlobeFeed is the one request the globe makes for its points: every
// sighting that has a position, as a single JSON array.
//
// One request for five hundred points, not five hundred requests. A
// sighting with no position is left out, because there is nowhere on a
// sphere to put it; the page's list still carries it.
func GlobeFeed(logger *slog.Logger, sightings SightingsLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		items, err := sightings.List(ctx, sighting.ListFilter{})
		if err != nil {
			logger.ErrorContext(ctx, "handlers: listing sightings for the globe feed: "+err.Error())
			feedFailed(ctx, w, logger)
			return
		}

		// Encoded whole before a byte is written, so a failure half way
		// through is a status the fetch can read rather than a truncated
		// array the globe would try to plot.
		body, err := json.Marshal(pages.GlobePoints(items))
		if err != nil {
			logger.ErrorContext(ctx, "handlers: encoding the globe feed: "+err.Error())
			feedFailed(ctx, w, logger)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))

		if _, err = w.Write(body); err != nil {
			logger.ErrorContext(ctx, "handlers: writing the globe feed: "+err.Error())
		}
	}
}

// feedFailed answers the globe's fetch in the language it asked in. The
// error page is HTML, which is no use to a script, and the page has
// already listed every sighting, so all this has to do is be a failure the
// globe can recognise and say so about.
func feedFailed(ctx context.Context, w http.ResponseWriter, logger *slog.Logger) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)

	if _, err := w.Write([]byte(`{"error":"Unable to load the sightings."}`)); err != nil {
		logger.ErrorContext(ctx, "handlers: writing the globe feed's error: "+err.Error())
	}
}
