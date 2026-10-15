package handlers_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"
	"uuid"
	"workshop/internal/common/logs"
	"workshop/internal/domain/sighting"
	"workshop/internal/domain/species"
	"workshop/internal/domain/weather"
	"workshop/web/handlers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHome(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		got := serveHome(t, newRecord(1), "/")
		assert.Equal(t, http.StatusOK, got.Code)
		assert.Equal(t, "text/html; charset=utf-8", got.Header().Get("Content-Type"))
	})

	t.Run("No sightings", func(t *testing.T) {
		t.Parallel()

		got := serveHome(t, newRecord(0), "/")
		assert.Equal(t, http.StatusOK, got.Code)
		assert.Contains(t, got.Body.String(), "Nothing has been ingested yet")
		assert.NotContains(t, got.Body.String(), `class="pager"`, "an empty record has no pages to turn")
	})

	t.Run("List error", func(t *testing.T) {
		t.Parallel()

		record := newRecord(5)
		record.listErr = errors.New("store unavailable")

		got := serveHome(t, record, "/")
		assert.Equal(t, http.StatusInternalServerError, got.Code)
		assert.Equal(t, "text/html; charset=utf-8", got.Header().Get("Content-Type"))
	})

	t.Run("Count error", func(t *testing.T) {
		t.Parallel()

		record := newRecord(5)
		record.countErr = errors.New("store unavailable")

		got := serveHome(t, record, "/")
		assert.Equal(t, http.StatusInternalServerError, got.Code)
	})

	t.Run("Totals error", func(t *testing.T) {
		t.Parallel()

		record := newRecord(5)
		record.totalsErr = errors.New("store unavailable")

		got := serveHome(t, record, "/")
		assert.Equal(t, http.StatusInternalServerError, got.Code)
	})
}

func TestHome_Pages(t *testing.T) {
	t.Parallel()

	// 500 sightings at 24 a page is 21 pages, the last holding 20.
	record := newRecord(500)

	t.Run("A page holds twenty-four tiles", func(t *testing.T) {
		t.Parallel()

		assert.Len(t, tileLinks(serveHome(t, record, "/").Body.String()), 24)
		assert.Len(t, tileLinks(serveHome(t, record, "/?page=21").Body.String()), 20)
	})

	t.Run("Page two is a different set from page one", func(t *testing.T) {
		t.Parallel()

		one := tileLinks(serveHome(t, record, "/?page=1").Body.String())
		two := tileLinks(serveHome(t, record, "/?page=2").Body.String())

		require.NotEmpty(t, one)
		require.NotEmpty(t, two)
		assert.NotEqual(t, one, two)
		for _, href := range two {
			assert.NotContains(t, one, href, "a sighting should not be on two pages")
		}
	})

	t.Run("Pages laid end to end are the whole record once", func(t *testing.T) {
		t.Parallel()

		seen := make(map[string]int)
		for page := 1; page <= 21; page++ {
			for _, href := range tileLinks(serveHome(t, record, fmt.Sprintf("/?page=%d", page)).Body.String()) {
				seen[href]++
			}
		}

		assert.Len(t, seen, 500)
		for href, n := range seen {
			assert.Equal(t, 1, n, href)
		}
	})

	t.Run("The page is stated", func(t *testing.T) {
		t.Parallel()

		assert.Contains(t, serveHome(t, record, "/").Body.String(), "Page 1 of 21")
		assert.Contains(t, serveHome(t, record, "/?page=2").Body.String(), "Page 2 of 21")
		assert.Contains(t, serveHome(t, record, "/?page=21").Body.String(), "Page 21 of 21")
	})

	t.Run("A bad page number is clamped to a page that exists", func(t *testing.T) {
		t.Parallel()

		tt := map[string]string{
			"/?page=0":     "Page 1 of 21",
			"/?page=-3":    "Page 1 of 21",
			"/?page=abc":   "Page 1 of 21",
			"/?page=":      "Page 1 of 21",
			"/?page=2.5":   "Page 1 of 21",
			"/?page=99999": "Page 21 of 21",
			// Far past what an int can hold.
			"/?page=99999999999999999999999": "Page 21 of 21",
		}

		for target, want := range tt {
			t.Run(target, func(t *testing.T) {
				t.Parallel()

				got := serveHome(t, record, target)
				assert.Equal(t, http.StatusOK, got.Code)
				assert.Contains(t, got.Body.String(), want)
				assert.NotEmpty(t, tileLinks(got.Body.String()), "never an empty grid")
			})
		}
	})

	t.Run("A single page has no one to turn to", func(t *testing.T) {
		t.Parallel()

		got := serveHome(t, newRecord(24), "/?page=5")
		assert.Equal(t, http.StatusOK, got.Code)
		assert.Len(t, tileLinks(got.Body.String()), 24)
		assert.NotContains(t, got.Body.String(), `class="pager"`, "a record that fits a page has no pages to turn")
	})
}

