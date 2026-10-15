package pages_test

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
	"time"
	"uuid"
	"workshop/internal/domain/game"
	"workshop/internal/domain/sighting"
	"workshop/internal/domain/weather"
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

func TestTaxon_NamesTheGroupInPlainEnglish(t *testing.T) {
	t.Parallel()

	tt := map[string]struct {
		class, phylum, want string
	}{
		"mammals":            {class: "Mammalia", want: "Mammals"},
		"birds":              {class: "Aves", want: "Birds"},
		"turtles":            {class: "Testudines", want: "Turtles"},
		"lizards and snakes": {class: "Squamata", want: "Lizards and snakes"},
		"crocodiles":         {class: "Crocodylia", want: "Crocodiles"},
		"sharks and rays":    {class: "Elasmobranchii", want: "Sharks and rays"},
		// A class this table has not heard of is still a hint, so it is
		// shown as recorded rather than hidden.
		"an unlisted class": {class: "Amphibia", want: "Amphibia"},
		"no class":          {phylum: "Chordata", want: "Chordata"},
		"nothing at all":    {want: "Unrecorded"},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := sighting.Sighting{}
			s.Species.Class = test.class
			s.Species.Phylum = test.phylum
			assert.Equal(t, test.want, pages.Taxon(s))
		})
	}
}

func TestQuestion_ExplainsTheGroupAndTheWeatherInPlainEnglish(t *testing.T) {
	t.Parallel()

	s := sighting.Sighting{
		ID:         uuid.New(),
		HappenedAt: time.Date(2024, time.March, 5, 0, 0, 0, 0, time.UTC),
		Media:      []sighting.Media{{Type: "StillImage", URL: "https://example.org/animal.jpg"}},
	}
	s.Species.Class = "Mammalia"
	s.Weather.Temperature.Actual = 28
	// Code 2 is "Partly cloudy" in Open-Meteo's table. The stored
	// description is what the wrong WMO table called it.
	s.Weather.Condition = weather.Condition{Code: 2, Description: "State of sky on the whole unchanged"}

	var buf bytes.Buffer
	err := pages.Question(pages.QuestionProps{Game: game.New([]uuid.UUID{s.ID}), Sighting: s}).Render(context.Background(), &buf)
	require.NoError(t, err)
	html := buf.String()

	assert.Contains(t, html, "Mammals")
	assert.NotContains(t, html, ">Mammalia<", "the Latin should not be the visible text")
	assert.Contains(t, html, `title="Mammalia"`, "the Latin should stay available on hover")
	assert.Contains(t, html, "28°C, partly cloudy")
	assert.NotContains(t, html, "State of sky on the whole unchanged", "the stored description is from the wrong table")
}

func TestCredit(t *testing.T) {
	t.Parallel()

	tt := map[string]struct {
		media          sighting.Media
		scientificName string
		want           string
	}{
		"a photographer is named": {
			media: sighting.Media{Creator: "Jos Simons"}, scientificName: "Ara macao",
			want: "Jos Simons",
		},
		"the rights holder stands in for a missing creator": {
			media: sighting.Media{RightsHolder: "Jos Simons"}, scientificName: "Ara macao",
			want: "Jos Simons",
		},
		"nobody is recorded": {
			media: sighting.Media{}, scientificName: "Ara macao",
			want: "Photographer unrecorded",
		},
		// A credit that is the species gives the answer away on a round
		// that has not been guessed yet, and is no credit at all.
		"a credit that is the genus in capitals": {
			media: sighting.Media{RightsHolder: "MALACOCHERSUS"}, scientificName: "Malacochersus tornieri",
			want: "Photographer unrecorded",
		},
		"a credit that is the genus": {
			media: sighting.Media{Creator: "Malacochersus"}, scientificName: "Malacochersus tornieri",
			want: "Photographer unrecorded",
		},
		"a credit that is the full name": {
			media: sighting.Media{Creator: "  malacochersus TORNIERI "}, scientificName: "Malacochersus tornieri",
			want: "Photographer unrecorded",
		},
		"an unknown species leaves the credit alone": {
			media: sighting.Media{Creator: "Jos Simons"}, scientificName: "",
			want: "Jos Simons",
		},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, pages.Credit(test.media, test.scientificName))
		})
	}
}

// finished returns a complete game over Rounds sightings, scored from the
// given distances, and the sightings keyed as the handler supplies them.
func finished(distances []float64) (game.Game, map[uuid.UUID]sighting.Sighting) {
	ids := make([]uuid.UUID, len(distances))
	played := make(map[uuid.UUID]sighting.Sighting, len(distances))
	for i := range distances {
		ids[i] = uuid.New()

		s := sighting.Sighting{
			ID:       ids[i],
			Location: sighting.Location{Locality: "Nairobi", Coordinates: sighting.Coordinates{Latitude: -1.29, Longitude: 36.82}},
		}
		s.Species.VernacularName = "plains zebra"
		played[ids[i]] = s
	}

	g := game.New(ids)
	for _, km := range distances {
		guess := sighting.Coordinates{Latitude: 10, Longitude: 20}
		g.Record(game.Result{DistanceKm: km, Points: game.ScoreFor(km), Guess: &guess})
	}
	return g, played
}

