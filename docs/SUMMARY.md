# PostgreSQL Playground — Ringkasan Belajar

Proyek latihan fitur-fitur PostgreSQL yang jarang dipakai orang tapi powerful,
di-drive dari Go (pgx) dengan frontend HTML sederhana per case. Tiap case berdiri
sendiri: satu migration, satu package `internal/cases/<x>`, satu halaman `/ui/<x>`.

## Cara jalan

```bash
docker compose up -d                                   # Postgres 17 + pgvector di :5434
export DATABASE_URL="postgres://app:app@localhost:5434/playground"

# seed data (urut penting: products butuh sellers+categories, dst)
go run ./cmd/gen -table users       -rows 200  -truncate
go run ./cmd/gen -table categories  -rows 20   -truncate   # >12 → muncul hierarki
go run ./cmd/gen -table sellers     -rows 50   -truncate
go run ./cmd/gen -table products    -rows 300  -truncate
go run ./cmd/gen -table orders      -rows 500  -truncate
go run ./cmd/gen -table order_items -rows 1500 -truncate
go run ./cmd/gen -table events      -rows 2000 -truncate
go run ./cmd/gen -table embeddings                         # backfill kolom, tanpa -rows

go run ./cmd/api                                       # http://localhost:8080
```

Migration jalan otomatis saat `cmd/api` start. Extension diaktifkan di `0000_extensions.sql`:
`pg_trgm`, `cube`, `earthdistance`, `vector`.

---

## Case 00 — Foundation

**Inti:** kerangka jalan — Docker Postgres, pool koneksi, migration runner, generator data.

- **`pgxpool`** dipakai, bukan `database/sql`. Pool async-native milik pgx, lebih cepat & mendukung fitur PG spesifik (LISTEN/NOTIFY, COPY, tipe custom) tanpa driver adapter.
- **`pgx.CopyFrom`** untuk bulk insert — protokol `COPY` jauh lebih cepat dari ribuan `INSERT` karena satu round-trip + tanpa parse per baris. Lihat `internal/gen/gen.go:copyBatches`.
- Migration runner sederhana baca `migrations/*.sql` berurutan (`internal/db/migrate.go`).

**Skema dasar:** `users, categories, sellers, products, reviews`. `products.embedding vector(128)` disiapkan di sini (diisi case 07).

---

## Case 01 — Full-Text Search + Trigram Fuzzy

**Inti:** cari teks dengan ranking, tahan typo, dan autocomplete — semua di dalam PG.

**Konsep:**
- `tsvector` (dokumen ter-tokenize) vs `tsquery` (query). Kolom `search` dibuat **`GENERATED ALWAYS AS ... STORED`** dari `to_tsvector` — otomatis ter-update tiap row berubah, tak perlu trigger.
- **GIN index** pada tsvector → pencarian cepat. `ts_rank` untuk skor relevansi, `ts_headline` untuk highlight.
- **`pg_trgm`**: `similarity()` & operator `%` untuk fuzzy match (typo-tolerant), didukung GIN `gin_trgm_ops`. Juga mempercepat `ILIKE prefix%` untuk autocomplete.

**Endpoint** (`internal/cases/fts`):
| Endpoint | Teknik |
|---|---|
| `GET /api/search?q=` | `websearch_to_tsquery` + `ts_rank` + `ts_headline` |
| `GET /api/search/fuzzy?q=` | `WHERE name % $1 ORDER BY similarity(...)` |
| `GET /api/autocomplete?prefix=` | `ILIKE prefix||'%'` didukung trigram |

**Migration:** `0002_fts.sql`. **UI:** `/ui/search`.

---

## Case 02 — JSONB Atribut Dinamis

**Inti:** produk punya atribut beda-beda per kategori (elektronik: `ram/storage`; fashion: `color/size`) di kolom `attributes jsonb`.