func TestHome_PagerEnds(t *testing.T) {
	t.Parallel()

	record := newRecord(500)

	t.Run("The first page links onward and not back", func(t *testing.T) {
		t.Parallel()

		html := serveHome(t, record, "/").Body.String()
		assert.Contains(t, html, `href="/?page=2#record" rel="next"`)
		assert.NotContains(t, html, `rel="prev"`, "there is no page before the first")
		assert.Contains(t, html, `aria-disabled="true"`, "the missing link is stated as disabled")
	})

	t.Run("A middle page links both ways", func(t *testing.T) {
		t.Parallel()

		html := serveHome(t, record, "/?page=5").Body.String()
		assert.Contains(t, html, `href="/?page=4#record" rel="prev"`)
		assert.Contains(t, html, `href="/?page=6#record" rel="next"`)
		assert.NotContains(t, html, `aria-disabled="true"`)
	})

	t.Run("The last page links back and not onward", func(t *testing.T) {
		t.Parallel()

		html := serveHome(t, record, "/?page=21").Body.String()
		assert.Contains(t, html, `href="/?page=20#record" rel="prev"`)
		assert.NotContains(t, html, `rel="next"`, "there is no page after the last")
		assert.Contains(t, html, `aria-disabled="true"`)
	})

	t.Run("Going back to the first page drops the query", func(t *testing.T) {
		t.Parallel()

		html := serveHome(t, record, "/?page=2").Body.String()
		assert.Contains(t, html, `href="/#record" rel="prev"`)
	})
}

func TestHome_Tiles(t *testing.T) {
	t.Parallel()

	t.Run("Each tile leads to its own record, once", func(t *testing.T) {
		t.Parallel()

		record := newRecord(30)
		html := serveHome(t, record, "/").Body.String()

		for _, s := range record.items[:24] {
			href := `href="/record/` + s.ID.String() + `"`
			assert.Equal(t, 1, countOf(html, href), "one link for %s", s.ID)
		}

		// The keyboard reaches each tile once: a tile is a single link,
		// never a link with another inside it.
		assert.Equal(t, 24, countOf(html, `class="tile__link"`))
		for _, tile := range regexp.MustCompile(`(?s)<li class="tile">(.*?)</li>`).FindAllStringSubmatch(html, -1) {
			assert.Equal(t, 1, countOf(tile[1], "<a "), "a tile holds exactly one link")
		}
	})

	t.Run("A reading of zero degrees is shown", func(t *testing.T) {
		t.Parallel()

		record := newRecord(1)
		record.items[0].Weather = weather.Weather{ID: uuid.New()}

		assert.Contains(t, serveHome(t, record, "/").Body.String(), "0°C")
	})

	t.Run("No reading, no temperature", func(t *testing.T) {
		t.Parallel()

		record := newRecord(1)
		record.items[0].Weather = weather.Weather{}

		assert.NotContains(t, serveHome(t, record, "/").Body.String(), "°C")
	})
}