func renderOver(t *testing.T, props pages.OverProps) string {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, pages.Over(props).Render(context.Background(), &buf))
	return buf.String()
}

func TestOver_SharesAnAddressThatCanBeFollowed(t *testing.T) {
	t.Parallel()

	g, played := finished([]float64{7.2, 900, 3000, 6000, 9000})
	html := renderOver(t, pages.OverProps{Game: g, Sightings: played, URL: "https://safari.example.test/play"})

	// The share block is the only thing designed to spread the game, and a
	// result pasted into Slack with no address cannot be acted on.
	share := regexp.MustCompile(`(?s)<pre[^>]*id="share-text"[^>]*>(.*?)</pre>`).FindStringSubmatch(html)
	require.Len(t, share, 2, "the page should carry the share block")
	assert.Contains(t, share[1], "https://safari.example.test/play")
}

func TestOver_NamesTheBestRound(t *testing.T) {
	t.Parallel()

	g, played := finished([]float64{3000, 900, 7.2, 6000, 9000})
	html := renderOver(t, pages.OverProps{Game: g, Sightings: played, URL: "https://safari.example.test/play"})

	best := regexp.MustCompile(`(?s)<p[^>]*class="final__best[^"]*"[^>]*>(.*?)</p>`).FindStringSubmatch(html)
	require.Len(t, best, 2, "the best round should be headlined")
	assert.Contains(t, best[1], "Your best")
	assert.Contains(t, best[1], "Plains zebra")
	assert.Contains(t, best[1], "7.2 km out")
	assert.Contains(t, best[1], "round 3", "the player should be able to find it in the list")

	assert.Less(t, regexp.MustCompile(`class="final__best`).FindStringIndex(html)[0],
		regexp.MustCompile(`class="rounds"`).FindStringIndex(html)[0], "the headline comes above the rounds list")
}

func TestOver_HasNoBestRoundWhenNothingWasScored(t *testing.T) {
	t.Parallel()

	html := renderOver(t, pages.OverProps{Game: game.New(nil)})

	assert.NotContains(t, html, "final__best")
}

func TestOver_MapsTheFivePlacesButReadsWithoutIt(t *testing.T) {
	t.Parallel()

	g, played := finished([]float64{7.2, 900, 3000, 6000, 9000})
	html := renderOver(t, pages.OverProps{Game: g, Sightings: played, URL: "https://safari.example.test/play"})

	box := regexp.MustCompile(`<div[^>]*data-safari-map[^>]*>`).FindString(html)
	require.NotEmpty(t, box, "the five places should be on a map")

	// Without Leaflet the box would be an empty panel, so it ships hidden
	// and safari.js reveals it only once there is a map to draw.
	assert.Contains(t, box, "hidden")

	data := regexp.MustCompile(`(?s)<script id="safari-map"[^>]*>(.*?)</script>`).FindStringSubmatch(html)
	require.Len(t, data, 2)
	assert.Contains(t, data[1], `"mode":"final"`)
	assert.Equal(t, game.Rounds, strings.Count(data[1], `"answer"`), "one true position per round")
	assert.Equal(t, game.Rounds, strings.Count(data[1], `"guess"`), "one guess per round")

	assert.Contains(t, html, "leaflet", "the page that draws a map has to load Leaflet")

	// The list carries the same places as text, which is the fallback.
	assert.Equal(t, game.Rounds, strings.Count(html, "Nairobi</span>"), "every place is also named in the rounds list")
}

func TestOver_MapsARoundWhoseGuessWasNotKept(t *testing.T) {
	t.Parallel()

	g, played := finished([]float64{7.2, 900, 3000, 6000, 9000})
	g.Results[1].Guess = nil
	html := renderOver(t, pages.OverProps{Game: g, Sightings: played})

	data := regexp.MustCompile(`(?s)<script id="safari-map"[^>]*>(.*?)</script>`).FindStringSubmatch(html)
	require.Len(t, data, 2)
	assert.Equal(t, game.Rounds, strings.Count(data[1], `"answer"`))
	assert.Equal(t, game.Rounds-1, strings.Count(data[1], `"guess"`), "a missing guess is left off, not drawn at 0,0")
}

// A medal and a tone are two ways of saying how a round went, one in the
// pasted share text and one in the page's colour. They are separate
// switches, so this is what keeps them from drifting into disagreement.
func TestMedalAndToneAgree(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"🟩": "is-near",
		"🟨": "is-mid",
		"🟧": "is-far",
		"⬛": "is-far",
	}

	for points := 0; points <= game.MaxRoundScore; points += 50 {
		assert.Equal(t, want[game.Medal(points)], pages.ToneFor(points), "%d points", points)
	}
}

func TestOver_DrawsTheMedalsRatherThanEmojiTheyCanVanishAs(t *testing.T) {
	t.Parallel()

	g, played := finished([]float64{7.2, 900, 3000, 6000, 9000})
	html := renderOver(t, pages.OverProps{Game: g, Sightings: played})

	assert.Equal(t, game.Rounds, strings.Count(html, `class="medal"`), "one drawn square per round")
	for _, emoji := range []string{"🟩", "🟨", "🟧", "⬛"} {
		assert.NotContains(t, html[:strings.Index(html, `id="share-text"`)], emoji, "the rounds list is drawn, not typed")
	}
}
