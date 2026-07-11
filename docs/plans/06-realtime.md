# Case 06 — Realtime LISTEN/NOTIFY → SSE

**Depends:** 00 (lebih baik setelah 03 agar ada event order). **Tujuan:** Push event order ke browser realtime pakai fitur PG murni.

## Yang dipelajari
- `LISTEN` / `NOTIFY channel, payload` dan `pg_notify()`.
- Trigger `AFTER INSERT` yang memanggil `pg_notify`.
- Konsumsi notifikasi dari Go: `pgxpool` dedicated conn + `conn.WaitForNotification(ctx)`.
- Batas payload NOTIFY (~8000 byte) → kirim id, bukan seluruh row.

## Migration `0007_notify.sql`
```sql
CREATE OR REPLACE FUNCTION notify_order() RETURNS trigger AS $$
BEGIN
  PERFORM pg_notify('orders', json_build_object(
    'id', NEW.id, 'status', NEW.status, 'total', NEW.total)::text);
  RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_notify_order AFTER INSERT ON orders
  FOR EACH ROW EXECUTE FUNCTION notify_order();
```

## Endpoints (`internal/cases/realtime`)
- `GET /api/events/stream` → SSE (`text/event-stream`). Ambil satu koneksi dari pool, `LISTEN orders`, loop `WaitForNotification`, tulis `data: <payload>\n\n`, flush. Hormati `ctx.Done()` saat klien putus.
- `POST /api/orders/simulate` → INSERT 1 order (trigger memicu notify) untuk demo.

## Frontend `/ui/realtime`
`new EventSource('/api/events/stream')` → tambahkan tiap order baru ke daftar live. Tombol "simulate order".

## Catatan
- Satu conn LISTEN dipakai bersama semua klien SSE (fan-out di Go via channel), **jangan** satu LISTEN-conn per klien. `// ponytail: satu listener + fan-out; cukup untuk demo`.

## Selesai bila
Buka `/ui/realtime`, klik simulate → baris muncul tanpa refresh; test: NOTIFY diterima Go dalam <1s.
