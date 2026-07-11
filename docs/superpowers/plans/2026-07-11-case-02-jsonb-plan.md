# Implementation Plan — Case 02: JSONB Atribut Dinamis

Source spec: `docs/superpowers/specs/2026-07-11-pgsql-playground-design.md` and
`docs/plans/02-jsonb.md`. This plan decomposes Case 02 into three ordered,
mostly-independent tasks.

## Global Constraints

These apply to every task in this plan — copy verbatim into every reviewer dispatch.

- **Stack:** Go stdlib only, except `github.com/jackc/pgx/v5` (`pgxpool`, `pgx.CopyFrom`, `pgx.Identifier`) for the database. No ORM, no router library, no faker library.
- **Module:** `pgsql-playground`. Go 1.25 (generics, `min()` builtin, `http.ServeMux` method+pattern routing like `"GET /api/search"` are all available and already used in the codebase).
- **Migrations:** one `.sql` file per case under `migrations/`, embedded via `migrations/embed.go` (`//go:embed *.sql`), applied in filename order by `internal/db/migrate.go` (tracked in `schema_migrations`). This case's file must be named `0003_jsonb.sql` — it runs after `0000_extensions.sql`, `0001_core.sql`, `0002_fts.sql`.
- **`products` table** (from `migrations/0001_core.sql`) already has an `attributes jsonb NOT NULL DEFAULT '{}'::jsonb` column — do not redefine it, only `ALTER TABLE` to add indexes/generated columns.
- **Generator package** `internal/gen` (`internal/gen/gen.go`, `internal/gen/words.go`): table generators are functions `genX(ctx, pool, rnd, rows) error`, dispatched from `Generate()`'s switch statement by table name constant (e.g. `TableProducts = "products"`). Bulk insert goes through the existing `copyBatches(ctx, pool, table, columns, rows, build)` helper — reuse it, do not write a parallel insert path. Word lists live in `internal/gen/words.go` as package-level `var` slices.
- **`genProducts`** in `internal/gen/gen.go` currently builds only `seller_id, category_id, name, description, price`. This task must extend it to also populate `attributes` (a Go `map[string]any`, marshaled to JSON before insert — pgx accepts a JSON-encodable value for a `jsonb` column when passed as `[]byte`; use `encoding/json.Marshal` on the map and pass the resulting `[]byte`). Keep the reproducibility property: same `-seed` ⇒ identical output.
- **HTTP layer:** `net/http` with Go 1.22+ `http.ServeMux` patterns (`mux.HandleFunc("GET /api/products", handler)`). Response helpers already exist in `internal/web/web.go`: `web.JSON(w, status, data)` and `web.Error(w, status, msg)` — use them, do not write new response-encoding helpers.
- **Case package layout:** each case lives in `internal/cases/<name>/<name>.go` with a single exported `RegisterRoutes(mux *http.ServeMux, pool *pgxpool.Pool)` function, called from `cmd/api/main.go`. Follow the exact shape of `internal/cases/fts/fts.go` (handler-factory functions returning `http.HandlerFunc`, a small result struct with `json` tags, query-param parsing at the top of each handler, `web.Error` for a `400` on missing/invalid params and `500` on query errors).
- **Frontend:** `internal/web/templates/<name>.html` defining `{{define "content"}}...{{end}}`, rendered via `web.RenderPage(w, "<name>", data)` inside the shared `layout.html`. Vanilla JS only, using the existing `fetchJSON(url)` helper in `internal/web/static/app.js` — do not add a JS framework or build step. Register the case in the `web.Cases` slice in `internal/web/web.go` (same file where `{Slug: "search", ...}` currently lives) so it shows on the `/` index page.
- **No new third-party dependencies.** Everything needed (JSON, HTTP, templates) is stdlib or already-vendored pgx.
- **Testing:** this codebase's existing test is `internal/db/migrate_test.go`, an integration-style test that skips if `DATABASE_URL` has no reachable Postgres (`t.Skipf` on connect error). Follow that pattern for any new test: no mocking framework, no fixtures directory, tests run against the real dev Postgres started via `docker-compose.yml` (already running on `localhost:5434`, `DATABASE_URL=postgres://app:app@localhost:5434/playground`).
- **Ponytail / YAGNI:** do not add pagination, auth, caching, or generic query-builder abstractions beyond what each task explicitly asks for. Filtering by one or two known attribute keys via simple `WHERE attributes @> $1::jsonb` is sufficient — no dynamic arbitrary-field query DSL.

