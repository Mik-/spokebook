-- name: AddReading :one
INSERT INTO odometer_readings (id, owner_id, bike_id, recorded_at, distance_m, source, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListReadings :many
SELECT * FROM odometer_readings
WHERE owner_id = ? AND bike_id = ?
ORDER BY recorded_at DESC, created_at DESC;

-- name: PrevReading :one
SELECT * FROM odometer_readings
WHERE owner_id = ? AND bike_id = ? AND recorded_at <= ? AND id != sqlc.arg(exclude_id)
ORDER BY recorded_at DESC, distance_m DESC
LIMIT 1;

-- name: NextReading :one
SELECT * FROM odometer_readings
WHERE owner_id = ? AND bike_id = ? AND recorded_at > ? AND id != sqlc.arg(exclude_id)
ORDER BY recorded_at ASC, distance_m ASC
LIMIT 1;

-- name: ListAllReadings :many
SELECT * FROM odometer_readings WHERE owner_id = ? ORDER BY bike_id, recorded_at;

-- name: GetReading :one
SELECT * FROM odometer_readings WHERE owner_id = ? AND id = ?;

-- name: UpdateReading :execrows
UPDATE odometer_readings SET recorded_at = ?, distance_m = ? WHERE owner_id = ? AND id = ?;

-- name: DeleteReading :execrows
DELETE FROM odometer_readings WHERE owner_id = ? AND id = ?;
