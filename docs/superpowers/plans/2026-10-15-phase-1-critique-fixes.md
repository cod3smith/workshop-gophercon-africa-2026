# Phase 1 — Critique Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the one P0 and three P1 defects the critique found, warm up the game's ending, and clear the P3 polish — so the site is defensible before any new feature is built on it.

**Architecture:** Six independent tasks against the existing `templ` + `sqlc` Go app. Facts about the world (what a weather code means, what an animal group is called) move into the domain where they can be tested; presentation stays in `web/views/pages`. No new dependencies, no new routes, no schema change.

**Tech Stack:** Go 1.27, templ, sqlc, SQLite, Leaflet 1.9.4 from cdnjs, vanilla JS.

**Spec:** `docs/superpowers/specs/2026-10-15-sightings-platform-design.md` (Phase 1)

## Global Constraints

- No JavaScript framework; the site is server-rendered and every page must render and be readable with JavaScript disabled.
- `make web`, `make test` and the existing CLI must keep working — this is a fork of a teaching repository.
- No new third-party origin may be added in this phase.
- Commit messages carry no `Co-Authored-By` attribution lines.
- Run `go tool templ generate ./web/...` after editing any `.templ` file, and commit the generated `_templ.go` alongside it.
- Every text pair stays at 4.5:1 or better; control boundaries stay at 3:1 or better.

## Review Focus

Five things the spec implies that no task's own happy-path test would catch. Each has a test added to the task that owns the code.

1. **A photo URL that is not an iNaturalist `/original.` URL.** The committed data also contains `observation.org` photos; the size-variant swap must return those unchanged rather than mangling them. *(Task 3)*
2. **A weather code absent from the table.** Open-Meteo adds codes; `Describe` must return a usable phrase, never an empty string that renders as a blank clue. *(Task 4)*
3. **A species with an empty `Class`.** GBIF records thin out; `taxon` must fall through rather than render an empty group row. *(Task 4)*
4. **A photographer credit differing from the genus only by case.** The credit filter exists to stop an answer leak; `MALACOCHERSUS` vs `Malacochersus` must both be rejected. *(Task 4)*
5. **A non-numeric or out-of-range value typed into the new visible latitude/longitude inputs.** This is a new entry path for untrusted input; the player must be returned to their round, not dropped on a full-page error that loses it. *(Task 1)*

---

### Task 1: A guess that does not need a working map (P0)

