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
