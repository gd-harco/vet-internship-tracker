-- +goose Up
CREATE TABLE IF NOT EXISTS clinics (
  id INTEGER PRIMARY KEY,
  external_id TEXT UNIQUE NOT NULL,
  name TEXT NOT NULL,
  phone TEXT,
  address_way TEXT,
  address_additional1 TEXT,
  address_additional2 TEXT,
  zip_code TEXT,
  city TEXT,
  vet_lastname TEXT,
  vet_firstname TEXT,
  vet_ordinal_number INTEGER,
  fetched_at DATETIME NOT NULL,
  source TEXT NOT NULL DEFAULT 'fetched',
  visibility TEXT NOT NULL DEFAULT 'visible'
);

-- +goose Down
DROP TABLE IF EXISTS clinics;
