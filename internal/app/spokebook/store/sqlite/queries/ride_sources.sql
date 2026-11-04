-- name: GetRideSource :one
SELECT * FROM ride_sources WHERE owner_id = ? AND source = ?;

-- name: ListAllRideSources :many
SELECT * FROM ride_sources ORDER BY owner_id, source;

-- name: CreateRideSource :exec
INSERT INTO ride_sources (owner_id, source, credentials, since, cursor, last_run_at, last_error, last_imported,
                          last_unassigned, more, created_at)
VALUES (?, ?, ?, ?, NULL, NULL, '', 0, 0, 0, ?);

-- name: SetRideSourceCredentials :execrows
UPDATE ride_sources SET credentials = ? WHERE owner_id = ? AND source = ?;

-- name: SetRideSourceSince :execrows
UPDATE ride_sources SET since = ?, cursor = ? WHERE owner_id = ? AND source = ?;

-- name: SetRideSourceRun :execrows
UPDATE ride_sources
SET cursor = ?, last_run_at = ?, last_error = ?, last_imported = ?, last_unassigned = ?, more = ?
WHERE owner_id = ? AND source = ?;

-- name: DeleteRideSource :execrows
DELETE FROM ride_sources WHERE owner_id = ? AND source = ?;

-- name: DeleteRideSourceItems :exec
DELETE FROM ride_source_items WHERE owner_id = ? AND source = ?;

-- name: HasRideSourceItem :one
SELECT EXISTS (SELECT 1 FROM ride_source_items WHERE owner_id = ? AND source = ? AND item_id = ?);

-- name: AddRideSourceItem :exec
INSERT INTO ride_source_items (owner_id, source, item_id, created_at) VALUES (?, ?, ?, ?)
ON CONFLICT DO NOTHING;
