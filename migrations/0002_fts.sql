ALTER TABLE products
  ADD COLUMN IF NOT EXISTS search tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', coalesce(name,'') || ' ' || coalesce(description,''))) STORED;
CREATE INDEX IF NOT EXISTS idx_products_search ON products USING gin(search);
CREATE INDEX IF NOT EXISTS idx_products_name_trgm ON products USING gin(name gin_trgm_ops);

ALTER TABLE reviews
  ADD COLUMN IF NOT EXISTS search tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', coalesce(body,''))) STORED;
CREATE INDEX IF NOT EXISTS idx_reviews_search ON reviews USING gin(search);
