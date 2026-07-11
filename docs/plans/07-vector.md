# Case 07 — Vector Similarity (pgvector)

**Depends:** 00 (produk). **Tujuan:** Semantic / "produk mirip" pakai `pgvector`.

## Yang dipelajari
- Tipe `vector(n)`; operator jarak `<->` (L2), `<#>` (inner product), `<=>` (cosine).
- Index ANN: `ivfflat` vs `hnsw` (trade-off build time / recall / speed).
- `SET LOCAL ivfflat.probes` / `hnsw.ef_search` untuk atur recall.

## Migration `0008_vector.sql`
```sql
-- embedding vector(128) sudah ada di products (case 00). Isi via generator.
CREATE INDEX IF NOT EXISTS idx_products_embedding
  ON products USING hnsw (embedding vector_cosine_ops);
```

## Generator embedding (`internal/gen`)
- Tiap kategori punya centroid acak dim-128 (seed tetap). Embedding produk = centroid kategori + noise gaussian kecil, lalu normalisasi.
- Efek: produk sekategori berdekatan → "mirip" bermakna, bukan random.
- Isi via `UPDATE ... = $1::vector` batch, atau CopyFrom saat generate products (case 00 kosongkan, case ini isi).

## Endpoints (`internal/cases/vector`)
- `GET /api/search/semantic?product_id=&limit=` → "produk mirip":
  ```sql
  SELECT id,name, embedding <=> (SELECT embedding FROM products WHERE id=$1) AS dist
  FROM products WHERE id<>$1 ORDER BY dist LIMIT $2;
  ```
- (Opsional) `GET /api/search/semantic?q=` → map kata kunci ke centroid kategori terdekat lalu cari. `// ponytail: tanpa model embedding nyata; pakai centroid kategori`.

## Frontend `/ui/semantic`
Pilih produk → tampil daftar "mirip" + skor jarak.

## Latihan EXPLAIN
- `<=>` dengan HNSW index vs seq scan (waktu & recall).
- Ubah `hnsw.ef_search` → banding kecepatan vs kualitas hasil.
- Banding `ivfflat` vs `hnsw` (build & query).

## Selesai bila
"Produk mirip" mayoritas sekategori dengan sumber; test: top-5 mirip berbagi category_id dengan produk sumber.
