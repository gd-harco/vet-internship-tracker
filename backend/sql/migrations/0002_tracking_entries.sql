-- +goose Up
CREATE TABLE IF NOT EXISTS tracking_entries (
  id INTEGER PRIMARY KEY,
  clinic_id INTEGER UNIQUE NOT NULL REFERENCES clinics(id),
  status TEXT NOT NULL DEFAULT 'not_contacted',
  updated_at DATETIME NOT NULL
);


-- +goose Down
DROP TABLE IF EXISTS tracking_entries;
