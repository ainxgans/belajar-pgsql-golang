# Case 00 — Foundation

**Tujuan:** Kerangka jalan: Postgres di Docker, koneksi pgxpool, migration runner, skema dasar, generator data inti, dan shell frontend. Setelah ini `curl` & buka browser bisa jalan.

## Yang dipelajari
- Setup Postgres via Docker + extension (`pg_trgm`, `cube`, `earthdistance`, `vector`).
- Connection pool dengan pgxpool (bukan `database/sql`).
- Bulk insert cepat dengan `pgx.CopyFrom` vs INSERT biasa.

## Docker
`docker-compose.yml` service `db` image `pgvector/pgvector:pg17`, port 5432, volume data, env `POSTGRES_*`. `DATABASE_URL=postgres://app:app@localhost:5432/playground`.

## Migrations
- `internal/db/migrate.go`: baca `migrations/*.sql` (embed), urut nama, jalankan yang belum ada di `schema_migrations(version text pk)`, dalam transaksi.
- `migrations/0000_extensions.sql`:
  ```sql
  CREATE EXTENSION IF NOT EXISTS pg_trgm;
  CREATE EXTENSION IF NOT EXISTS cube;
  CREATE EXTENSION IF NOT EXISTS earthdistance;
  CREATE EXTENSION IF NOT EXISTS vector;
  ```
- `migrations/0001_core.sql`: `users`, `sellers`, `categories`, `products` (kolom `attributes jsonb`, `search tsvector`, `embedding vector(128)` dibiarkan nullable dulu — diisi case terkait), FK dasar.

## Generator (`cmd/gen`, `internal/gen`)
- Flag: `-table`, `-rows`, `-seed` (default 1), `-truncate`.
- `math/rand.New(rand.NewSource(seed))`, word-list kecil untuk nama/email/deskripsi.
- Pakai `pgx.CopyFrom` per tabel, batch (mis. 50k), progress log tiap batch.
- Tabel di case ini: `users`, `categories`, `sellers`, `products` (attributes/embedding kosong dulu).

## API + Frontend shell
- `cmd/api/main.go`: `http.ServeMux`, `GET /healthz` (ping pool), serve `/static/`, `GET /` index case.
- `internal/web`: `templates/layout.html` + `index.html`, `static/style.css`, `static/app.js` (helper `fetchJSON`, render).
- Index list semua case + link.

## Selesai bila
- `docker compose up -d` → `go run ./cmd/gen -table=users -rows=10000` isi data.
- `curl localhost:8080/healthz` → ok; buka `/` tampil daftar case.
- Test: `internal/db` migrate idempoten (jalan 2x aman).