func TestHome_Figures(t *testing.T) {
	t.Parallel()

	// The record holds 500 sightings, 78 species, 56 countries and 921
	// photographs. A page of it shows 24 tiles, and the figures must not
	// shrink to match.
	record := newRecord(500)
	record.totals = sighting.Totals{
		Sightings: 500,
		Species:   78,
		Countries: 56,
		Media:     921,
		Earliest:  time.Date(1948, time.February, 24, 0, 0, 0, 0, time.UTC),
		Latest:    time.Date(2026, time.September, 12, 0, 0, 0, 0, time.UTC),
	}

	for _, target := range []string{"/", "/?page=2", "/?page=21"} {
		html := serveHome(t, record, target).Body.String()

		assert.Regexp(t, `<dt>Sightings</dt>\s*<dd class="mono">500</dd>`, html, target)
		assert.Regexp(t, `<dt>Species</dt>\s*<dd class="mono">78</dd>`, html, target)
		assert.Regexp(t, `<dt>Countries</dt>\s*<dd class="mono">56</dd>`, html, target)
		assert.Regexp(t, `<dt>Photographs</dt>\s*<dd class="mono">921</dd>`, html, target)
		assert.Contains(t, html, "Feb 1948", target)
		assert.Contains(t, html, "Sep 2026", target)
	}

	t.Run("The range line says which of the record is on the page", func(t *testing.T) {
		t.Parallel()

		assert.Contains(t, serveHome(t, record, "/").Body.String(), "1–24 of 500")
		assert.Contains(t, serveHome(t, record, "/?page=2").Body.String(), "25–48 of 500")
		assert.Contains(t, serveHome(t, record, "/?page=21").Body.String(), "481–500 of 500")
	})
}

// recordStore is a record of n sightings, newest first, that pages the
// way the real store does: a window of Limit from Offset.
type recordStore struct {
	items []sighting.Sighting

	totals sighting.Totals

	listErr, countErr, totalsErr error
}

// newRecord returns a record of n sightings, each with a name of its own
// and a reading of the weather.
func newRecord(n int) *recordStore {
	items := make([]sighting.Sighting, n)
	for i := range items {
		items[i] = sighting.Sighting{
			ID:         uuid.New(),
			GBIFKey:    int64(i + 1),
			HappenedAt: time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Hour),
			Species:    species.Species{VernacularName: fmt.Sprintf("animal %d", i+1)},
			Weather:    weather.Weather{ID: uuid.New()},
		}
	}

	return &recordStore{
		items:  items,
		totals: sighting.Totals{Sightings: n, Species: n, Countries: 1, Media: 0, Earliest: time.Now(), Latest: time.Now()},
	}
}

func (r *recordStore) List(_ context.Context, filter sighting.ListFilter) ([]sighting.Sighting, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}

	from := min(filter.Offset, len(r.items))
	to := len(r.items)
	if filter.Limit > 0 {
		to = min(from+filter.Limit, to)
	}

	return r.items[from:to], nil
}

func (r *recordStore) Count(context.Context, sighting.ListFilter) (int, error) {
	return len(r.items), r.countErr
}

func (r *recordStore) Totals(context.Context) (sighting.Totals, error) {
	return r.totals, r.totalsErr
}

// fakeLister is a store that can only list, which is all the globe asks.
type fakeLister struct {
	items []sighting.Sighting
	err   error
}

func (f fakeLister) List(context.Context, sighting.ListFilter) ([]sighting.Sighting, error) {
	return f.items, f.err
}

func serveHome(t *testing.T, record handlers.SightingsPager, target string) *httptest.ResponseRecorder {
	t.Helper()

	logger := logs.NewProduction(io.Discard, slog.LevelError)
	rec := httptest.NewRecorder()
	handlers.Home(logger, record)(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))

	return rec
}

var tileHref = regexp.MustCompile(`<a[^>]*class="tile__link"[^>]*href="([^"]*)"|<a[^>]*href="([^"]*)"[^>]*class="tile__link"`)

// tileLinks returns where each tile on the page leads, in order.
func tileLinks(html string) []string {
	var hrefs []string
	for _, m := range tileHref.FindAllStringSubmatch(html, -1) {
		hrefs = append(hrefs, m[1]+m[2])
	}
	return hrefs
}

// countOf is how many times needle appears in html.
func countOf(html, needle string) int {
	return len(regexp.MustCompile(regexp.QuoteMeta(needle)).FindAllStringIndex(html, -1))
}
