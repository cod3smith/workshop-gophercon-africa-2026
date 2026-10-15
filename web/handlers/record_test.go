package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"uuid"
	"workshop/internal/domain/sighting"
	"workshop/internal/domain/species"
	"workshop/internal/domain/weather"
	"workshop/web/handlers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeFinder holds the sightings it can find, and fails with err when it is
// given one.
type fakeFinder struct {
	held []sighting.Sighting
	err  error
}

func (f fakeFinder) Find(_ context.Context, id uuid.UUID) (sighting.Sighting, error) {
	if f.err != nil {
		return sighting.Sighting{}, f.err
	}
	for _, s := range f.held {
		if s.ID == id {
			return s, nil
		}
	}
	return sighting.Sighting{}, sighting.ErrNotFound
}

// aLion is a sighting with everything the page can show.
func aLion() sighting.Sighting {
	return sighting.Sighting{
		ID:         uuid.New(),
		HappenedAt: time.Date(2024, time.March, 9, 0, 0, 0, 0, time.UTC),
		Location: sighting.Location{
			Locality:    "Maasai Mara",
			Country:     "Kenya",
			Coordinates: sighting.Coordinates{Latitude: -1.4061, Longitude: 35.0117},
		},
		Species: species.Species{
			ScientificName: "Panthera leo (Linnaeus, 1758)",
			CanonicalName:  "Panthera leo",
			VernacularName: "lion",
		},
		Weather: weather.Weather{ID: uuid.New(), Temperature: weather.Temperature{Actual: 24, Apparent: 26}},
	}
}

// getRecord asks the real route for path, so the pattern is under test too.
func getRecord(t *testing.T, finder handlers.RecordFinder, sounds handlers.SoundService, path string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle("GET /record/{id}", handlers.Record(quietLogger(), finder, sounds))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return rec
}

func TestRecord(t *testing.T) {
	t.Parallel()

	lion := aLion()
	finder := fakeFinder{held: []sighting.Sighting{lion}}

	tests := []struct {
		name     string
		finder   fakeFinder
		path     string
		wantCode int
		wantBody string
	}{
		{name: "a known id", finder: finder, path: "/record/" + lion.ID.String(), wantCode: http.StatusOK, wantBody: "Lion"},
		{name: "a well-formed id nobody recorded", finder: finder, path: "/record/" + uuid.New().String(), wantCode: http.StatusNotFound, wantBody: "no sighting at that address"},
		{name: "not a uuid", finder: finder, path: "/record/not-a-uuid", wantCode: http.StatusNotFound, wantBody: "no sighting at that address"},
		{name: "a uuid one digit short", finder: finder, path: "/record/" + lion.ID.String()[1:], wantCode: http.StatusNotFound, wantBody: "no sighting at that address"},
		{name: "the nil uuid", finder: finder, path: "/record/" + uuid.Nil().String(), wantCode: http.StatusNotFound, wantBody: "no sighting at that address"},
		{name: "an id with markup in it", finder: finder, path: "/record/%3Cscript%3E", wantCode: http.StatusNotFound, wantBody: "no sighting at that address"},
		{name: "a store that is unwell", finder: fakeFinder{err: errors.New("database is locked")}, path: "/record/" + lion.ID.String(), wantCode: http.StatusInternalServerError, wantBody: "Unable to load this sighting"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := getRecord(t, tt.finder, nil, tt.path)

			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
			assert.Contains(t, rec.Body.String(), tt.wantBody)
			assert.NotContains(t, rec.Body.String(), "database is locked", "a store's message is for the log, not the page")
		})
	}
}

func TestRecord_OffersTheSoundWhenThereIsOne(t *testing.T) {
	t.Parallel()

	lion := aLion()
	sounds := &soundsFor{name: "Panthera leo", recording: lionRoar}

	rec := getRecord(t, fakeFinder{held: []sighting.Sighting{lion}}, sounds, "/record/"+lion.ID.String())

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "Panthera leo", sounds.asked, "the species is looked up by its name without the authority")
	assert.Contains(t, rec.Body.String(), "<audio")
	assert.Contains(t, rec.Body.String(), "Africa Gomez")
}

func TestRecord_SaysSoWhenThereIsNoRecording(t *testing.T) {
	t.Parallel()

	lion := aLion()
	finder := fakeFinder{held: []sighting.Sighting{lion}}

	for name, sounds := range map[string]handlers.SoundService{
		"no sound service":        nil,
		"a species with no sound": &soundsFor{name: "Acinonyx jubatus", recording: lionRoar},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rec := getRecord(t, finder, sounds, "/record/"+lion.ID.String())

			require.Equal(t, http.StatusOK, rec.Code)
			assert.NotContains(t, rec.Body.String(), "<audio")
			assert.Contains(t, rec.Body.String(), "No recording of this animal is available.")
		})
	}
}

// A slow iNaturalist must not hold the page up, and a page that gave up
// waiting reads exactly as one for a species with no recording.
func TestRecord_DoesNotWaitLongForTheSound(t *testing.T) {
	t.Parallel()

	lion := aLion()
	sounds := &soundsFor{block: true}

	start := time.Now()
	rec := getRecord(t, fakeFinder{held: []sighting.Sighting{lion}}, sounds, "/record/"+lion.ID.String())

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, sounds.hasLimit, "the lookup is not allowed to wait for as long as the request does")
	assert.Less(t, time.Since(start), 2*time.Second)
	assert.NotContains(t, rec.Body.String(), "<audio")
	assert.Contains(t, rec.Body.String(), "entry__name", "the sighting is still shown")
}
