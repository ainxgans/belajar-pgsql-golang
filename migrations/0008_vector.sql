ALTER TABLE products ADD COLUMN IF NOT EXISTS embedding vector(128);
CREATE INDEX IF NOT EXISTS idx_products_embedding
  ON products USING hnsw (embedding vector_cosine_ops);
