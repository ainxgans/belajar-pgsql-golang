CREATE TABLE users (
  id bigserial PRIMARY KEY,
  email text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE categories (
  id bigserial PRIMARY KEY,
  name text NOT NULL,
  parent_id bigint REFERENCES categories(id)
);

CREATE TABLE sellers (
  id bigserial PRIMARY KEY,
  name text NOT NULL,
  lat double precision NOT NULL,
  lng double precision NOT NULL
);

CREATE TABLE products (
  id bigserial PRIMARY KEY,
  seller_id bigint NOT NULL REFERENCES sellers(id),
  category_id bigint NOT NULL REFERENCES categories(id),
  name text NOT NULL,
  description text NOT NULL,
  price numeric(12,2) NOT NULL,
  attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE reviews (
  id bigserial PRIMARY KEY,
  product_id bigint NOT NULL REFERENCES products(id),
  user_id bigint NOT NULL REFERENCES users(id),
  body text NOT NULL,
  rating smallint NOT NULL CHECK (rating BETWEEN 1 AND 5),
  created_at timestamptz NOT NULL DEFAULT now()
);
