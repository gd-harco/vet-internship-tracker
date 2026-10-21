-- name: InsertNote :execresult
INSERT INTO notes (clinic_id, content, created_at)
VALUES (?, ?, ?);

-- name: ListNotesByClinicID :many
SELECT *
FROM notes
WHERE clinic_id = ?
ORDER BY created_at DESC;

-- name: DeleteNote :execresult
DELETE FROM notes
WHERE id = ?;

-- name: CountNotesByClinicID :one
SELECT COUNT(*)
FROM notes
WHERE clinic_id = ?;
