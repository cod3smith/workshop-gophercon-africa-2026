package pages_test

import (
	"bytes"
	"context"
	"regexp"
	"testing"
	"uuid"
	"workshop/internal/domain/game"
	"workshop/internal/domain/sighting"
	"workshop/web/views/pages"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inputNamed finds the <input> tag with the given name, whatever order
// templ writes its attributes in.
func inputNamed(html, name string) string {
	return regexp.MustCompile(`<input[^>]*name="` + name + `"[^>]*>`).FindString(html)
}

func TestQuestion_OffersAGuessWithoutTheMap(t *testing.T) {
	t.Parallel()

	s := sighting.Sighting{
		ID:       uuid.New(),
		Location: sighting.Location{Coordinates: sighting.Coordinates{Latitude: -1.29, Longitude: 36.82}},
		Media:    []sighting.Media{{Type: "StillImage", URL: "https://example.org/animal.jpg"}},
	}

	var buf bytes.Buffer
	err := pages.Question(pages.QuestionProps{Game: game.New([]uuid.UUID{s.ID}), Sighting: s}).Render(context.Background(), &buf)
	require.NoError(t, err)
	html := buf.String()

	// A screen reader cannot aim a crosshair, and a blocked CDN leaves no
	// map at all, so the coordinates have to be fields a person can fill in.
	assert.Contains(t, html, `type="number"`, "the coordinates should be typed, not hidden")
	assert.Contains(t, html, `aria-live="polite"`, "the crosshair position should be announced")
	assert.Contains(t, html, `id="crosshair-readout"`)

	for _, name := range []string{"lat", "lng"} {
		input := inputNamed(html, name)
		require.NotEmpty(t, input, "the form should carry a %q field", name)
		assert.Contains(t, input, `type="number"`)
		assert.NotContains(t, input, `type="hidden"`)
	}

	// Half the world is negative. iOS's decimal keypad has no minus sign, so
	// inputmode="decimal" would make every southern or western guess, which
	// is most of Africa, impossible to type on an iPhone. type="number"
	// already picks a keyboard that has one.
	assert.Contains(t, inputNamed(html, "lat"), `min="-90"`)
	assert.Contains(t, inputNamed(html, "lng"), `min="-180"`)
	assert.NotContains(t, inputNamed(html, "lat"), `inputmode="decimal"`)
	assert.NotContains(t, inputNamed(html, "lng"), `inputmode="decimal"`)

	// With no JavaScript nothing can enable the button, so it has to be
	// enabled when the page arrives. safari.js disables it while the fields
	// are empty.
	submit := regexp.MustCompile(`<button[^>]*id="guess-submit"[^>]*>`).FindString(html)
	require.NotEmpty(t, submit, "the form should carry the submit button")
	assert.NotContains(t, submit, "disabled")
}

func TestQuestion_KeepsTheTypedRouteDescribedAndTheCluesAnnounced(t *testing.T) {
	t.Parallel()

	s := sighting.Sighting{
		ID:    uuid.New(),
		Media: []sighting.Media{{Type: "StillImage", URL: "https://example.org/animal.jpg"}},
	}

	var buf bytes.Buffer
	err := pages.Question(pages.QuestionProps{Game: game.New([]uuid.UUID{s.ID}), Sighting: s}).Render(context.Background(), &buf)
	require.NoError(t, err)
	html := buf.String()

	// The map cites this paragraph as its description, so the typed route
	// has to be in its visible sentence, not in text a stylesheet can drop.
	hint := regexp.MustCompile(`(?s)<p[^>]*id="map-help"[^>]*>(.*?)<span`).FindStringSubmatch(html)
	require.Len(t, hint, 2, "the map's description paragraph should be present")
	assert.Contains(t, hint[1], "type a latitude and longitude")

	// The clues come after the submit button in source order, so a screen
	// reader is told where they went before it reaches the button.
	told := "The clues about this animal are listed after the map."
	assert.Contains(t, html, told)
	assert.Less(t, bytes.Index([]byte(html), []byte(told)), bytes.Index([]byte(html), []byte(`id="guess-submit"`)))
}

func TestPhotoURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		size string
		want string
	}{
		{
			name: "an iNaturalist original is swapped for the requested size",
			url:  "https://inaturalist-open-data.s3.amazonaws.com/photos/1/original.jpeg",
			size: "medium",
			want: "https://inaturalist-open-data.s3.amazonaws.com/photos/1/medium.jpeg",
		},
		{
			name: "the extension is kept, whatever it is",
			url:  "https://inaturalist-open-data.s3.amazonaws.com/photos/2/original.png",
			size: "large",
			want: "https://inaturalist-open-data.s3.amazonaws.com/photos/2/large.png",
		},
		{
			name: "a host with no original segment is left untouched",
			url:  "https://observation.org/photos/9.jpg",
			size: "medium",
			want: "https://observation.org/photos/9.jpg",
		},
		{
			name: "an original that is not the final segment is left untouched",
			url:  "https://example.org/original.photos/9.jpg",
			size: "medium",
			want: "https://example.org/original.photos/9.jpg",
		},
		{
			name: "no media gives no URL",
			url:  "",
			size: "medium",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, pages.PhotoURL(sighting.Media{URL: tt.url}, tt.size))
		})
	}
}

