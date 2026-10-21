-- +goose Up
ALTER TABLE clinics ADD COLUMN lat REAL;
ALTER TABLE clinics ADD COLUMN lng REAL;

-- +goose Down
ALTER TABLE clinics DROP COLUMN lng;
ALTER TABLE clinics DROP COLUMN lat;