# pgsql-playground — Design Spec

**Tanggal:** 2026-07-11
**Tujuan:** Belajar PostgreSQL di luar CRUD dasar (search, analitik, JSONB, partisi, geo, realtime, vector) lewat satu domain e-commerce yang koheren. Go pakai stdlib; hanya koneksi pakai `pgxpool`. Frontend Go web (`html/template`) + vanilla JS `fetch`.

## Prinsip

- **Satu domain, banyak case.** Semua fitur berbagi skema e-commerce yang sama.
- **Fokus DB.** Kode Go seminimal mungkin; boilerplate ditekan agar perhatian di query & fitur PG.
- **stdlib only** kecuali `github.com/jackc/pgx/v5/pgxpool` untuk koneksi/pool dan `CopyFrom`.
- **Reproducible.** Generator pakai `math/rand` seed tetap → data sama tiap generate.
- **Tiap case punya latihan `EXPLAIN ANALYZE`** — banding dengan/tanpa index.

## Keputusan penyederhanaan (ponytail)

- **Geo pakai `earthdistance`+`cube` (contrib), bukan PostGIS** → satu image Docker `pgvector/pgvector:pg17` sudah bawa `pgvector` + semua contrib (`pg_trgm`, `cube`, `earthdistance`). Tambah PostGIS hanya bila butuh geometry sebenarnya.
- **Migrasi = file `.sql` bernomor**, dijalankan runner kecil (`embed` + `pgxpool.Exec`), tabel `schema_migrations` untuk track. Tanpa library migrasi.
- **Generator tanpa faker lib** — word-list kecil + `math/rand`. Bulk load `pgx.CopyFrom`.
- **Embedding pgvector** diturunkan dari centroid kategori + noise (dim 128) → semantic search mengelompok.
- **Frontend** satu server Go: `html/template` untuk shell tiap halaman, vanilla JS `fetch` untuk panggil JSON API. Tanpa build step, tanpa framework JS.
- **Skala dikonfigurasi.** Default kecil supaya cepat; naikkan lewat flag `-rows` untuk merasakan efek index/partisi (target `orders` ~10 jt).

## Skema (ringkas)

| Tabel | Baris (skala penuh) | Kolom kunci | Case |
|---|---|---|---|
| `users` | ~1 jt | id, email, created_at | join, analitik |
| `sellers` | ~50 rb | id, name, lat, lng, `earth` (generated) | geo |
| `categories` | ~500 | id, name, parent_id | JSONB, analitik |
| `products` | ~500 rb | id, seller_id, category_id, name, description, price, `attributes jsonb`, `search tsvector`, `embedding vector(128)` | FTS, JSONB, vector |
| `orders` | ~10 jt | id, user_id, status, total, created_at | **partisi per bulan**, analitik |
| `order_items` | ~30 jt | order_id, product_id, qty, price | agregasi |
| `reviews` | ~5 jt | id, product_id, user_id, body, rating, `search tsvector` | FTS + trigram fuzzy |
| `events` | ~50 jt | id, user_id, product_id, kind, created_at | funnel, BRIN |

## Struktur repo

```
cmd/
  api/main.go          # server net/http (ServeMux Go 1.22), serve API + frontend
  gen/main.go          # generator CLI: gen -table=orders -rows=10000000 -seed=1
internal/
  db/pool.go           # pgxpool setup dari DATABASE_URL
  db/migrate.go        # runner .sql (embed)
  gen/                 # generator per tabel (CopyFrom)
  cases/
    fts/               # /api/search, /api/search/fuzzy, /api/autocomplete
    jsonb/             # /api/products (filter attr)
    perf/              # /api/explain, demo partisi & index
    analytics/         # /api/analytics/*
    geo/               # /api/sellers/nearby
    realtime/          # /api/events/stream (SSE) via LISTEN/NOTIFY
    vector/            # /api/search/semantic
  web/                 # html/template + static (vanilla js, css)
    templates/*.html
    static/app.js, style.css
migrations/*.sql
docker-compose.yml     # pgvector/pgvector:pg17
README.md              # index case + catatan belajar
```

## Frontend

- Server Go yang sama melayani `/` (index case) dan satu halaman per case (mis. `/ui/search`).
- `html/template` render shell (form input + area hasil). `internal/web/static/app.js` melakukan `fetch` ke endpoint `/api/...` dan render JSON ke DOM. CSS vanilla minimal.
- Setiap halaman case menampilkan: form, hasil JSON terformat, dan blok "EXPLAIN" (endpoint `perf` bisa dipanggil untuk lihat plan).

## Error handling & testing

- Config via env `DATABASE_URL`; gagal konek → log jelas + exit non-zero.
- Generator idempoten: opsi `-truncate`, batch COPY dengan progress log.
- Handler API kembalikan JSON `{error: "..."}` + status code sesuai; input divalidasi di boundary.
- Test: per case satu test integrasi ringan (`testing` stdlib) terhadap DB Docker — cek bentuk hasil query benar. Tanpa framework/fixtures.

## Fase / urutan pengerjaan

Tiap fase = satu file plan di `docs/plans/`, dikerjakan berurutan:

0. `00-foundation` — docker, pgxpool, migration runner, skema dasar, generator inti, web shell.
1. `01-fts` — full-text search + trigram fuzzy + autocomplete.
2. `02-jsonb` — atribut dinamis, operator, GIN, generated column.
3. `03-partition-perf` — orders partisi, index (BTREE/GIN/BRIN), COPY 10jt, EXPLAIN playground.
4. `04-analytics` — window functions, GROUPING SETS, CTE, materialized view, funnel, RFM.
5. `05-geo` — earthdistance nearest.
6. `06-realtime` — LISTEN/NOTIFY → SSE.
7. `07-vector` — pgvector semantic search.

**Dependensi:** 04 butuh `orders`/`events` dari 03. Lainnya independen setelah fondasi.

## Yang di-skip sengaja

PostGIS, library migrasi/faker/router, auth, ORM. Tambah bila kebutuhan nyata muncul.