func TestQuestion_AsksForAMeasuredPhotograph(t *testing.T) {
	t.Parallel()

	s := sighting.Sighting{
		ID:    uuid.New(),
		Media: []sighting.Media{{Type: "StillImage", URL: "https://inaturalist-open-data.s3.amazonaws.com/photos/1/original.jpeg"}},
	}

	var buf bytes.Buffer
	err := pages.Question(pages.QuestionProps{Game: game.New([]uuid.UUID{s.ID}), Sighting: s}).Render(context.Background(), &buf)
	require.NoError(t, err)
	html := buf.String()

	// The plate is the one image the player is scored on reading, so it
	// asks for a size fit to look at rather than the original, and says how
	// big it is so the page does not jump when it lands.
	img := regexp.MustCompile(`<img[^>]*class="plate__photo"[^>]*>`).FindString(html)
	require.NotEmpty(t, img)
	assert.Contains(t, img, `src="https://inaturalist-open-data.s3.amazonaws.com/photos/1/large.jpeg"`)
	assert.Regexp(t, `width="\d+"`, img)
	assert.Regexp(t, `height="\d+"`, img)
}

func TestHome_ShipsMeasuredThumbnailsNotOriginals(t *testing.T) {
	t.Parallel()

	s := sighting.Sighting{
		ID:    uuid.New(),
		Media: []sighting.Media{{Type: "StillImage", URL: "https://inaturalist-open-data.s3.amazonaws.com/photos/1/original.jpeg"}},
	}

	var buf bytes.Buffer
	err := pages.Home([]sighting.Sighting{s}).Render(context.Background(), &buf)
	require.NoError(t, err)
	html := buf.String()

	// Sixty originals on one page stall conference wifi, and without
	// dimensions each one shoves the page down as it lands.
	img := regexp.MustCompile(`<img[^>]*class="tile__photo"[^>]*>`).FindString(html)
	require.NotEmpty(t, img)
	assert.Contains(t, img, `src="https://inaturalist-open-data.s3.amazonaws.com/photos/1/medium.jpeg"`)
	assert.NotContains(t, html, "original.jpeg")
	assert.Regexp(t, `width="\d+"`, img)
	assert.Regexp(t, `height="\d+"`, img)
}

func TestQuestion_LetsThePhotographBeOpenedFullSize(t *testing.T) {
	t.Parallel()

	s := sighting.Sighting{
		ID:    uuid.New(),
		Media: []sighting.Media{{Type: "StillImage", URL: "https://inaturalist-open-data.s3.amazonaws.com/photos/1/original.jpeg"}},
	}

	var buf bytes.Buffer
	err := pages.Question(pages.QuestionProps{Game: game.New([]uuid.UUID{s.ID}), Sighting: s}).Render(context.Background(), &buf)
	require.NoError(t, err)
	html := buf.String()

	// A portrait photograph letterboxed into the phone's 16:9 plate is
	// whole but small, so the plate has to open the full photograph with no
	// script involved.
	link := regexp.MustCompile(`<a[^>]*class="plate__link"[^>]*>`).FindString(html)
	require.NotEmpty(t, link, "the photograph should be a link to itself")
	assert.Contains(t, link, `href="https://inaturalist-open-data.s3.amazonaws.com/photos/1/large.jpeg"`)
	assert.Contains(t, link, `target="_blank"`)
	assert.Contains(t, link, `rel="noopener"`)
	assert.Contains(t, link, `aria-label="See the full photograph`, "the link should say what it does, not just what the image shows")

	// The link must wrap the image and sit ahead of the guess form's fields,
	// so it adds one tab stop before the map rather than hiding the controls.
	assert.Less(t, regexp.MustCompile(`class="plate__link"`).FindStringIndex(html)[0], regexp.MustCompile(`name="lat"`).FindStringIndex(html)[0])
	assert.Regexp(t, `class="plate__link"[^>]*>\s*<img[^>]*class="plate__photo"`, html)
}