## Task 1 — Migration + generator: populate product attributes

**Files:** `migrations/0003_jsonb.sql` (new), `internal/gen/gen.go` (edit `genProducts`), `internal/gen/words.go` (edit, add attribute value lists).

**What to build:**

1. `migrations/0003_jsonb.sql`:
   ```sql
   CREATE INDEX IF NOT EXISTS idx_products_attrs ON products USING gin(attributes jsonb_path_ops);
   ALTER TABLE products
     ADD COLUMN IF NOT EXISTS brand text
     GENERATED ALWAYS AS (attributes->>'brand') STORED;
   CREATE INDEX IF NOT EXISTS idx_products_brand ON products(brand);
   ```
2. In `internal/gen/words.go`, add two new word lists: `brands` (8-10 fake brand names, e.g. `"Northline", "Vertex", "Aurora", ...`) and `colors` (8-10 colors) and `sizes` (`"S","M","L","XL"`).
3. In `internal/gen/gen.go`, extend `genProducts` so each generated product's `attributes` map is built as follows (category-dependent, using the product's already-computed `categoryID` and the existing `rnd`):
   - Every product gets `"brand"` (random pick from the new `brands` list).
   - If `categoryID % 2 == 0` (even categories — treat as "apparel-like"): also add `"color"` and `"size"` keys (random picks).
   - Otherwise (odd categories — treat as "electronics-like"): also add `"ram_gb"` (int, one of `4, 8, 16, 32`) and `"storage_gb"` (int, one of `128, 256, 512, 1024`).
   - Marshal the map with `encoding/json.Marshal` and pass the `[]byte` as the `attributes` column value; add `"attributes"` to the `columns` slice passed to `copyBatches`/`CopyFrom` and to the row values returned by `build`.
4. Update the `TableProducts` case: the INSERT column list gains `attributes`.

**Acceptance criteria:**
- `go build ./...` succeeds.
- Running `go run ./cmd/gen -table=products -rows=500 -truncate` (after categories/sellers already generated) populates `attributes` with non-empty JSON on every row — spot check with `SELECT attributes FROM products LIMIT 5;` via `psql` or a quick query.
- `SELECT brand FROM products LIMIT 5;` returns non-null values matching the JSON `brand` key (proves the generated column works).
- Re-running the same command with the same `-seed` produces byte-identical `attributes` JSON for the same row index (reproducibility preserved).
- `go vet ./...` clean.

## Task 2 — Query endpoints: filter + facets

**Depends on:** Task 1 (needs `attributes`/`brand` populated and indexed to return meaningful results, and the GIN index to demonstrate in the EXPLAIN exercise below).

**Files:** `internal/cases/jsonb/jsonb.go` (new package), `cmd/api/main.go` (edit — register the new case).

**What to build:**

