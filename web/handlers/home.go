package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"workshop/internal/domain/sighting"
	"workshop/web/views/pages"
)

// homePageSize is how many sightings a page of the record holds. It is a
// number a person can look through, and a grid of whole rows at the widths
// the tiles lay out in.
const homePageSize = 24

// SightingsLister is the part of the sighting domain that lists the
// record, which is all a page that shows the whole of it needs.
type SightingsLister interface {
	List(ctx context.Context, filter sighting.ListFilter) ([]sighting.Sighting, error)
}

// SightingsPager is what the home page needs from the sighting domain: a
// window of the record to show as tiles, and the count and figures that
// describe all of it.
type SightingsPager interface {
	SightingsLister
	Count(ctx context.Context, filter sighting.ListFilter) (int, error)
	Totals(ctx context.Context) (sighting.Totals, error)
}

// Home renders the front page: the hero, and the record a page at a time,
// chosen by ?page=. A page that is missing, not a number or out of range
// is the nearest page that exists, never an error and never an empty grid.
// The hero's figures describe the whole record, not the page.
func Home(logger *slog.Logger, sightings SightingsPager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		count, err := sightings.Count(ctx, sighting.ListFilter{})
		if err != nil {
			logger.ErrorContext(ctx, "handlers: counting sightings: "+err.Error())
			renderError(ctx, w, logger, http.StatusInternalServerError, "Unable to load sightings.")
			return
		}

		pageCount := max(1, (count+homePageSize-1)/homePageSize)
		page := clampPage(r.URL.Query().Get("page"), pageCount)

		items, err := sightings.List(ctx, sighting.ListFilter{
			Limit:  homePageSize,
			Offset: (page - 1) * homePageSize,
		})
		if err != nil {
			logger.ErrorContext(ctx, "handlers: listing sightings: "+err.Error())
			renderError(ctx, w, logger, http.StatusInternalServerError, "Unable to load sightings.")
			return
		}

		totals, err := sightings.Totals(ctx)
		if err != nil {
			logger.ErrorContext(ctx, "handlers: totalling sightings: "+err.Error())
			renderError(ctx, w, logger, http.StatusInternalServerError, "Unable to load sightings.")
			return
		}

		render(ctx, w, logger, pages.Home(pages.HomeProps{
			Sightings: items,
			Totals:    totals,
			Page:      page,
			Pages:     pageCount,
			First:     (page-1)*homePageSize + 1,
		}))
	}
}

// clampPage reads a page number and returns the nearest page from 1 to
// last. Something that is not a whole number is the first page. A number
// too large for an int still says which way it ran off: Atoi hands back
// the limit it overflowed, so a vast positive one is the last page.
func clampPage(raw string, last int) int {
	page, err := strconv.Atoi(raw)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 1
	}

	return min(max(page, 1), last)
}