**Files:**
- Modify: `web/assets/js/safari.js`
- Modify: `web/views/pages/play.templ` (the `Question` view's `.guess` section)
- Modify: `web/assets/css/app.css`
- Modify: `web/handlers/play.go` (`Guess`)
- Test: `web/views/pages/render_test.go` (create)
- Test: `web/handlers/play_test.go` (extend)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: nothing later tasks rely on.

- [ ] **Step 1: Write the failing render test**

Create `web/views/pages/render_test.go`, package `pages_test`. Render `pages.Question` with a sighting that has a photo and coordinates, into a `bytes.Buffer`, and assert the markup:

```go
func TestQuestion_OffersAGuessWithoutTheMap(t *testing.T) {
    // html contains the rendered Question view
    assert.Contains(t, html, `name="lat"`)
    assert.Contains(t, html, `name="lng"`)
    assert.Contains(t, html, `type="number"`)          // typed, not hidden
    assert.Contains(t, html, `aria-live="polite"`)     // the crosshair readout
    assert.NotContains(t, html, `id="lat" type="hidden"`)
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./web/views/pages/ -run TestQuestion -v`
Expected: FAIL — the latitude and longitude inputs are `type="hidden"`.

- [ ] **Step 3: Replace the hidden inputs with a labelled, visible pair**

In `play.templ`'s `Question`, replace the two `<input type="hidden" name="lat|lng">` with `<input type="number" step="any" min="-90" max="90" name="lat" id="lat">` and the longitude equivalent (`min="-180" max="180"`), each with a visible `<label>`, inside the `.guess` section beneath the map. `safari.js` already writes to `#lat`/`#lng`, so the map keeps working unchanged. Add a second `aria-live="polite"` element with id `crosshair-readout` beside the existing `#guess-readout`.

- [ ] **Step 4: Announce the crosshair as the map moves**

In `safari.js`'s `takeGuess`, subscribe to Leaflet's `moveend` and write `Crosshair at <lat>, <lng>` into `#crosshair-readout`, debounced to at most one update every 400ms. Enable the submit button whenever both inputs hold a parseable value, so a typed guess submits without touching the map.

- [ ] **Step 5: Style the new field pair**

Add `.coords` to `app.css`: a two-column row above `.guess__bar`, `--surface` background, `--edge` border to meet 3:1, labels in `--paper-dim` at the mono face, inputs at 16px or above so iOS does not zoom on focus.

- [ ] **Step 6: Write the failing handler test for a bad typed value**

In `web/handlers/play_test.go`, extend `TestGuess` with a subtest asserting that a malformed coordinate returns the player to their round rather than a dead-end error page:

```go
rec := guess(t, session, games, cookie, "0", "not-a-number", "36.82")
assert.Equal(t, http.StatusSeeOther, rec.Code)
assert.Equal(t, "/play", rec.Header().Get("Location"))
```

- [ ] **Step 7: Run it and watch it fail**

Run: `go test ./web/handlers/ -run TestGuess -v`
Expected: FAIL — currently 400 with a full-page error.

- [ ] **Step 8: Redirect a malformed guess back to the round**

In `Guess`, when `coordinates` returns an error, redirect `http://StatusSeeOther` to `/play` instead of calling `renderError`. The round is unscored and the cookie untouched, so the player keeps their game.

- [ ] **Step 9: Run the suite**

Run: `go tool templ generate ./web/... && go test ./... 2>&1 | grep -E "FAIL|ok.*(pages|handlers)"`
Expected: PASS for `web/views/pages` and `web/handlers`, no FAIL anywhere.

- [ ] **Step 10: Commit**

```bash
git add web/ && git commit -m "fix: let a round be answered without a working map"
```

---

### Task 2: The photograph and the map on one phone screen (P1)

**Files:**
- Modify: `web/assets/css/app.css`
- Modify: `web/views/pages/play.templ` (the `Question` view's source order)

**Interfaces:**
- Consumes: the `.coords` block from Task 1.
- Produces: nothing later tasks rely on.

- [ ] **Step 1: Reorder the round under 60rem**

In `play.templ`, the `Question` view's children must be in the order `.plate`, `.guess`, `.panel` in the DOM, so no CSS reordering is needed on mobile and the reading order matches the visual one. On desktop, restore the current two-column arrangement with `grid-template-areas` rather than source order.

- [ ] **Step 2: Shrink the plate and collapse the clues on mobile**

In the `max-width: 60rem` block: `.plate__photo { aspect-ratio: 16/9; min-height: 0; max-height: 34vh }`; render `.clues` as a wrapping row of chips rather than stacked rows; move `.panel__hint` below the map and cap it to one sentence.

- [ ] **Step 3: Make the submit bar sticky**

`.guess__bar { position: sticky; bottom: 0 }` with the page background and a top hairline in `--line`, so "Lock it in" is reachable without hunting.

- [ ] **Step 4: Verify co-visibility by measurement, not by eye**

Run `make web` (or `go run main.go web --port 8099`), open `/play` at a 375×812 viewport, and evaluate in the page:

```js
({photoBottom: document.querySelector('.plate').getBoundingClientRect().bottom,
  mapTop: document.querySelector('.map').getBoundingClientRect().top,
  overflow: document.documentElement.scrollWidth > document.documentElement.clientWidth})
```

Expected: `photoBottom` under 400, `mapTop` under 812 (both on screen at once), `overflow` false.

- [ ] **Step 5: Commit**

```bash
git add web/ && git commit -m "fix: put the photograph and the map on one phone screen"
```

---

### Task 3: Stop cropping the evidence, and stop shipping originals (P1)

**Files:**
- Modify: `web/views/pages/play.go` (add the size helper)
- Modify: `web/views/pages/play.templ`, `web/views/pages/home.templ` (use it, add dimensions)
- Modify: `web/assets/css/app.css`
- Test: `web/views/pages/render_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `pages.photoURL(m sighting.Media, size string) string` — used by Task 5's final-score map card.

- [ ] **Step 1: Write the failing test, including the non-iNaturalist case**

In `render_test.go` (package `pages_test`, so export the helper or test it through a rendered view — prefer a small exported `pages.PhotoURL`):

```go
func TestPhotoURL(t *testing.T) {
    inat := "https://inaturalist-open-data.s3.amazonaws.com/photos/1/original.jpeg"
    assert.Equal(t, ".../photos/1/medium.jpeg", pages.PhotoURL(media(inat), "medium"))
    other := "https://observation.org/photos/9.jpg"
    assert.Equal(t, other, pages.PhotoURL(media(other), "medium")) // untouched
    assert.Equal(t, "", pages.PhotoURL(sighting.Media{}, "medium"))
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./web/views/pages/ -run TestPhotoURL -v`
Expected: FAIL — undefined: `pages.PhotoURL`.

- [ ] **Step 3: Implement `PhotoURL(m sighting.Media, size string) string`**

Swap the final `/original.` path segment for `/<size>.`; return the URL unchanged when that segment is absent, and the empty string for empty media.

- [ ] **Step 4: Use it, with intrinsic dimensions**

Home tiles request `medium`; the round plate and the reveal request `large`. Every `<img>` in `home.templ` and `play.templ` gains `width` and `height` attributes matching its CSS aspect ratio, so nothing shifts as images land.

- [ ] **Step 5: Letterbox the plate so nothing is cut**

In `app.css`, `.plate__photo { object-fit: contain; background: var(--sunken) }`. The horizon, the edge vegetation and the sky are what the player is scored on reading, so they must all be in frame.

- [ ] **Step 6: Run the tests and look at a round**

Run: `go test ./web/views/pages/ -v && go tool templ generate ./web/...`
Expected: PASS. Then load `/play` and confirm the photograph is whole, not cropped.

- [ ] **Step 7: Commit**

```bash
git add web/ && git commit -m "fix: show the whole photograph, and stop serving originals"
```

---

### Task 4: Plain English in the clue panel (P1)

**Files:**
- Create: `internal/domain/weather/describe.go`
- Create: `internal/domain/weather/describe_test.go`
- Modify: `web/views/pages/play.go` (`taxon`, `condition`, `credit`)
- Test: `web/views/pages/render_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `weather.Describe(code int) string` — the human phrase for an Open-Meteo `weather_code`.

- [ ] **Step 1: Write the failing weather test**

`internal/domain/weather/describe_test.go`, package `weather_test`. The three commonest codes in the database are all mislabelled today; pin them to Open-Meteo's table, and pin the unknown-code fallback:

```go
assert.Equal(t, "Clear sky", weather.Describe(0))
assert.Equal(t, "Mainly clear", weather.Describe(1))
assert.Equal(t, "Partly cloudy", weather.Describe(2))
assert.Equal(t, "Overcast", weather.Describe(3))
assert.Equal(t, "Slight rain", weather.Describe(61))
assert.NotEmpty(t, weather.Describe(999)) // an unknown code must still read
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/domain/weather/ -run TestDescribe -v`
Expected: FAIL — undefined: `weather.Describe`.

- [ ] **Step 3: Implement `Describe(code int) string`**

A map from Open-Meteo `weather_code` to a short human phrase, covering the 17 codes present in the data (0,1,2,3,51,61,63,65,71,73,75,80,81,82,95,96,99) and the rest of Open-Meteo's published set. Unknown codes return a neutral phrase such as "Unsettled". Document in the file comment why this exists: `internal/clients/openmeteo/codes.go` numbers WMO table 4677 for manned stations, which is a different table from the one Open-Meteo's `weather_code` uses, and the stored `condition` strings came from the wrong one.

- [ ] **Step 4: Write the failing view tests**

In `render_test.go`, pin the three view helpers through a rendered `Question` (or export them):

```go
// taxon
assert.Equal(t, "Mammals", pages.Taxon(speciesWithClass("Mammalia")))
assert.Equal(t, "Birds", pages.Taxon(speciesWithClass("Aves")))
assert.NotEmpty(t, pages.Taxon(speciesWithClass("")))     // falls through, never blank
// credit: both cases must be rejected as an answer leak
assert.Equal(t, "Photographer unrecorded", pages.Credit(mediaBy("MALACOCHERSUS"), "Malacochersus tornieri"))
assert.Equal(t, "Photographer unrecorded", pages.Credit(mediaBy("Malacochersus"), "Malacochersus tornieri"))
assert.Equal(t, "Jos Simons", pages.Credit(mediaBy("Jos Simons"), "Ara macao"))
```

- [ ] **Step 5: Run them and watch them fail**

Run: `go test ./web/views/pages/ -v`
Expected: FAIL — the helpers are unexported and take different arguments.

- [ ] **Step 6: Implement the three helpers**

`taxon` maps the taxonomic classes present in the data to plain English ("Mammalia" → "Mammals", "Aves" → "Birds", "Testudines" → "Turtles", "Squamata" → "Lizards and snakes", "Crocodylia" → "Crocodiles", "Elasmobranchii" → "Sharks and rays"), keeping the Latin as a `title` attribute and falling through to phylum then "Unrecorded". `condition` takes its text from `weather.Describe(s.Weather.Condition.Code)` rather than the stored string. `credit` gains the sighting's scientific name as a second argument and returns "Photographer unrecorded" when the credit case-insensitively matches the genus or the scientific name.

- [ ] **Step 7: Run the suite**

Run: `go tool templ generate ./web/... && go test ./... 2>&1 | grep -E "FAIL|ok.*(weather|pages)"`
Expected: PASS, no FAIL.

- [ ] **Step 8: Commit**

```bash
git add internal/ web/ && git commit -m "fix: say what the weather and the group actually were"
```

---

### Task 5: An ending worth passing on (P2)

**Files:**
- Modify: `internal/domain/game/game.go` (`Rank`, `Share`, add `Best`)
- Modify: `internal/domain/game/game_test.go`
- Modify: `web/views/pages/play.go`, `play.templ` (the `Over` view)
- Modify: `web/handlers/play.go` (pass the request host)

**Interfaces:**
- Consumes: `pages.PhotoURL` from Task 3.
- Produces: `game.Game.Best() (Result, int, bool)`; `game.Game.Share(url string) string`.

- [ ] **Step 1: Write the failing game tests**

In `game_test.go`:

```go
func TestGame_Share_CarriesTheURL(t *testing.T) {
    assert.Contains(t, g.Share("https://example.test/play"), "https://example.test/play")
}
func TestGame_Best(t *testing.T) {
    // the highest-scoring round, its index, and false for a game with no results
    r, i, ok := g.Best()
    assert.True(t, ok)
    assert.Equal(t, 4976, r.Points)
    assert.Equal(t, 2, i)
    _, _, ok = game.Game{}.Best()
    assert.False(t, ok)
}
func TestGame_Rank_IsNeverAPutDown(t *testing.T) {
    for _, banned := range []string{"Hopeless", "Lost in the Bush"} {
        for _, total := range []int{0, 1500, 6000, 12000, 18000, 22500, game.MaxScore} {
            assert.NotContains(t, (game.Game{Results: []game.Result{{Points: total}}}).Rank(), banned)
        }
    }
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test ./internal/domain/game/ -v`
Expected: FAIL — `Share` takes no argument, `Best` undefined, the bottom ranks are the banned strings.

- [ ] **Step 3: Implement the three changes**

`Share(url string) string` appends the URL on its own line after the squares. `Best() (Result, int, bool)` returns the highest-scoring result and its zero-based round index, false when no rounds are scored. `Rank`'s two bottom bands become warmer, still witty names — the spec's wording is "something a person would repeat"; keep the upper bands as they are.

- [ ] **Step 4: Headline the best round and map the five places**

`OverProps` gains `URL string` and the sightings map it already carries is reused to render a single `MapData` with the five true positions and the five guesses. Above the rounds list, state the best round in words — "Your best: 7.2 km from Nairobi" — built from `Best()` and `distanceLabel`. `Over` renders `props.Game.Share(props.URL)`.

- [ ] **Step 5: Fill the URL from the request**

In `Play`'s complete-game branch, set `URL` from the request's scheme and `r.Host` plus `/play` rather than a hardcoded host.

- [ ] **Step 6: Run the suite and play to the end**

Run: `go tool templ generate ./web/... && go test ./... 2>&1 | grep -E "FAIL|ok.*game"`
Expected: PASS. Then play five rounds and confirm the share block contains a clickable address and the best round is named.

- [ ] **Step 7: Commit**

```bash
git add internal/ web/ && git commit -m "feat: an ending that travels and does not scold"
```

---

### Task 6: The polish the critique listed (P3)

**Files:**
- Modify: `internal/domain/game/game.go` (`Medal` bands)
- Modify: `web/views/pages/play.go` (`tone` bands)
- Modify: `web/assets/css/app.css` (pips, dead classes)
- Modify: `web/views/pages/error.templ`
- Delete: `web/views/components/gallery.templ`, `web/views/components/sighting_card.templ` and their generated files
- Modify: `web/views/layout/layout.templ` (`BaseProps.Class`)

**Interfaces:**
- Consumes: nothing.
- Produces: nothing.

- [ ] **Step 1: Write the failing banding test**

In `game_test.go`, pin the two band sets to each other so they cannot drift again:

```go
func TestMedalAndToneAgree(t *testing.T) {
    for points := 0; points <= game.MaxRoundScore; points += 50 {
        assert.Equal(t, game.Medal(points) == "🟩", pages.ToneFor(points) == "is-near")
    }
}
```

Place it where both packages are importable — `web/views/pages/render_test.go` is the right home, since `pages` already imports `game`.

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./web/views/pages/ -run TestMedalAndTone -v`
Expected: FAIL — `Medal` bands at 4000/2500/1000, `tone` at 4000/1500.

- [ ] **Step 3: Align the bands and clear the rest**

Make `tone`'s thresholds match `Medal`'s. Give `.pip--done` a visibly different fill and border from an unplayed pip. Either style `is-playing`/`is-error` or delete `BaseProps.Class` and its comment. Delete the two unused components and the `.gallery__*` and `.card*` rules that only they used. Give `/nope` a heading and a paragraph that say different things — "Nothing was recorded here" is the heading; the paragraph should offer the way back.

- [ ] **Step 4: Run everything**

Run: `go tool templ generate ./web/... && go vet ./... && go test ./... 2>&1 | grep -E "FAIL|^---"`
Expected: no output — vet clean, no failures.

- [ ] **Step 5: Re-run the detector**

Run: `"/Users/theforge/.claude/plugins/cache/impeccable/impeccable/4.5.0/skills/impeccable/scripts/impeccable" detect --json web/views web/assets`
Expected: no new findings beyond the known `overused-font` (Inter), which is a separate `typeset` decision.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "refactor: clear the polish the critique listed"
```

---

## Self-review notes

- **Spec coverage:** Phase 1's six numbered items map to Tasks 1–6 in order. Phase 0 and Phases 2–5 are out of scope for this plan by the agreed split.
- **Review Focus coverage:** item 1 → Task 3 Step 1; items 2–4 → Task 4 Steps 1 and 4; item 5 → Task 1 Step 6.
- **Type consistency:** `PhotoURL`, `Taxon`, `Credit` and `ToneFor` are exported because the tests live in `pages_test`; the plan uses those exact names throughout. `Share` takes a `url string` in Tasks 5 and nowhere earlier.
- **Known cost:** Task 6 deletes `components.SightingCard`, which the workshop's `/design-site` skill references in its block documentation. That is acceptable in a fork, but worth a line in the commit message.

---

## Addendum — tasks added from the re-critique (29/40)

The first critique's P0 and P1s are closed. A second critique at `c9c4dc9` scored
29/40 and found that the two remaining P1s are about **order**, not correctness:
the answer screen and the ending both bury their best moment below the fold. It
also found that the P0 fix cost a point on minimalism, which is recoverable. The
owner chose to do these plus Task 6, then re-critique.

### Task 7: Lead with the reward

**Files:**
- Modify: `web/views/pages/play.templ` (the `Answer` and `Over` views)
- Modify: `web/assets/css/app.css`
- Test: `web/views/pages/render_test.go`

**Interfaces:**
- Consumes: `game.Game.Best()`, `pages.PhotoURL` from Tasks 3 and 5.
- Produces: nothing later tasks rely on.

- [ ] **Step 1: Write the failing order tests**

In `render_test.go`, assert position rather than mere presence — these are
ordering defects, so only order catches a regression:

```go
// Answer: the species name precedes the map, and "Next round" is reachable.
assert.Less(t, strings.Index(html, speciesName), strings.Index(html, `class="map`))
// Over: the share block and Play again precede the rounds list.
assert.Less(t, strings.Index(html, `class="share`), strings.Index(html, `class="rounds`))
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test ./web/views/pages/ -v`
Expected: FAIL — the reveal currently follows the map, and the share follows the rounds.

- [ ] **Step 3: Lead the answer screen with the reveal**

Move the species name, binomial and a small photo (`PhotoURL(..., "medium")`)
into or directly beneath `.result__head`, above the map. Measured before: species
at y=949 and "Next round" at y=1287 on an 812px phone. Target: the species name
on the first screen.

- [ ] **Step 4: Make "Next round" sticky**

Give the answer screen a sticky action bar matching `.guess__bar`'s structure and
tokens, so the next step is never hunted for. Respect the `max-height: 30rem`
landscape rule that makes `.guess__bar` static.

- [ ] **Step 5: Reorder the ending**

Order the Over view: score, rank, best line, **share and Play again**, then the
rounds list and the map. Retitle the share block from "Brag about it" to
something that fits a 955/25,000 run — the product's voice is dry and
affectionate, not boastful.

- [ ] **Step 6: Stop painting a weak rank in alarm**

`gameTone` currently gives a low total `is-far`, so the rank renders in `--clay`.
Leave the per-round rows toned, but render the rank itself in `--paper` regardless
of total.

- [ ] **Step 7: Run the tests and play a full game**

Run: `go tool templ generate ./web/... && go test ./... 2>&1 | grep -E "FAIL|^---"`
Expected: no output. Then play five rounds and measure the species name's and the
share block's positions at 375×812.

- [ ] **Step 8: Commit**

```bash
git add web/ && git commit -m "fix: lead with the animal, and let the ending be shared"
```

### Task 8: Collapse the fallback when the map works

**Files:**
- Modify: `web/views/pages/play.templ` (the `Question` view's `.coords` block)
- Modify: `web/assets/js/safari.js`
- Modify: `web/assets/css/app.css`
- Test: `web/views/pages/render_test.go`

**Interfaces:**
- Consumes: the `.coords` block and `#crosshair-readout` from Tasks 1 and 2.
- Produces: nothing.

- [ ] **Step 1: Write the failing test**

The no-JavaScript guarantee is that the fallback ships **open**. Assert the
rendered markup contains a `<details open>` wrapping the coordinate inputs:

```go
assert.Contains(t, html, "<details")
assert.Regexp(t, `<details[^>]*\sopen`, html)   // open in the markup, closed by JS
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./web/views/pages/ -v`
Expected: FAIL — the coordinates are an always-visible block.

- [ ] **Step 3: Wrap the coordinates in a `<details open>`**

Summary text names the route ("Enter coordinates instead"). It ships open so a
visitor with no JavaScript, or a blocked Leaflet, sees the fields immediately.

- [ ] **Step 4: Close it once Leaflet is live**

In `safari.js`'s `takeGuess`, after the map is confirmed, remove the `open`
attribute. The fallback is then one click away rather than permanently occupying
the screen. Do not hide it — closing a `<details>` keeps it in the accessibility
tree and keyboard-reachable.

- [ ] **Step 5: Restyle the readouts as status, not fields**

`.field__readout` currently shares the border and fill of a real `<input>`, so a
status line reads as something you can type into. Render both readouts as plain
text in `--paper-dim` with no border or box.

- [ ] **Step 6: Verify both paths**

Run `go tool templ generate ./web/... && go test ./web/views/pages/ -v`. Then load
`/play` with Leaflet reachable (the details should be closed) and with it blocked
(it should be open and usable), and confirm a round can still be scored by typing.

- [ ] **Step 7: Commit**

```bash
git add web/ && git commit -m "refactor: offer the typed route without it crowding the map"
```

### Task 9: Tell a first-timer how to play

**Why:** `Help and documentation` scored 2/4 in both critiques and was the only
heuristic that did not move. Nothing visible states the rules, and the share link
lands a cold visitor on `/play` with no hero and no context.

**Files:**
- Modify: `web/views/pages/play.templ` (the `Question` view), `home.templ`
- Modify: `web/assets/css/app.css`
- Test: `web/views/pages/render_test.go`

- [ ] **Step 1: Write the failing test**

```go
// The rules are stated where a first-timer will actually be.
assert.Contains(t, html, "Five rounds")
assert.Regexp(t, `5,?000`, html)   // what a perfect round is worth
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./web/views/pages/ -v` — FAIL, the rules appear nowhere on `/play`.

- [ ] **Step 3: State the rules on the round, once**

One visible line near the round label: five rounds, closer scores more, up to
5,000 a round. It must be visible to a sighted phone player, not clipped into a
screen-reader-only span.

- [ ] **Step 4: Greet a first-timer only**

On round 1 of a fresh game, show a short dismissible panel naming the three ways
to answer and what scores. Suppress it from round 2 onward — the game is already
in a signed cookie, so `g.Round == 0` is the signal; no new state and no
JavaScript required to decide it.

- [ ] **Step 5: Give the home page a one-line "how it works"**

Three steps beneath the hero CTA: look at the photograph, drop a pin, score by
distance. This is the context a share link currently skips.

- [ ] **Step 6: Verify and commit**

Run `go tool templ generate ./web/... && go test ./... 2>&1 | grep -E "FAIL|^---"`,
then load `/play/new` and confirm the greeting shows on round 1 and not round 2.

```bash
git add web/ && git commit -m "feat: tell a first-timer how the game works"
```

### Task 10: Gophers

**Why:** it is GopherCon, the judge is a Go advocate, and the product has no
moment of pure character. Requested by the owner.

**Scope decision (the owner's brief, as interpreted):** gophers appear only where
the page is quiet. **Not on the round screen** — the critique already found that
screen crowded, and attention there belongs to the photograph and the map.

**Files:**
- Create: `web/views/components/gopher.templ` (drawn SVG, no emoji, no raster)
- Modify: `web/views/pages/error.templ`, `home.templ`, `play.templ` (answer screen only)
- Modify: `web/assets/css/app.css`
- Test: `web/views/pages/render_test.go`

- [ ] **Step 1: Write the failing test**

```go
// A gopher appears on the quiet screens and never on the round.
assert.Contains(t, errorHTML, `class="gopher`)
assert.NotContains(t, questionHTML, `class="gopher`)
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./web/views/pages/ -v` — FAIL, no gopher exists.

- [ ] **Step 3: Draw the gopher**

An inline SVG component in the site's palette, built from the existing tokens.
Two or three poses is enough. It is a mark in this product's world, not the
Go mascot copied — keep it simple enough to read at 40px.

- [ ] **Step 4: Set them running where it is quiet**

A CSS keyframe run across the footer, one on the 404, one in the empty state, and
one that scampers across the answer screen after a guess is scored. Transform and
opacity only — no layout properties — so nothing re-lays-out per frame.

- [ ] **Step 5: Respect reduced motion**

The stylesheet's `prefers-reduced-motion` block already parks animations globally.
Confirm the gophers are still *visible* when parked rather than mid-run off-screen:
a reduced-motion visitor should see a gopher standing still, not an empty strip.

- [ ] **Step 6: Verify and commit**

Run the suite and the detector
(`"<skill-base-dir>/scripts/impeccable" detect --json web/views web/assets`),
and confirm no new findings.

```bash
git add web/ && git commit -m "feat: a few gophers, where the page is quiet"
```