1. New package `internal/cases/jsonb` mirroring `internal/cases/fts`'s structure:
   - `RegisterRoutes(mux *http.ServeMux, pool *pgxpool.Pool)` registering:
     - `GET /api/products` — query params: any number of `attr.<key>=<value>` pairs (e.g. `?attr.brand=Vertex&attr.color=Red`), plus optional `min_price` and `max_price` (numeric). Build a `map[string]any` from all `attr.*` params found in `r.URL.Query()`, marshal it to JSON, and filter with `WHERE attributes @> $1::jsonb`. Add `AND price >= $n` / `AND price <= $n` clauses only when those params are present. Return matching products (`id, name, price, attributes`) as JSON, limited to 50 rows, ordered by `id`.
     - `GET /api/products/facets?category_id=<id>` — for products in the given category, return the distribution of each attribute value actually present. Implementation approach: `SELECT key, value, count(*) FROM products, jsonb_each_text(attributes) WHERE category_id = $1 GROUP BY key, value ORDER BY key, count(*) DESC`. Shape the JSON response as an object keyed by attribute name, each value a list of `{value, count}`, e.g. `{"brand": [{"value":"Vertex","count":12}], "color": [...]}`.
   - `GET /ui/products` route rendering the `products` template (added in Task 3 — if Task 3 hasn't run yet, `web.RenderPage` will error on a missing template; that's fine, Task 2 only needs the handler wired, the page itself is Task 3's job to make functional. If you want Task 2 fully demonstrable standalone, you may render a placeholder inline string via `w.Write` instead of `web.RenderPage` for `/ui/products` in this task, and let Task 3 replace it with the real `web.RenderPage` call — pick whichever is less code).
2. Wire the new case into `cmd/api/main.go`: import `pgsql-playground/internal/cases/jsonb` and call `jsonb.RegisterRoutes(mux, pool)` alongside the existing `fts.RegisterRoutes(mux, pool)`.

**Acceptance criteria:**
- `go build ./...` succeeds.
- With the API running against seeded data: `curl "localhost:8080/api/products?attr.brand=<some-real-brand>"` returns only products whose `attributes.brand` matches.
- Combining `attr.*` params with `min_price`/`max_price` further narrows results correctly.
- `curl "localhost:8080/api/products/facets?category_id=1"` returns a JSON object with per-attribute value counts that sum to the number of products in that category (for keys present on all products in that category).
- Malformed/missing required input still returns valid JSON via `web.Error`, never a raw 500 stack trace or empty body.
- `go vet ./...` clean.

## Task 3 — Frontend page `/ui/products`

**Depends on:** Task 2 (needs the `/api/products` and `/api/products/facets` endpoints live).

**Files:** `internal/web/templates/products.html` (new), `internal/web/web.go` (edit — add to `Cases` slice), `internal/cases/jsonb/jsonb.go` (edit — replace the Task 2 placeholder `/ui/products` handler with a real `web.RenderPage(w, "products", nil)` call, if Task 2 used a placeholder).

**What to build:**

1. `internal/web/templates/products.html` following the shape of `internal/web/templates/search.html`: a `{{define "content"}}` block with:
   - A filter form: text inputs for `brand` and `color` (the two most common attribute keys from Task 1's data), plus `min_price`/`max_price` number inputs, and a `category_id` number input used for the facet sidebar.
   - A `<div id="results">` for the product list and a `<div id="facets">` sidebar.
   - Inline `<script>` (same pattern as `search.html`) that on submit calls `fetchJSON` against `/api/products?attr.brand=...&attr.color=...&min_price=...&max_price=...` (only include params the user filled in) and renders each product as a `.result` div (reuse the CSS class already defined in `internal/web/static/style.css`) showing name, price, and the raw `attributes` JSON.
   - When `category_id` is filled in, also call `/api/products/facets?category_id=...` and render the counts in the `#facets` div (e.g. a `<ul>` per attribute key).
2. Add `{Slug: "products", Title: "JSONB Atribut Dinamis", Desc: "filter atribut dinamis via containment (@>), facet count, generated column"}` to the `Cases` slice in `internal/web/web.go`.

**Acceptance criteria:**
- `go build ./...` succeeds.
- Opening `http://localhost:8080/` lists "JSONB Atribut Dinamis" linking to `/ui/products`.
- Opening `/ui/products`, filling brand/color and submitting, shows matching products without a page reload.
- Filling `category_id` and submitting shows a facet breakdown.
- No JS console errors on a fresh page load or after a submit with empty/partial filters.
