# Case 05 — Geospatial (earthdistance)

**Depends:** 00. **Tujuan:** Cari seller/toko terdekat dari koordinat, tanpa PostGIS.

## Yang dipelajari
- `cube` + `earthdistance`: tipe `earth`, `ll_to_earth(lat,lng)`, `earth_distance`, `earth_box`.
- Index GiST pada ekspresi `ll_to_earth` untuk nearest-neighbor cepat.
- Kenapa `earth_box` (bounding) dipakai untuk filter kasar sebelum jarak presisi.

## Migration `0006_geo.sql`
```sql
CREATE INDEX IF NOT EXISTS idx_sellers_earth
  ON sellers USING gist (ll_to_earth(lat, lng));
```
Generator: `sellers.lat/lng` tersebar di beberapa cluster kota (biar hasil "terdekat" bermakna).

## Endpoints (`internal/cases/geo`)
- `GET /api/sellers/nearby?lat=&lng=&radius_km=&limit=` →
  ```sql
  SELECT id,name, earth_distance(ll_to_earth($1,$2), ll_to_earth(lat,lng))/1000 AS km
  FROM sellers
  WHERE earth_box(ll_to_earth($1,$2), $3*1000) @> ll_to_earth(lat,lng)
    AND earth_distance(ll_to_earth($1,$2), ll_to_earth(lat,lng)) <= $3*1000
  ORDER BY km LIMIT $4;
  ```

## Frontend `/ui/nearby`
Input lat/lng + radius (atau tombol "pakai lokasi saya" via `navigator.geolocation`) → daftar seller + jarak.

## Latihan EXPLAIN
- Dengan vs tanpa GiST index (`earth_box` pakai index, `earth_distance` murni = seq scan).
- Efek `radius` kecil vs besar pada jumlah baris.

## Selesai bila
Nearby balik seller urut jarak naik; test: titik dekat cluster kota kembalikan seller kota itu dulu.
