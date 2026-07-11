# Case 01 — Full-Text Search + Trigram Fuzzy

**Depends:** 00. **Tujuan:** Pencarian teks produk & review dengan ranking, fuzzy (typo-tolerant), dan autocomplete.

## Yang dipelajari
- `tsvector` / `tsquery`, `to_tsvector`, `plainto_tsquery`, `websearch_to_tsquery`.
- Index GIN pada tsvector; `ts_rank` / `ts_rank_cd`; `ts_headline` untuk highlight.
- `pg_trgm` (`similarity`, `%`, GIN/GiST `gin_trgm_ops`) untuk fuzzy & autocomplete.

## Migration `0002_fts.sql`
```sql
ALTER TABLE products
  ADD COLUMN IF NOT EXISTS search tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', coalesce(name,'')||' '||coalesce(description,''))) STORED;
CREATE INDEX IF NOT EXISTS idx_products_search ON products USING gin(search);
CREATE INDEX IF NOT EXISTS idx_products_name_trgm ON products USING gin(name gin_trgm_ops);

ALTER TABLE reviews
  ADD COLUMN IF NOT EXISTS search tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', coalesce(body,''))) STORED;
CREATE INDEX IF NOT EXISTS idx_reviews_search ON reviews USING gin(search);
```
Generator: pastikan `products.name/description` & `reviews.body` terisi kata dari word-list (case ini juga generate `reviews` bila belum).

## Endpoints (`internal/cases/fts`)
- `GET /api/search?q=&limit=` → produk, `websearch_to_tsquery`, urut `ts_rank`, kembalikan `ts_headline` name.
- `GET /api/search/fuzzy?q=` → `WHERE name % $1 ORDER BY similarity(name,$1) DESC` (typo tolerant).
- `GET /api/autocomplete?prefix=` → `WHERE name ILIKE prefix||'%'` didukung trgm, limit 10.

## Frontend `/ui/search`
Input teks + toggle mode (exact/fuzzy), tampil hasil + skor + highlight.

## Latihan EXPLAIN
- `EXPLAIN ANALYZE` search dengan vs tanpa index GIN (drop/create).
- Banding `ts_rank` vs `ts_rank_cd`; banding `%` (trgm) vs `ILIKE %..%` tanpa index.

## Selesai bila
Search/fuzzy/autocomplete balik hasil relevan; test integrasi cek query "laptop" dapat produk relevan & fuzzy "laptp" tetap kena.
