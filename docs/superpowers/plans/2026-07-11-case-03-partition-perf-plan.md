# Implementation Plan — Case 03: Partisi & Performa

Source spec: `docs/superpowers/specs/2026-07-11-pgsql-playground-design.md` and
`docs/plans/03-partition-perf.md`. Three ordered tasks.

## Global Constraints

- **Stack:** Go stdlib only, except `github.com/jackc/pgx/v5` (`pgxpool`, `pgx.CopyFrom`, `pgx.Identifier`). No ORM, no router/faker library, no new dependency.
- **Migrations:** this case's file is `migrations/0004_orders.sql`, applied after `0000`–`0003` by the existing `internal/db/migrate.go` embed-and-run mechanism (nothing to change there).
- **`orders` is RANGE-partitioned by `created_at`** (declarative partitioning), one partition per calendar month plus a `DEFAULT` partition catching anything outside the generated range. `order_items` and `events` are plain (non-partitioned) tables — only `orders` gets partitioned, matching the design spec's table list.
- **Deliberate FK simplification:** do NOT add a foreign key from `order_items.order_id` to `orders.id`, and do NOT add one from `events` to `orders`. A partitioned table can only be an FK target if its referenced columns carry a partition-key-inclusive unique constraint (e.g. `PRIMARY KEY (id, created_at)`), which is unnecessary machinery for this case's actual learning goal (partition pruning, BRIN vs BTREE, COPY throughput). Mark the omission with a one-line `-- ponytail:` comment in the migration. `order_items.product_id`, `events.user_id`, and `events.product_id` still get normal FKs to the existing (non-partitioned) `products`/`users` tables.
- **Generator package** `internal/gen`: follow the exact existing shape in `internal/gen/gen.go` — a `TableX` constant, a `genX(ctx, pool, rnd, rows) error` function dispatched from the `Generate()` switch, bulk insert via the existing `copyBatches(ctx, pool, table, columns, rows, build)` helper. Do not introduce a second insert path. `time.Now()` is fine to use directly in this generator code (this is a plain Go program run via `go run`/`go build`, not a sandboxed workflow script — no restriction on wall-clock calls here).
- **Reproducibility:** keep the existing property that the same `-seed` produces the same data — every value derived from `rnd`, never from unseeded randomness. (`time.Now()` is used only to anchor *where* the reproducible spread of dates sits relative to "today", not to introduce nondeterminism into which values are picked.)
- **HTTP layer:** `net/http` with Go 1.22+ `http.ServeMux` patterns (`mux.HandleFunc("GET /api/x", handler)` / `mux.HandleFunc("POST /api/x", handler)`), matching `internal/cases/fts/fts.go` and `internal/cases/jsonb/jsonb.go`. Use the existing `web.JSON(w, status, data)` / `web.Error(w, status, msg)` helpers — do not add new response-encoding helpers.
- **Case package layout:** `internal/cases/perf/perf.go`, exported `RegisterRoutes(mux *http.ServeMux, pool *pgxpool.Pool)`, called from `cmd/api/main.go` alongside the existing `fts.RegisterRoutes` / `jsonb.RegisterRoutes` calls.
- **Frontend:** `internal/web/templates/explain.html` with a `{{define "content"}}` block, rendered via `web.RenderPage(w, "explain", nil)`, registered in the `web.Cases` slice in `internal/web/web.go` (same pattern as the `search` and `products` entries). Vanilla JS only, reuse `fetchJSON` from `internal/web/static/app.js`.
- **No new third-party dependencies.**
- **Testing:** follow `internal/db/migrate_test.go`'s pattern (skip via `t.Skipf` if no reachable Postgres) if a test is added — no mocking, no fixtures.
- **Ponytail / YAGNI:** no `pg_stat_statements` setup, no `VACUUM` automation, no generic SQL-builder abstraction. The `/api/explain` endpoint's only safety requirement is: reject anything that isn't a single `SELECT` statement (case-insensitive, after trimming whitespace) — this is a local learning tool with one operator, not a multi-tenant service, so a simple prefix/statement-count check is enough; do not build a full SQL parser or permissions system.

## Task 1 — Migration: partitioned orders + order_items + events, and their generators

**Files:** `migrations/0004_orders.sql` (new), `internal/gen/gen.go` (edit), `internal/gen/words.go` (edit, add `orderStatuses` and `eventKinds` word lists).

**What to build:**