**Konsep:**
- Operator: `->` (ambil jsonb), `->>` (ambil text), `@>` (containment), `?`/`?|`/`?&` (key exists).
- **`@>` containment** = cara idiomatik filter "punya atribut X=Y". Query dibangun dari query-param jadi satu objek JSON lalu `WHERE attributes @> $1`.
- **GIN `jsonb_path_ops`** — index lebih kecil & cepat khusus untuk `@>` (trade-off: tak mendukung operator key-exists).
- **Generated column** `brand` (`attributes->>'brand'` STORED) + btree index — untuk atribut yang sering difilter, lebih cepat dari ekspresi index.
- Facet count via `jsonb_each` + `GROUP BY`.

**Endpoint** (`internal/cases/jsonb`):
| Endpoint | Teknik |
|---|---|
| `GET /api/products?attr.color=red&...` | bangun `@>` dari query-param |
| `GET /api/products/facets?category_id=` | distribusi nilai atribut via `jsonb_each` |

**Migration:** `0003_jsonb.sql`. **UI:** `/ui/products`.

---

## Case 03 — Partisi & Performa

**Inti:** tabel besar (`orders/order_items/events`), partisi per waktu, jenis index, dan playground EXPLAIN dari browser.

**Konsep:**
- **Declarative partitioning** `PARTITION BY RANGE (created_at)`, satu partisi per bulan. **Partition pruning:** query dengan filter tanggal hanya scan partisi relevan.
- **BRIN index** (`USING brin(created_at)`) — sangat kecil, cocok untuk kolom yang nilainya monoton naik (waktu) di tabel besar. Bandingkan dengan BTREE.
- **`EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`** untuk baca plan: node, cost, actual time, buffer hits.

**Endpoint** (`internal/cases/perf`):
| Endpoint | Teknik |
|---|---|
| `POST /api/explain` `{sql}` | allowlist SELECT → jalankan EXPLAIN, balik plan JSON |
| `GET /api/orders?from=&to=&user_id=` | tunjukkan pruning di plan |

**Migration:** `0004_orders.sql`. **UI:** `/ui/explain` (textarea SQL → render plan).

---

## Case 04 — Analitik & Window Functions

**Inti:** laporan analitik nyata — revenue time-series, ranking, funnel, RFM, kohort.

**Konsep:**
- **Window functions:** `RANK() OVER (PARTITION BY ...)`, running total `SUM(...) OVER (ORDER BY ...)`, `NTILE(5)` untuk segmentasi kuintil.
- **Agregasi lanjutan:** `count(*) FILTER (WHERE ...)` (agregat kondisional dalam satu scan), `GROUPING SETS` (subtotal + grand total sekaligus).
- **Recursive CTE** (`WITH RECURSIVE`) untuk telusuri hierarki kategori via `parent_id`.
- **Materialized view** `mv_daily_revenue` — hasil agregasi disimpan, dibaca cepat. Perlu **`REFRESH MATERIALIZED VIEW CONCURRENTLY`** (butuh unique index) supaya data baru muncul tanpa lock read.

**Endpoint** (`internal/cases/analytics`):
| Endpoint | Teknik |
|---|---|
| `GET /api/analytics/revenue?from=&to=&bucket=` | baca MV + running total `SUM OVER` |
| `GET /api/analytics/top-products?by=revenue\|qty` | `RANK() OVER (PARTITION BY category)` |
| `GET /api/analytics/funnel` | `count FILTER` view→cart→purchase |
| `GET /api/analytics/rfm` | `NTILE(5)` atas recency/frequency/monetary |
| `GET /api/analytics/summary` | `GROUPING SETS` (per-status + total) |
| `GET /api/analytics/categories` | `WITH RECURSIVE` hierarki (depth + path) |
| `POST /api/analytics/refresh` | `REFRESH MATERIALIZED VIEW CONCURRENTLY` |

**Migration:** `0005_analytics.sql`. **UI:** `/ui/analytics` (tab per laporan + tombol Refresh MV).

> Catatan: hierarki kategori baru muncul kalau `-rows` > 12 (jumlah base name); di bawah itu semua kategori flat (root).

---

## Case 05 — Geospatial (earthdistance)

**Inti:** cari seller terdekat dari koordinat, **tanpa PostGIS** — cukup `cube` + `earthdistance`.

