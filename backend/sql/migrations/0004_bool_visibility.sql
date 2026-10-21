-- +goose Up
ALTER TABLE clinics ADD COLUMN is_visible BOOLEAN NOT NULL DEFAULT 1;
 
UPDATE clinics SET is_visible = CASE visibility WHEN 'hidden' THEN 0 ELSE 1 END;
 
ALTER TABLE clinics DROP COLUMN visibility;
 
-- +goose Down
ALTER TABLE clinics ADD COLUMN visibility TEXT NOT NULL DEFAULT 'visible';
 
UPDATE clinics SET visibility = CASE is_visible WHEN 0 THEN 'hidden' ELSE 'visible' END;
 
ALTER TABLE clinics DROP COLUMN is_visible;