1. `migrations/0004_orders.sql`:
   - `orders` table: `id bigserial, user_id bigint NOT NULL REFERENCES users(id), status text NOT NULL, total numeric(12,2) NOT NULL, created_at timestamptz NOT NULL`, `PARTITION BY RANGE (created_at)`.
   - A `orders_default PARTITION OF orders DEFAULT` partition.
   - A `DO $$ ... EXECUTE format(...) ... END $$` block that creates one monthly partition (named `orders_YYYY_MM`) for each month from 24 months before `now()` through 2 months after `now()` (27 partitions total) — use `format('CREATE TABLE IF NOT EXISTS %I PARTITION OF orders FOR VALUES FROM (%L) TO (%L)', ...)` with `%I` for the identifier so it's safely quoted.
   - `order_items` table: `order_id bigint NOT NULL, product_id bigint NOT NULL REFERENCES products(id), qty int NOT NULL, price numeric(12,2) NOT NULL` (no FK on `order_id` — see Global Constraints). Add a plain btree index `idx_order_items_order ON order_items(order_id)`.
   - `events` table: `id bigserial PRIMARY KEY, user_id bigint NOT NULL REFERENCES users(id), product_id bigint REFERENCES products(id), kind text NOT NULL, created_at timestamptz NOT NULL` (`product_id` nullable — some events, like a homepage view, have no product).
   - Indexes: `CREATE INDEX idx_orders_created ON orders USING brin(created_at);`, `CREATE INDEX idx_orders_user ON orders(user_id);`, `CREATE INDEX idx_events_created_brin ON events USING brin(created_at);`.
2. `internal/gen/words.go`: add `orderStatuses = []string{"pending", "paid", "shipped", "delivered", "cancelled"}` and `eventKinds = []string{"view", "cart", "purchase"}`.
3. `internal/gen/gen.go`:
   - Add `TableOrders = "orders"`, `TableOrderItems = "order_items"`, `TableEvents = "events"` constants and wire them into `Generate()`'s switch.
   - `genOrders(ctx, pool, rnd, rows)`: needs `userCount` via the existing `tableCount` helper. For each row: `userID := rnd.Int63n(userCount) + 1`; `status` random from `orderStatuses`; `total` a random price similar in style to `genProducts`'s price calc; `createdAt` = `time.Now().AddDate(0, -24, 0)` plus a random offset uniformly within `26 months` (so it lands inside the 27-partition range built in the migration) — compute as `time.Now().Add(-time.Duration(rnd.Int63n(int64(26*30*24))) * time.Hour)` or equivalent, passed as a `time.Time` value (pgx encodes `time.Time` directly for `timestamptz`, no string formatting needed). Insert columns: `user_id, status, total, created_at`.
   - `genOrderItems(ctx, pool, rnd, rows)`: needs `orderCount` and `productCount` via `tableCount`. Each row: `orderID := rnd.Int63n(orderCount) + 1`, `productID := rnd.Int63n(productCount) + 1`, `qty := rnd.Intn(5) + 1`, `price` similar random price. Insert columns: `order_id, product_id, qty, price`.
   - `genEvents(ctx, pool, rnd, rows)`: needs `userCount` and `productCount`. Each row: `userID`, `kind` random from `eventKinds` but weighted so `view` is most common — simplest weighting: pick an index via `rnd.Intn(10)`, map `0-5 → "view"`, `6-8 → "cart"`, `9 → "purchase"` (roughly 60/30/10), `productID` a `*int64` (nil ~10% of the time via `rnd.Intn(10) == 0`, matching "some events have no product"), `createdAt` same date-spread approach as orders (does not need to land inside orders' partitions since `events` isn't partitioned — just spread over the same ~26 month window for consistency). Insert columns: `user_id, product_id, kind, created_at`.
   - Because `orders`/`order_items`/`events` have no incoming-FK-driven ordering constraint from each other beyond needing `users`/`products` to already exist, no special truncate-order logic is needed beyond what `-truncate` already does per-table (note: truncating `orders` with `RESTART IDENTITY CASCADE` will NOT truncate `order_items`/`events` since there's no FK relationship enforcing cascade — if the operator wants a clean slate they truncate each table explicitly; this is expected given the deliberate no-FK simplification, do not add code to compensate).

**Acceptance criteria:**
- `go build ./...` and `go vet ./...` clean.
- Migration applies cleanly on top of `0000`-`0003` (verify via running the api server, which runs `db.Migrate` on startup) and is idempotent (running it again is a no-op — already guaranteed by the existing `schema_migrations` tracking, no new code needed for this).
- `go run ./cmd/gen -table=orders -rows=1000000 -truncate` completes and `SELECT count(*) FROM orders;` matches; `SELECT count(*) FROM pg_partition_tree('orders');` (or checking `\d+ orders`) shows rows landed across multiple monthly partitions, not all in `orders_default`.
- `go run ./cmd/gen -table=order_items -rows=2000000 -truncate` and `go run ./cmd/gen -table=events -rows=3000000 -truncate` complete without FK errors.
- `EXPLAIN SELECT * FROM orders WHERE created_at >= date_trunc('month', now()) AND created_at < date_trunc('month', now()) + interval '1 month';` shows only 1-2 partitions scanned (partition pruning), not all of them.

## Task 2 — Endpoints: `/api/explain` and `/api/orders`

**Depends on:** Task 1 (needs `orders` populated to produce a meaningful plan/result).

**Files:** `internal/cases/perf/perf.go` (new package), `cmd/api/main.go` (edit — register the case).

**What to build:**

1. `internal/cases/perf/perf.go`, `RegisterRoutes(mux, pool)` registering:
   - `POST /api/explain` — request body `{"sql": "SELECT ..."}` (`encoding/json.Decode` into a small struct). Validate: trim whitespace, reject (400 via `web.Error`) if the trimmed, case-insensitive-uppercased string does not start with `"SELECT "` or isn't a single statement (reject if it contains a `;` anywhere except possibly a single trailing one — simplest check: reject if `strings.Count(trimmed, ";") > 1`, or if it contains `;` before the final character). On a valid query, run `pool.QueryRow(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) " + sql)` and scan the single JSON column into a `[]byte`/`json.RawMessage`, then return it as the response body via `web.JSON` (Postgres's `FORMAT JSON` output is already valid JSON text — decode it into `[]map[string]any` or pass through as `json.RawMessage` so the client gets the real plan structure, not a JSON-encoded string). If the query errors (bad SQL), return the Postgres error message via `web.Error` with 400.
   - `GET /api/orders` — query params `from`, `to` (RFC3339 or `YYYY-MM-DD`, parsed with `time.Parse`), `user_id` (int64, optional). Build `SELECT id, user_id, status, total, created_at FROM orders WHERE created_at >= $1 AND created_at < $2` and append `AND user_id = $3` only if `user_id` was given; default `from`/`to` to a sensible range (e.g. last 30 days) when omitted. Order by `created_at DESC`, limit 100. Return as JSON.
