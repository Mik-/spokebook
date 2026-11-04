-- name: CreateComponent :one
INSERT INTO components (id, owner_id, type, manufacturer, model, part_number, purchased_at, dealer,
                        price_cents, currency, max_distance_m, replace_by, url, notes, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetComponent :one
SELECT * FROM components WHERE owner_id = ? AND id = ?;

-- name: ListComponents :many
SELECT * FROM components WHERE owner_id = ? ORDER BY type, manufacturer, model;

-- name: AddInstallation :one
INSERT INTO installations (id, owner_id, component_id, bike_id, position, installed_at, installed_m,
                           installed_auto, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetInstallation :one
SELECT * FROM installations WHERE owner_id = ? AND id = ?;

-- name: CloseInstallation :execrows
UPDATE installations SET removed_at = ?, removed_m = ?, removed_auto = ?
WHERE owner_id = ? AND id = ? AND removed_at IS NULL;

-- name: ListInstallations :many
SELECT * FROM installations
WHERE owner_id = ? AND component_id = ?
ORDER BY installed_at DESC, created_at DESC;

-- name: ListAllInstallations :many
SELECT * FROM installations WHERE owner_id = ? ORDER BY installed_at;

-- name: UpdateComponent :execrows
UPDATE components
SET type = ?, manufacturer = ?, model = ?, part_number = ?, purchased_at = ?, dealer = ?, price_cents = ?,
    currency = ?, max_distance_m = ?, replace_by = ?, url = ?, notes = ?
WHERE owner_id = ? AND id = ?;

-- name: DeleteComponent :execrows
DELETE FROM components WHERE owner_id = ? AND id = ?;

-- Dealers of components and bikes.
-- name: ListDealers :many
SELECT dealer FROM components WHERE components.owner_id = sqlc.arg(owner_id) AND dealer != ''
UNION
SELECT dealer FROM bikes WHERE bikes.owner_id = sqlc.arg(owner_id) AND dealer != ''
ORDER BY dealer;

-- name: UpdateInstallation :execrows
UPDATE installations
SET bike_id = ?, position = ?, installed_at = ?, installed_m = ?, installed_auto = ?, removed_at = ?,
    removed_m = ?, removed_auto = ?
WHERE owner_id = ? AND id = ?;

-- name: SetInstallationDistances :execrows
UPDATE installations SET installed_m = ?, removed_m = ? WHERE owner_id = ? AND id = ?;

-- name: DeleteInstallation :execrows
DELETE FROM installations WHERE owner_id = ? AND id = ?;

-- name: CountInstallationsOnBike :one
SELECT COUNT(*) FROM installations WHERE owner_id = ? AND bike_id = ?;

-- name: SetComponentRetired :execrows
UPDATE components SET retired_at = ?, retire_reason = ? WHERE owner_id = ? AND id = ?;

-- name: ListManufacturers :many
SELECT DISTINCT manufacturer FROM components WHERE owner_id = ? AND manufacturer != '' ORDER BY manufacturer;
