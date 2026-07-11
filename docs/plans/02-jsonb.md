# Case 02 — JSONB Atribut Dinamis

**Depends:** 00. **Tujuan:** Produk punya atribut bervariasi per kategori (warna, ukuran, brand, spec) di `products.attributes jsonb`; query & index-nya.

## Yang dipelajari
- Operator `->`, `->>`, `#>`, `@>`, `?`, `?|`, `?&`; `jsonb_path_query` / `@?` / `@@`.
- Index GIN (`jsonb_path_ops` vs default) untuk containment.
- `generated column` dari JSONB untuk kolom yang sering difilter.
- Agregasi `jsonb_agg`, `jsonb_object_agg`.

## Migration `0003_jsonb.sql`
```sql
CREATE INDEX IF NOT EXISTS idx_products_attrs ON products USING gin(attributes jsonb_path_ops);
ALTER TABLE products
  ADD COLUMN IF NOT EXISTS brand text
  GENERATED ALWAYS AS (attributes->>'brand') STORED;
CREATE INDEX IF NOT EXISTS idx_products_brand ON products(brand);
```
Generator: isi `attributes` sesuai kategori — mis. elektronik `{brand, ram, storage}`, fashion `{brand, color, size}`.

## Endpoints (`internal/cases/jsonb`)
- `GET /api/products?attr.color=red&attr.brand=X&min_price=` → bangun `WHERE attributes @> $1` dari query param (containment).
- `GET /api/products/facets?category_id=` → hitung distribusi nilai atribut (mis. jumlah per warna) via `jsonb_each`/`GROUP BY`.

## Frontend `/ui/products`
Filter form (brand/color/size) → hasil kartu produk; sidebar facet count.

## Latihan EXPLAIN
- `@>` dengan vs tanpa GIN.
- Filter `attributes->>'brand'=` (butuh ekspresi index / generated column) vs kolom generated `brand`.
- `jsonb_path_ops` vs GIN default (ukuran index & query yang didukung).

## Selesai bila
Filter kombinasi atribut balik hasil benar; facet count akurat; test cek containment `{"color":"red"}` sesuai.
