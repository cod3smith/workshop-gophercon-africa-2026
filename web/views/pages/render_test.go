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
