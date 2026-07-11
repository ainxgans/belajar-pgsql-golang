CREATE TABLE orders (
  id bigserial,
  user_id bigint NOT NULL REFERENCES users(id),
  status text NOT NULL,
  total numeric(12,2) NOT NULL,
  created_at timestamptz NOT NULL
) PARTITION BY RANGE (created_at);

CREATE TABLE orders_default PARTITION OF orders DEFAULT;

DO $$
DECLARE
  start_month date := date_trunc('month', now() - interval '24 months');
  m date;
  i int;
BEGIN
  FOR i IN 0..26 LOOP
    m := start_month + (i || ' months')::interval;
    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS %I PARTITION OF orders FOR VALUES FROM (%L) TO (%L)',
      'orders_' || to_char(m, 'YYYY_MM'), m, m + interval '1 month'
    );
  END LOOP;
END $$;

-- ponytail: order_items/events skip FK to orders (partitioned table would need a
-- partition-key-inclusive PK to be an FK target); not needed for this case's goal.
CREATE TABLE order_items (
  order_id bigint NOT NULL,
  product_id bigint NOT NULL REFERENCES products(id),
  qty int NOT NULL,
  price numeric(12,2) NOT NULL
);
CREATE INDEX idx_order_items_order ON order_items(order_id);

CREATE TABLE events (
  id bigserial PRIMARY KEY,
  user_id bigint NOT NULL REFERENCES users(id),
  product_id bigint REFERENCES products(id),
  kind text NOT NULL,
  created_at timestamptz NOT NULL
);

CREATE INDEX idx_orders_created ON orders USING brin(created_at);
CREATE INDEX idx_orders_user ON orders(user_id);
CREATE INDEX idx_events_created_brin ON events USING brin(created_at);
