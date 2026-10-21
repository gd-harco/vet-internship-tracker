-- +goose Up
CREATE TABLE IF NOT EXISTS notes (
  id INTEGER PRIMARY KEY,
  clinic_id INTEGER NOT NULL REFERENCES clinics(id),
  content TEXT NOT NULL,
  created_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_clinics_city ON clinics(city);
CREATE INDEX IF NOT EXISTS idx_tracking_status ON tracking_entries(status);
CREATE INDEX IF NOT EXISTS idx_notes_clinic ON notes(clinic_id);

-- +goose Down
DROP TABLE IF EXISTS notes;