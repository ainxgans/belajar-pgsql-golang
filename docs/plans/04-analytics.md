# Case 04 — Analitik & Window Functions

**Depends:** 03 (butuh `orders`, `order_items`, `events`). **Tujuan:** Laporan analitik nyata: revenue, ranking, funnel, RFM, kohort.

## Yang dipelajari
- Window: `ROW_NUMBER/RANK/DENSE_RANK`, `SUM() OVER (... running)`, `LAG/LEAD`, `NTILE`, `percentile_cont`.
- Agregasi lanjutan: `GROUPING SETS`, `ROLLUP`, `CUBE`, `FILTER (WHERE ...)`.
- CTE & recursive CTE (kategori hierarki via `parent_id`).
- `date_trunc` time-series; materialized view + `REFRESH`.

## Migration `0005_analytics.sql`
```sql
CREATE MATERIALIZED VIEW mv_daily_revenue AS
SELECT date_trunc('day', created_at) AS day,
       count(*) AS orders, sum(total) AS revenue
FROM orders GROUP BY 1;
CREATE UNIQUE INDEX ON mv_daily_revenue(day);
```

## Endpoints (`internal/cases/analytics`)
- `GET /api/analytics/revenue?from=&to=&bucket=day|week|month` → time-series + running total (`SUM OVER`).
- `GET /api/analytics/top-products?by=revenue|qty&per=category` → `RANK() OVER (PARTITION BY category ORDER BY ...)`.
- `GET /api/analytics/funnel` → `view→cart→purchase` dari `events` (`count FILTER`).
- `GET /api/analytics/rfm` → segmentasi user `NTILE(5)` atas recency/frequency/monetary.
- `GET /api/analytics/summary` → `GROUPING SETS` (total per status + grand total).

## Frontend `/ui/analytics`
Pilih rentang & bucket → tabel + (opsional) chart sederhana canvas/vanilla. Tab per laporan.

## Latihan EXPLAIN
- Query langsung vs materialized view (waktu).
- Window function vs subquery self-join untuk running total.
- `REFRESH MATERIALIZED VIEW CONCURRENTLY` (butuh unique index).

## Selesai bila
Semua endpoint balik angka konsisten; test cek running total monoton naik & funnel `view ≥ cart ≥ purchase`.
