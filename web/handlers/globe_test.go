package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"uuid"
	"workshop/internal/domain/sighting"
	"workshop/internal/domain/species"
	"workshop/web/handlers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// aPlacedLion is a sighting the globe can plot: a position, a name, a
// class and a photograph.
func aPlacedLion() sighting.Sighting {
	return sighting.Sighting{
		ID:         uuid.New(),
		HappenedAt: time.Date(2024, time.March, 9, 0, 0, 0, 0, time.UTC),
		Location: sighting.Location{
			Locality:    "Maasai Mara",
			Country:     "Kenya",
			Coordinates: sighting.Coordinates{Latitude: -1.4061, Longitude: 35.0117},
		},
		Species: species.Species{
			Class:          "Mammalia",
			ScientificName: "Panthera leo (Linnaeus, 1758)",
			CanonicalName:  "Panthera leo",
			VernacularName: "lion",
		},
		Media: []sighting.Media{{Type: "StillImage", URL: "https://example.org/photos/1/original.jpg"}},
	}
}

// anUnplacedEagle is a sighting nobody wrote a position for. The globe has
// nowhere to put it, and the page still has to list it.
func anUnplacedEagle() sighting.Sighting {
	return sighting.Sighting{
		ID:         uuid.New(),
		HappenedAt: time.Date(2023, time.July, 2, 0, 0, 0, 0, time.UTC),
		Location:   sighting.Location{Country: "Namibia"},
		Species: species.Species{
			Class:          "Aves",
			ScientificName: "Aquila verreauxii Lesson, 1831",
			VernacularName: "Verreaux's eagle",
		},
	}
}

// serveFeed asks the real route for the feed, so the pattern is under test
// too.
func serveFeed(t *testing.T, lister handlers.SightingsLister) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle("GET /globe/sightings.json", handlers.GlobeFeed(quietLogger(), lister))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/globe/sightings.json", nil))

	return rec
}

// serveGlobe asks the real route for the page.
func serveGlobe(t *testing.T, lister handlers.SightingsLister) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle("GET /globe", handlers.Globe(quietLogger(), lister))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/globe", nil))

	return rec
}

func TestGlobeFeed_IsOneArrayOfEverySightingWithAPosition(t *testing.T) {
	t.Parallel()

	lion, eagle := aPlacedLion(), anUnplacedEagle()

	rec := serveFeed(t, fakeLister{items: []sighting.Sighting{lion, eagle}})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

	var points []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &points), "the feed should be one JSON array")

	// The eagle has no position, so there is nowhere on a sphere to put it.
	require.Len(t, points, 1, "only the sighting with a position belongs in the feed")

	got := points[0]
	assert.Equal(t, lion.ID.String(), got["id"])
	assert.InDelta(t, -1.4061, got["latitude"], 0.00001)
	assert.InDelta(t, 35.0117, got["longitude"], 0.00001)
	assert.Equal(t, "Lion", got["name"])
	assert.Equal(t, "Panthera leo (Linnaeus, 1758)", got["scientificName"])
	assert.Equal(t, "Mammalia", got["class"])
	assert.Equal(t, "Mammals", got["group"], "the class is named in English for the card, by the server and not twice")
	assert.Equal(t, "Kenya", got["country"])
	assert.Equal(t, "Maasai Mara, Kenya", got["place"])
	assert.Equal(t, "9 March 2024", got["when"])
	assert.Equal(t, "https://example.org/photos/1/small.jpg", got["thumbnail"])
}

func TestGlobeFeed_IsAnEmptyArrayWhenNothingIsPlaceable(t *testing.T) {
	t.Parallel()

	for name, lister := range map[string]fakeLister{
		"an empty record":         {},
		"nothing with a position": {items: []sighting.Sighting{anUnplacedEagle()}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rec := serveFeed(t, lister)

			require.Equal(t, http.StatusOK, rec.Code)
			// An array, never null: a browser handed null fails at the first
			// thing it tries to do with it.
			assert.JSONEq(t, "[]", rec.Body.String())
		})
	}
}

func TestGlobeFeed_SaysSoWhenTheStoreIsUnwell(t *testing.T) {
	t.Parallel()

	rec := serveFeed(t, fakeLister{err: errors.New("database is locked")})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"), "a fetch asked for JSON and gets JSON, even when it fails")
	assert.NotContains(t, rec.Body.String(), "database is locked", "a store's message is for the log, not the page")
}

func TestGlobe_ListsEverySightingAsALinkAndNamesTheClasses(t *testing.T) {
	t.Parallel()

	lion, eagle := aPlacedLion(), anUnplacedEagle()

	rec := serveGlobe(t, fakeLister{items: []sighting.Sighting{lion, eagle}})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))

	html := rec.Body.String()

	// Both sightings are links in the HTML itself. The one with no position
	// is on the globe's list even though it is not on the globe.
	assert.Contains(t, html, `href="/record/`+lion.ID.String()+`"`)
	assert.Contains(t, html, `href="/record/`+eagle.ID.String()+`"`)
	assert.Contains(t, html, "No position recorded")

	// The legend names each class present, with its count.
	assert.Contains(t, html, "Mammals")
	assert.Contains(t, html, "Birds")
}

func TestGlobe_SaysSoWhenTheStoreIsUnwell(t *testing.T) {
	t.Parallel()

	rec := serveGlobe(t, fakeLister{err: errors.New("database is locked")})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), "Unable to load the globe")
	assert.NotContains(t, rec.Body.String(), "database is locked")
}
