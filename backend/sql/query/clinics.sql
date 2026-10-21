-- name: CreateClinic :execresult
INSERT INTO clinics (
  external_id,
  name,
  phone,
  address_way,
  address_additional1,
  address_additional2,
  zip_code,
  city,
  vet_lastname,
  vet_firstname,
  vet_ordinal_number,
  fetched_at,
  source,
  is_visible,
  track_status
) VALUES (
  ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
);

-- name: UpsertClinic :execresult
INSERT INTO clinics (
  external_id,
  name,
  phone,
  address_way,
  address_additional1,
  address_additional2,
  zip_code,
  city,
  vet_lastname,
  vet_firstname,
  vet_ordinal_number,
  fetched_at,
  source,
  is_visible,
  track_status
) VALUES (
  ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT(external_id) DO UPDATE SET
  name = excluded.name,
  phone = excluded.phone,
  address_way = excluded.address_way,
  address_additional1 = excluded.address_additional1,
  address_additional2 = excluded.address_additional2,
  zip_code = excluded.zip_code,
  city = excluded.city,
  vet_lastname = excluded.vet_lastname,
  vet_firstname = excluded.vet_firstname,
  vet_ordinal_number = excluded.vet_ordinal_number,
  fetched_at = excluded.fetched_at,
  source = excluded.source,
  is_visible = COALESCE(clinics.is_visible, excluded.is_visible),
  track_status = COALESCE(clinics.track_status, excluded.track_status);

-- name: UpdateTrackingStatus :execresult
UPDATE clinics
SET track_status = ?
WHERE id = ?;

-- name: GetTrackingByClinicID :one
SELECT
  id,
  track_status
FROM clinics
WHERE id = ?;

-- name: GetVisibleClinics :many
SELECT
  *
FROM clinics
WHERE is_visible = True
ORDER BY id DESC
LIMIT ? OFFSET ?;

-- name: GetClinicByID :one
SELECT
  *
FROM clinics
WHERE id = ?;

-- name: UpdateClinic :execresult
UPDATE clinics
SET name = ?, phone = ?, address_way = ?, zip_code = ?, city = ?
WHERE id = ?;

-- name: HideClinic :exec
UPDATE clinics
SET is_visible = false
WHERE id = ?;

-- name: LastFetchedAt :one
SELECT fetched_at
FROM clinics
WHERE source = 'fetched'
ORDER BY fetched_at DESC
LIMIT 1;

-- name: CountClinics :one
SELECT COUNT(*)
FROM clinics;

-- name: GetNoGeoClinics :many
SELECT *
FROM clinics
WHERE (lat IS NULL OR lng IS NULL)
  AND city IS NOT NULL;

-- name: AddGeoToClinics :exec
UPDATE clinics
SET
  lat = ?,
  lng = ?
WHERE id = ? ;

-- name: GetClinicMapMaker :many
SELECT id,
       track_status,
       CAST(COALESCE(lat, 0) AS REAL) AS lat,
       CAST(COALESCE(lng, 0) AS REAL) AS lng
FROM clinics
WHERE lat IS NOT NULL
  AND lng IS NOT NULL
  AND is_visible = true;

-- name: GetClinicPopup :one
SELECT  name,
        CAST(COALESCE(address_way, '') AS TEXT) AS address_way,
        CAST(COALESCE(city, '') AS TEXT) AS city,
        CAST(COALESCE(zip_code, '') AS TEXT) AS zip_code,
        track_status
FROM clinics
WHERE id = ?;