2. Wire into `cmd/api/main.go`: import `pgsql-playground/internal/cases/perf`, call `perf.RegisterRoutes(mux, pool)`.

**Acceptance criteria:**
- `go build ./...` clean.
- `curl -X POST localhost:8080/api/explain -d '{"sql":"SELECT * FROM orders WHERE created_at >= now() - interval '\''1 month'\''"}'` returns a JSON plan (an array/object with `Plan`/`Node Type` keys from Postgres's `EXPLAIN ... FORMAT JSON`).
- The same call with `{"sql":"DROP TABLE orders"}` or `{"sql":"SELECT 1; DROP TABLE orders"}` returns 400, not an executed statement.
- `curl "localhost:8080/api/orders?from=2026-01-01&to=2026-02-01"` returns only orders in that range; adding `&user_id=<id>` narrows further.
- `go vet ./...` clean.

## Task 3 — Frontend `/ui/explain`

**Depends on:** Task 2 (needs `/api/explain` live).

**Files:** `internal/web/templates/explain.html` (new), `internal/web/web.go` (edit — add to `Cases`), `internal/cases/perf/perf.go` (edit — add the `GET /ui/explain` route rendering the template).

**What to build:**

1. `internal/web/templates/explain.html`, `{{define "content"}}` block: a `<textarea>` pre-filled with a sample query (e.g. `SELECT * FROM orders WHERE created_at >= date_trunc('month', now()) AND created_at < date_trunc('month', now()) + interval '1 month'`), a submit button, and a `<pre id="plan">` output area. On submit, `POST` the textarea contents as JSON to `/api/explain` via `fetch` (not the existing `fetchJSON` helper, since that's GET-only — either extend `app.js` with a small `postJSON` helper following the same shape as `fetchJSON`, or inline a `fetch` call in the page's own `<script>`; prefer adding `postJSON` to `internal/web/static/app.js` since a POST helper is generically useful and mirrors the existing one) and `JSON.stringify` the parsed plan response into the `<pre>` with 2-space indentation for readability.
2. Add `{Slug: "explain", Title: "Partisi & EXPLAIN Playground", Desc: "orders dipartisi per bulan, BRIN vs BTREE, jalankan EXPLAIN ANALYZE dari browser"}` to `web.Cases`.
3. In `internal/cases/perf/perf.go`, register `mux.HandleFunc("GET /ui/explain", func(w, r) { web.RenderPage(w, "explain", nil) })`.

**Acceptance criteria:**
- `go build ./...` clean.
- `/` lists "Partisi & EXPLAIN Playground" linking to `/ui/explain`.
- Opening `/ui/explain`, the textarea has a working sample query pre-filled; submitting renders a readable JSON plan without a page reload.
- Submitting an invalid/blocked query shows the error message from the API, not a silent failure.
