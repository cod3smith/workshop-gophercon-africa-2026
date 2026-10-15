# Sightings Safari — from game to platform

**Date:** 2026-10-15
**Status:** design, approved decisions recorded; ready to plan
**Repo:** `cod3smith/workshop-gophercon-africa-2026` (fork of `ainsleyclark/workshop-gophercon-africa-2026`)

## Intent

The app today is a guessing game bolted onto a catalogue of 500 GBIF wildlife
occurrence records. The game is good and the catalogue is filler. This work turns
the catalogue into the product: something a hobby wildlife spotter would actually
open before a weekend, and something they can add their own sightings to.

The reason this is worth building, and the reason it is not just another map of
pins: **an occurrence record here can carry the weather at that exact place and
hour**, joined from Open-Meteo at ingest. That turns "here is where animals were
seen" into "here is when, and in what conditions, this animal gets seen" — the
question a spotter actually asks.

That premise depends on the weather being real, and in the currently committed
database it is not. See **Phase 0**; it is a prerequisite, not a nicety.

The game stays. It becomes the way people meet the data rather than the whole of
the product.

## Goals

1. A hobby spotter can answer four questions about any species, from real data:
   - **When should I go?** Which months this species is recorded in, in a place.
   - **Is today a good day?** The conditions this species is recorded in —
     temperature, cloud, rain.
   - **How unusual is this?** A rarity score for a species in a region.
   - **What else is around?** Species recorded near the same place and time.
2. Every record is reachable: a detail page, and pagination through the whole set
   rather than the most recent sixty.
3. A visitor can add a sighting of their own, with a position picked on a map.
4. The record becomes a 3D globe of clustered, vector-pinned sightings, with a
   list as the second view.
5. The critique's P0 and P1 defects are fixed before any of the above ships.

## Non-goals

- Accounts or authentication. A submitted sighting is public and anonymous.
  "My life list" is explicitly out of scope.
- Editing or deleting sightings through the web UI. (Removal still needs *some*
  path — see "What's left to the builder".)
- Mobile-native anything. This is a server-rendered site.
- Real-time or live-updating anything.
- Replacing GBIF ingest. Submitted sightings live alongside ingested ones.
- IUCN Red List integration. It needs a registered key with organisation details
  and non-instant approval; GBIF's `conservation` description type covers enough.

## Anti-goals

Things that would make this a failure even if every feature worked:

- **A dashboard.** Four charts in a grid with no sentence telling the spotter what
  to do is a failure. Every analysis must resolve to a plain-language answer.
- **Statistics presented as certainty.** The sample is small and biased
  (contributor-submitted, not a survey). Any figure drawn from fewer than the
  agreed threshold must say so on the page, not in a footnote.
- **Losing the game.** If the game gets slower, harder to find, or less fun as a
  result of this work, the work was wrong.
- **A page that needs JavaScript to read.** The globe is an enhancement; the list,
  the detail page and the analyses must render server-side.
- **Breaking the workshop app.** `make web`, `make test` and the existing CLI must
  keep working; this is still a fork of a teaching repository.

## Constraints

- Go, `templ`, `sqlc`, SQLite. No JS framework. Server-rendered.
- Third-party origins already in play: Google Fonts, cdnjs (Leaflet), OSM tiles,
  iNaturalist photos. GBIF species enrichment adds `api.gbif.org`. Every one must
  degrade: the page works without it.
- The repo is judged on what is pushed to the fork's default branch.

## Known data defects (verified, 2026-10-15)

These were measured against the committed `workshop.db`, not inferred. They are
not optional context; three of them silently break the obvious implementation.

1. **The weather is generated, not observed.** `db seed` uses the bundled mock
   unless `--live` is passed, and the mock's generator is built on `hash/fnv` and
   `math/rand/v2` — a deterministic hash of lat/lon/hour. Phase 0 exists for this.
2. **SQLite cannot parse the stored datetimes.** `happened_at` and `observed_at`
   are in Go's `time.Time.String()` format (`2024-03-28 18:47:00 +0000 UTC`).
   `strftime('%m', happened_at)` and `julianday(happened_at)` return NULL on every
   row — no error, just empty results. (`created_at` is fine; SQLite writes it.)
3. **The weather descriptions are mislabelled, not merely raw.** The stored
   `condition` strings come from WMO table 4677 (manned stations) while
   `condition_code` holds Open-Meteo `weather_code` values. Code 1 reads "Clouds
   dissolving or becoming less developed" but means *Mainly clear*; code 2 reads
   "State of sky on the whole unchanged" but means *Partly cloudy*; code 3 reads
   "Clouds generally forming or developing" but means *Overcast*. Those three codes
   alone are **314 of 500 records**.
4. **`sightings.gbif_key` is `NOT NULL UNIQUE` with no default** and
   `CreateParams.Validate` never sets it, so the first submission stores 0 and the
   second collides, surfacing to the visitor as "sighting already exists".
