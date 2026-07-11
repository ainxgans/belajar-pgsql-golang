# Case 03 — Partisi & Performa

**Depends:** 00. **Tujuan:** `orders`/`order_items`/`events` skala besar (target 10jt+), partisi per waktu, jenis index, dan playground EXPLAIN.

## Yang dipelajari
- Declarative partitioning `PARTITION BY RANGE (created_at)`, partition pruning.
- Index: BTREE, GIN, **BRIN** (cocok untuk kolom waktu monoton di tabel besar).
- Bulk load `COPY` (via `pgx.CopyFrom`) — kenapa jauh lebih cepat dari INSERT.
- `EXPLAIN (ANALYZE, BUFFERS)`, `VACUUM ANALYZE`, statistik & `pg_stat_statements` (opsional).

## Migration `0004_orders.sql`
```sql
CREATE TABLE orders (
  id bigserial, user_id bigint, status text, total numeric(12,2),
  created_at timestamptz NOT NULL
) PARTITION BY RANGE (created_at);
-- buat partisi per bulan (mis. 12-24 bulan) + default
CREATE TABLE orders_2025_01 PARTITION OF orders
  FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');
-- ... dst (bisa di-generate loop DO $$)
CREATE TABLE order_items (order_id bigint, product_id bigint, qty int, price numeric(12,2));
CREATE TABLE events (id bigserial, user_id bigint, product_id bigint, kind text, created_at timestamptz);
CREATE INDEX idx_orders_created ON orders USING brin(created_at);
CREATE INDEX idx_orders_user ON orders(user_id);
CREATE INDEX idx_events_created_brin ON events USING brin(created_at);
```

## Generator
- `gen -table=orders -rows=10000000` → CopyFrom ke tabel partisi induk, `created_at` tersebar 12-24 bulan.
- `order_items` ~3x orders; `events` ~5x. Progress log + timing.
- Tips: jalankan generate besar setelah index BRIN dibuat vs sebelum → banding waktu (catat).

## Endpoints (`internal/cases/perf`)
- `POST /api/explain` body `{sql}` (allowlist SELECT) → jalankan `EXPLAIN (ANALYZE, FORMAT JSON)`, kembalikan plan.
- `GET /api/orders?from=&to=&user_id=` → tunjukkan partition pruning di plan.

## Frontend `/ui/explain`
Textarea SQL → tampil plan (indent node, cost, actual time, buffers).

## Latihan EXPLAIN
- Query range tanggal 1 bulan: lihat pruning (hanya 1 partisi discan).
- BRIN vs BTREE pada `created_at` (ukuran index & waktu).
- Filter tanpa index `user_id` vs dengan.

## Selesai bila
`orders` terisi ≥1jt terpartisi; `/api/explain` balik plan JSON; test cek query 1-bulan hanya menyentuh 1 partisi (cek node plan).