**Konsep:**
- `ll_to_earth(lat, lng)` → titik 3D di bumi; `earth_distance(a, b)` → jarak meter.
- **GiST index** pada ekspresi `ll_to_earth(lat, lng)` → nearest-neighbor cepat.
- **`earth_box`** (bounding box) `@>` dipakai sebagai **filter kasar** yang bisa pakai index, baru `earth_distance` untuk filter presisi. Tanpa `earth_box`, `earth_distance` murni = seq scan.

**Endpoint** (`internal/cases/geo`):
| Endpoint | Teknik |
|---|---|
| `GET /api/sellers/nearby?lat=&lng=&radius_km=&limit=` | `earth_box @>` (kasar, index) + `earth_distance` (presisi), urut jarak |

Generator sebar `sellers.lat/lng` di beberapa cluster kota → hasil "terdekat" bermakna.

**Migration:** `0006_geo.sql`. **UI:** `/ui/nearby` (bisa pakai `navigator.geolocation`).

---

## Case 06 — Realtime LISTEN/NOTIFY → SSE

**Inti:** push order baru ke browser realtime pakai fitur PG murni, tanpa message broker.

**Konsep:**
- **`LISTEN` / `NOTIFY channel, payload`** — pub/sub bawaan Postgres. Trigger `AFTER INSERT` di `orders` panggil `pg_notify('orders', json...)`.
- Batas payload NOTIFY ~8000 byte → kirim id/ringkasan, bukan seluruh row.
- Di Go: satu koneksi dedicated `conn.WaitForNotification(ctx)`, lalu **fan-out** ke banyak klien SSE via channel. **Satu listener untuk semua klien**, bukan satu koneksi LISTEN per klien (lihat `hub` di `realtime.go`).
- SSE (`text/event-stream`) + `flusher.Flush()`; hormati `ctx.Done()` saat klien putus.

**Endpoint** (`internal/cases/realtime`):
| Endpoint | Teknik |
|---|---|
| `GET /api/events/stream` | SSE, `LISTEN orders` → fan-out |
| `POST /api/orders/simulate` | INSERT 1 order → trigger memicu notify |

**Migration:** `0007_notify.sql`. **UI:** `/ui/realtime` (`EventSource` + tombol simulate).

---

## Case 07 — Vector Similarity (pgvector)

**Inti:** "produk mirip" secara semantik pakai embedding vektor.

**Konsep:**
- Tipe **`vector(128)`**; operator jarak `<->` (L2), `<#>` (inner product), `<=>` (cosine). Di sini pakai `<=>`.
- **HNSW index** (`vector_cosine_ops`) untuk approximate nearest-neighbor cepat. Alternatif `ivfflat` (trade-off build time / recall / speed).
- **Tanpa model embedding nyata:** generator kasih tiap kategori satu centroid acak dim-128 (seed tetap), embedding produk = centroid kategori + noise gaussian kecil, lalu dinormalisasi → produk sekategori berdekatan, "mirip" jadi bermakna.

**Endpoint** (`internal/cases/vector`):
| Endpoint | Teknik |
|---|---|
| `GET /api/search/semantic?product_id=&limit=` | `embedding <=> (embedding produk sumber)` |
| `GET /api/search/semantic?q=&limit=` | keyword → kategori cocok (ILIKE) → centroid `avg(embedding)` → nearest |
| `GET /api/products/list` | daftar produk untuk dropdown UI |

**Migration:** `0008_vector.sql`. **UI:** `/ui/semantic` (pilih produk / ketik keyword → daftar mirip + skor jarak).

---

## Peta file

```
migrations/          0000..0008 SQL (dijalankan urut saat api start)
cmd/api/             HTTP server + wiring semua case
cmd/gen/             CLI generator data (-table -rows -seed -truncate)
internal/gen/        logika generate per tabel + centroid embedding
internal/db/         pool + migration runner
internal/web/        shell template, helper JSON/Error, daftar Case
internal/cases/<x>/  handler + query tiap case
```
