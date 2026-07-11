CREATE INDEX IF NOT EXISTS idx_sellers_earth
  ON sellers USING gist (ll_to_earth(lat, lng));
