# Globe and Flow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Give the site somewhere to go: a 3D globe of the whole record, a page per record, and a paginated front page whose tiles lead somewhere.

**Architecture:** Three new routes on the existing `net/http` mux, server-rendered with templ, following the handler/view split already in `web/`. The globe is a client-side three.js scene fed by a JSON endpoint; everything else is plain server-rendered HTML.

**Tech Stack:** Go 1.27, templ, sqlc/SQLite, three.js r160 from cdnjs, the existing design tokens in `web/assets/css/app.css`.

**Spec:** `docs/superpowers/specs/2026-10-15-sightings-platform-design.md` (Phase 2, previously unbuilt)

## The problem this fixes

Measured on the running app:

- The site is two pages: `/` and `/play`.
- `/` is 6,562px tall and renders all 481 sighting tiles in one scroll.
- Those tiles are `<article>` elements with no links. 481 dead ends.
- `sighting.ListFilter` has `Limit` but no `Offset`, which is why pagination was never possible.

## Global Constraints

- **No Claude attribution in commit messages.** No `Co-Authored-By`, no "generated with". Match the existing log: short imperative subject, prose body explaining why.
- Go 1.27: `uuid` is a **stdlib** package here; `uuid.Nil` is a function, `uuid.Nil()`.
- After editing any `.templ`, run `go tool templ generate ./web/...` and commit the generated `_templ.go` alongside. Never hand-edit a generated file.
- Design tokens only: `--gold` interactive and focus ONLY, `--moss`/`--clay` measurement, `--paper`/`--paper-dim` text, `--surface`/`--raised`/`--sunken` backgrounds, `--line` quiet structure, `--edge` control bounds. No new raw colours.
- Keyboard and screen-reader parity: anything clickable is reachable and operable by keyboard, and the globe must have a non-WebGL fallback.
- A dev server runs on :8099 and the user's own on :8081. Do not start, stop or restart either.
- `prefers-reduced-motion: reduce` must stop globe auto-rotation.

## Review Focus

- A record id that is not a uuid, or a uuid that does not exist → 404 page, never a panic or a 500.
- `?page=` that is negative, zero, enormous, or not a number → clamped to a valid page, never an empty grid or an error.
- A sighting with no coordinates, no photograph, or no species name → still renders on the globe list and the detail page, with the gap stated rather than blank.
- WebGL unavailable or three.js blocked → the globe page still lists every sighting and says the visual is unavailable.
- The JSON feed for 500 sightings → one request, not 500.

---

### Task 1: A page per record

**Files:**
- Create: `web/handlers/record.go`, `web/views/pages/record.templ`
- Modify: `web/web.go` (route), `web/assets/css/app.css`
- Test: `web/handlers/record_test.go`, `web/views/pages/render_test.go`

**Interfaces:**
- Consumes: `sighting.Service.Find(ctx, id) (Sighting, error)`, already present at `internal/domain/sighting/sighting.go:84`.
- Produces: `handlers.Record(logger, finder RecordFinder) http.HandlerFunc`, route `GET /record/{id}`, and a `pages.Record(props RecordProps)` view. Task 2 and Task 3 both link to `/record/{id}`.

- [ ] **Step 1: Write the failing handler tests**

Table-driven: a known id renders 200 and contains the species name; a well-formed uuid that is absent renders 404; a malformed id (`"not-a-uuid"`) renders 404 and does not panic.

- [ ] **Step 2: Run them and watch them fail**

- [ ] **Step 3: Implement `Record`**

Parse `r.PathValue("id")`, 404 on a parse error rather than 500 — a bad id in a URL is a wrong address, not a server fault.

- [ ] **Step 4: Build the view**

Photograph (reuse `PhotoURL` and `Credit` from `web/views/pages/play.go`), common and scientific name, place and country, date, the weather that day, a Leaflet map at the coordinates, and the recording where one exists (reuse the `Answer` reveal's audio block). Every absent field states its absence rather than rendering blank.

- [ ] **Step 5: Register the route and commit**

---

### Task 2: A front page you can get through

**Files:**
- Modify: `internal/domain/sighting/filter.go`, the sqlite store and its query, `web/handlers/home.go`, `web/views/pages/home.templ`, `web/assets/css/app.css`
- Test: the store's test, `web/handlers/home_test.go`

**Interfaces:**
- Consumes: Task 1's `/record/{id}`.
- Produces: `ListFilter.Offset int`, and `Count(ctx, filter) (int, error)` on the sightings store and service.

- [ ] **Step 1: Write the failing store test**

`Offset` skips the first N; `Limit` with `Offset` returns a stable, non-overlapping window across pages (order must be deterministic).

- [ ] **Step 2: Add `Offset` to `ListFilter` and the query; add `Count`**

- [ ] **Step 3: Write the failing handler test**

`?page=2` returns a different set than `?page=1`; `page=0`, `page=-3`, `page=abc` and `page=99999` all clamp to a valid page and return 200.

- [ ] **Step 4: Paginate the home page**

24 tiles a page. Previous/next controls with the page position stated ("Page 2 of 21"). Keep the hero figures describing the whole record, not the page — use `Count`.

- [ ] **Step 5: Make the tile a link**

The whole tile links to `/record/{id}`. One link per tile, not a nested link, so the keyboard reaches it once.

- [ ] **Step 6: Commit**

---

### Task 3: The globe

**Files:**
- Create: `web/handlers/globe.go`, `web/views/pages/globe.templ`, `web/assets/js/globe.js`
- Modify: `web/web.go`, `web/assets/css/app.css`
- Test: `web/handlers/globe_test.go`, `web/views/pages/render_test.go`

**Interfaces:**
- Consumes: `sighting.Service.List`, Task 1's `/record/{id}`.
- Produces: routes `GET /globe` and `GET /globe/sightings.json`.

- [ ] **Step 1: Write the failing feed test**

`GET /globe/sightings.json` returns one JSON array of every sighting that has coordinates, each with id, latitude, longitude, common name, scientific name, class, country and thumbnail. A sighting without coordinates is omitted from the feed but still counted in the page's list.

- [ ] **Step 2: Implement the feed**

- [ ] **Step 3: Write the failing view test**

The page renders every sighting as a list item linking to `/record/{id}` **in the HTML itself**, so the record is browsable with no JavaScript at all. The legend names each class present with its count.

- [ ] **Step 4: Build the page**

Left: the globe canvas. Right: the legend (one row per class, its colour swatch and count) and the scrollable list. Clicking a list row focuses that point on the globe; clicking a point shows a tooltip with the common name, place and date, and a "View record" link to `/record/{id}`.

- [ ] **Step 5: Build the globe**

three.js r160 from cdnjs, pinned. A sphere, coastlines from the world-atlas TopoJSON already used by the game, and one point per sighting coloured by class. Drag to rotate, scroll to zoom, raycaster for clicks. Auto-rotate slowly, stopped by `prefers-reduced-motion: reduce` and by any interaction.

Guard exactly as `safari.js` does: if `THREE` is undefined or WebGL is unavailable, leave the list and legend alone and say the visual could not load. The list is the fallback, so it is always in the HTML.

- [ ] **Step 6: Commit**

---

### Task 4: Navigation that admits the other pages exist

**Files:**
- Modify: the masthead in the layout, `web/views/pages/home.templ`, `web/assets/css/app.css`
- Test: `web/views/pages/render_test.go`

- [ ] **Step 1: Write the failing test**

Every page's masthead offers The record, The globe and Play, and marks the current one with `aria-current="page"`.

- [ ] **Step 2: Add the links, and a route into the globe from the hero**

- [ ] **Step 3: Commit**
