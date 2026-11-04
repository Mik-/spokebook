-- name: CreateRide :one
INSERT INTO rides (id, owner_id, bike_id, source, external_id, gear_key, bike_by_gear, started_at, ended_at,
                   ridden_on, distance_m, name, discarded, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetRide :one
SELECT * FROM rides WHERE owner_id = ? AND id = ?;

-- name: FindRide :one
SELECT * FROM rides WHERE owner_id = ? AND source = ? AND external_id = ?;

-- name: ListRides :many
SELECT * FROM rides WHERE owner_id = ? ORDER BY started_at DESC, created_at DESC;

-- name: OverlappingRides :many
SELECT * FROM rides
WHERE owner_id = ? AND started_at < sqlc.arg(until) AND ended_at > sqlc.arg(since)
ORDER BY started_at;

-- name: SetRideBike :execrows
UPDATE rides SET bike_id = ?, bike_by_gear = ? WHERE owner_id = ? AND id = ?;

-- name: SetRideDiscarded :execrows
UPDATE rides SET discarded = ? WHERE owner_id = ? AND id = ?;

-- The distance per bike and day of the assigned rides that are not discarded.
-- name: DailyDistances :many
SELECT CAST(bike_id AS TEXT) AS bike_id, ridden_on, CAST(SUM(distance_m) AS INTEGER) AS distance_m
FROM rides
WHERE owner_id = ? AND bike_id IS NOT NULL AND discarded = 0
GROUP BY bike_id, ridden_on
ORDER BY bike_id, ridden_on;

-- name: UnassignBikeRides :exec
UPDATE rides SET bike_id = NULL, bike_by_gear = 0 WHERE owner_id = ? AND bike_id = ?;

-- name: CreateRideGear :one
INSERT INTO ride_gears (id, owner_id, source, gear_key, name, bike_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetRideGear :one
SELECT * FROM ride_gears WHERE owner_id = ? AND id = ?;

-- name: FindRideGear :one
SELECT * FROM ride_gears WHERE owner_id = ? AND source = ? AND gear_key = ?;

-- name: ListRideGears :many
SELECT * FROM ride_gears WHERE owner_id = ? ORDER BY source, name, gear_key;

-- name: SetRideGearBike :execrows
UPDATE ride_gears SET bike_id = ? WHERE owner_id = ? AND id = ?;

-- name: UnassignBikeGears :exec
UPDATE ride_gears SET bike_id = NULL WHERE owner_id = ? AND bike_id = ?;

-- name: DeleteRide :execrows
DELETE FROM rides WHERE owner_id = ? AND id = ?;

-- name: DeleteRideGear :execrows
DELETE FROM ride_gears WHERE owner_id = ? AND id = ?;
