CREATE INDEX IF NOT EXISTS idx_products_attrs ON products USING gin(attributes jsonb_path_ops);
ALTER TABLE products
  ADD COLUMN IF NOT EXISTS brand text
  GENERATED ALWAYS AS (attributes->>'brand') STORED;
CREATE INDEX IF NOT EXISTS idx_products_brand ON products(brand);