5. **`sightings.weather_id` is `NOT NULL`, and all three read queries inner-join
   weather.** A weatherless sighting would be accepted and then be invisible to the
   list, the globe, the figures and the game pool.
6. **The data is thin.** Max 13 records per species, mean 6.4, five species with
   exactly one. Within 55km and 30 days there are 28 different-species pairs in the
   entire set. Phase 0 addresses this too.

## Decisions already made

Decided by the owner; not open:

- **The record becomes the 3D globe**, with the list as the second view. Vector
  icons per taxonomic group, clustered with a count, splitting as you zoom. Click a
  pin for a tooltip, "view more" through to the detail page.
- **The globe lives on its own route**, not the front door. `/` keeps the hero, the
  figures and the Play button — the game stays findable and the front page stays
  readable without WebGL.
- **All four analyses ship**: seasonality, conditions, rarity, co-occurrence.
- **Re-seed from the live APIs**, so the weather is real and the conditions analysis
  means something.
- **Grow the seed** well beyond 500, so the analyses have enough records to say
  anything.
- **Submitted sightings are marked and counted** — shown as community-contributed
  with their own pin treatment, and included in the figures, the globe and the
  analyses.
- **Critique fixes first, in severity order**, before new features.
- **The game's bottom ranks get warmer, still witty names.**
- Attribution lines are not added to commits in this repository.

## Decisions taken by the author of this document

Overturn any of these; they were settled to avoid a second round of questions.

- **A `/species/{key}` page is in scope** (Phase 2). Goal 1 cannot work without it:
  otherwise a spotter who cares about lions must first find a lion sighting, and
  "in a place" becomes wherever that one record happened to be. The analyses take
  `(species, place)`, not `(sighting)`.
- **Correct the weather mapping rather than whitelist it.** Re-map from
  `condition_code` against Open-Meteo's table at render time. This supersedes the
  "whitelist the descriptions" wording in Phase 1 fix #4.
- **Aggregate in the domain, not in SQL**, given defect 2. `sighting.Summarise` is
  the precedent. If a query must move to SQL later, wrap date columns in
  `substr(happened_at, 1, 19)` rather than migrating the committed database.
- **"Is today a good day?" ships historical-only first.** The Open-Meteo client is
  archive-only (`archive-api.open-meteo.com`); a live forecast needs a new origin, a
  call on page render, and a source for the viewer's location — all of which run at
  the conference-wifi risk. The live comparison is a later addition, not Phase 3.
- **The share URL reads the request host** rather than hardcoding a deployment URL.

## The work, in order

### Phase 0 — make the data true (prerequisite for Phase 3)

Re-seed from the live APIs at a larger size, and commit the resulting
`workshop.db`. This is one offline run, not a code change: the seed limit is a
constant in `internal/cmd/db.go`, and `--live` switches both clients off the mock.

It fixes defects 1 and 6 together. Expect roughly one GBIF call and one Open-Meteo
archive call per record, both rate-limited, so the run is slow and should be done
once, deliberately, with the output committed. Pick the size against how long the
run takes and how large the committed database may reasonably get — see "What's
left to the builder".

### Phase 1 — critique fixes

Severity order, from `.impeccable/critique/2026-10-15T09-49-20Z__web.md`:

1. **[P0] No way to answer without a working map.** The crosshair position is never
   announced, so a screen-reader user can place a pin but not aim it; with Leaflet
   blocked there is no path to a guess at all. Announce the crosshair on `moveend`
   into a polite live region, and add optional visible latitude/longitude number
   inputs bound to the existing hidden fields — which also gives a no-JavaScript path.
2. **[P1] Phone layout.** At 375×812 the photograph ends at y=532 and the map starts
   at y=994. Under 60rem: photo → map → clues, plate capped to a 16:9 strip, clues
   collapsed to a chip row, sticky submit bar.
3. **[P1] The game crops its own evidence.** `.plate__photo` is `object-fit: cover`,
   cutting horizon, edge vegetation and sky — what a geo-guesser reads. Use
   `object-fit: contain` on a letterboxed plate. Separately, every image is a 2048px
   original with no intrinsic dimensions: use `medium`/`large` variants, add
   `width`/`height`.
4. **[P1] Raw and wrong database strings.** Plain-English taxon names; **correct**
   the weather descriptions per defect 3; reject photographer credits matching the
   sighting's own genus (both a broken attribution promise and an answer leak).
5. **[P2] The ending.** Site URL in `Share()` from the request host. Warmer bottom
   ranks. The best round in the headline. The five true locations on one map.
6. **[P3] Polish.** Dead `is-playing`/`is-error` body classes; `Medal()` bands at
   4000/2500/1000 against `tone()` at 4000/1500; played and unplayed pips nearly
   identical; unused `components.Gallery` and `sighting_card.templ`; the `/nope`
   tautology.

