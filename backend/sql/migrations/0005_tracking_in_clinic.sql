-- +goose Up
ALTER TABLE clinics ADD COLUMN track_status TEXT NOT NULL DEFAULT 'not_contacted';

UPDATE clinics
SET track_status = (
    SELECT te.status
    FROM tracking_entries te
    WHERE te.clinic_id = clinics.id
    ORDER BY te.updated_at DESC, te.id DESC
    LIMIT 1
)
WHERE EXISTS (
    SELECT 1 FROM tracking_entries te WHERE te.clinic_id = clinics.id
);

DROP TABLE tracking_entries;

-- +goose Down
CREATE TABLE tracking_entries (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    clinic_id  INTEGER NOT NULL REFERENCES clinics(id) ON DELETE CASCADE,
    status     TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tracking_entries_clinic_id ON tracking_entries(clinic_id);

INSERT INTO tracking_entries (clinic_id, status)
SELECT id, track_status
FROM clinics
WHERE track_status <> 'not_contacted';

ALTER TABLE clinics DROP COLUMN track_status;