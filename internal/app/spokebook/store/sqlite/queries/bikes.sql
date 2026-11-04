-- name: CreateBike :one
INSERT INTO bikes (id, owner_id, name, type, notes, origin, purchased_at, dealer, price_cents,
                   currency, in_service_at, in_service_m, features, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetBike :one
SELECT * FROM bikes WHERE owner_id = ? AND id = ?;

-- name: ListBikes :many
SELECT * FROM bikes WHERE owner_id = ? ORDER BY name;

-- name: UpdateBike :execrows
UPDATE bikes
SET name = ?, type = ?, notes = ?, origin = ?, purchased_at = ?, dealer = ?, price_cents = ?,
    currency = ?, in_service_at = ?, in_service_m = ?, features = ?
WHERE owner_id = ? AND id = ?;

-- name: DeleteBike :execrows
DELETE FROM bikes WHERE owner_id = ? AND id = ?;

-- name: SetBikeRetired :execrows
UPDATE bikes SET retired_at = ?, retire_reason = ? WHERE owner_id = ? AND id = ?;