### Phase 2 — the record

- **Globe view**, on its own route. Dark sphere, coastlines from world-atlas
  TopoJSON, vector pins per `species.class`, clustered by a lat/lng grid whose cell
  size follows the zoom. Single pins show the group glyph; clusters show a count in
  the dominant group's colour. Clicking a cluster zooms; clicking a pin opens a
  tooltip with species, place, date and a link to the detail page. A legend lists
  groups with glyph and count, and filters. Proven in a throwaway prototype at
  `.superpowers/brainstorm/50482-1792056212/content/globe-markers.html` — reference,
  not code to lift.
- **List view**, with real pagination over the whole set. This is the accessible,
  no-JavaScript view and must carry the same records.
- **Record detail page** at `/sightings/{id}`: photograph uncropped, species with
  binomial, place, coordinates, date, the weather that day, the photographer credit,
  a link to the original GBIF record.
- **Species page** at `/species/{key}`: the four analyses, GBIF enrichment, and the
  records behind them. This is where Goal 1 is actually met.

### Phase 3 — the four analyses

All derived from existing columns; none needs new ingest beyond Phase 0.

- **Seasonality — "when should I go?"** Sightings per month for a species,
  optionally narrowed to a country. Twelve bars, with the peak months named in a
  sentence.
- **Conditions — "is today a good day?"** The distribution of `temperature`,
  `cloud_cover` and `precipitation` across that species' sightings against the
  distribution across all sightings, so the page says what is *distinctive* rather
  than what is merely common. Resolves to a sentence.
- **Rarity.** How many records exist of this species within a radius and in the
  country, relative to the busiest species there. A word and a number, not a
  percentile.
- **Co-occurrence.** Species recorded within a radius and time window, ranked by how
  often they appear together relative to how common each is alone, so an abundant
  species does not top every list.

### Phase 4 — adding a sighting

`GET /sightings/new` and `POST /sightings`. `sighting.Service.Create` and
`CreateParams.Validate` exist, but defects 4 and 5 mean this is not only a form:

- `gbif_key` needs a scheme for records that have no GBIF key, or a migration that
  relaxes the constraint.
- `weather_id` is `NOT NULL` and every read query inner-joins weather, so a
  submission must either carry weather or the schema and queries must change.
- `occurrence_id`, `basis_of_record`, `recorded_by` and `reference_url` are all
  `NOT NULL` and need sensible values a visitor will not supply.

Submissions are **marked and counted**: a source marker on the record, their own
pin treatment on the globe, and included in the figures and the analyses. Species
is chosen from those already in the database; resolving arbitrary new species
against GBIF is out of scope.

### Phase 5 — GBIF species enrichment

Verified working against `api.gbif.org/v1`, no API key, keyed by the `gbif_key`
already on every species row:

- `/species/{key}/descriptions` — typed, mostly English: `activity`,
  `biology_ecology`, `breeding`, `conservation`, `distribution`, `food_feeding`.
  The `activity` text gives time-of-day behaviour the local data cannot.
- `/species/{key}/vernacularNames`, `/distributions`, `/speciesProfiles`.

Shown on the species page beside the local analyses, attributed to GBIF, and cached
so a page view is not a third-party round trip.

## What's left to the builder

- **The seed size for Phase 0**, traded against run time and committed database size.
- **The small-sample threshold** below which an analysis says "too few records to
  say", and how that is worded.
- **Radius and time window** for rarity and co-occurrence.
- **How submissions satisfy the NOT NULL columns** — synthetic keys versus a
  migration — and whether they fetch weather at submission.
- **How a bad submission is removed.** Writes are public, anonymous and unmoderated,
  and there is no `Delete` on `sighting.Store`. A CLI path is probably enough, but
  something must exist before the endpoint opens.
- **How GBIF enrichment is cached** — against the species row, or in memory with a TTL.
- **Chart rendering**: inline SVG from Go, or the design system's existing CSS bar
  pattern. No charting library.
- **Whether the globe or the list is the default view** on the record route.
- All naming, package layout, and test strategy, consistent with
  `internal/domain/README.md`.

## Risks

- **Scope.** Six phases is a lot. Phase 0 gates Phase 3. Phases 1 and 2 change how
  the product is judged; 3 makes it distinctive; 4 and 5 are additive. Stopping
  cleanly after any phase must leave a working site.
- **Phase 0 is a slow, rate-limited run** against two external APIs, and its output
  is a committed binary. If it fails halfway the database must not be left
  half-seeded and committed.
- **Even after growing the seed, some species will be below the threshold.** The
  page must be designed for "too few records to say" as a normal state.
- **Conference wifi.** Every new third-party dependency is a new way for the page to
  be blank in the room where it matters.
