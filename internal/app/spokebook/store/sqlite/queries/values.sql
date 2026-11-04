-- name: ListComponentValues :many
SELECT * FROM component_values WHERE owner_id = ? AND component_id = ? ORDER BY position;

-- name: DeleteComponentValues :exec
DELETE FROM component_values WHERE owner_id = ? AND component_id = ?;

-- name: AddComponentValue :exec
INSERT INTO component_values (id, owner_id, component_id, position, key, kind, label, value_from, value_to, unit, hint)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListTypeValues :many
SELECT * FROM component_type_values WHERE owner_id = ? AND type_id = ? ORDER BY position;

-- name: DeleteTypeValues :exec
DELETE FROM component_type_values WHERE owner_id = ? AND type_id = ?;

-- name: AddTypeValue :exec
INSERT INTO component_type_values (id, owner_id, type_id, position, key, kind, label, value_from, value_to, unit, hint)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- Labels and units of component and step values.
-- name: ListValueLabels :many
SELECT label FROM component_values WHERE component_values.owner_id = sqlc.arg(owner_id)
UNION
SELECT label FROM step_values WHERE step_values.owner_id = sqlc.arg(owner_id)
UNION
SELECT label FROM component_type_values WHERE component_type_values.owner_id = sqlc.arg(owner_id)
ORDER BY label;

-- name: ListValueUnits :many
SELECT unit FROM component_values WHERE component_values.owner_id = sqlc.arg(owner_id) AND unit != ''
UNION
SELECT unit FROM step_values WHERE step_values.owner_id = sqlc.arg(owner_id) AND unit != ''
UNION
SELECT unit FROM component_type_values WHERE component_type_values.owner_id = sqlc.arg(owner_id) AND unit != ''
ORDER BY unit;

-- name: ListBikeValues :many
SELECT * FROM bike_values WHERE owner_id = ? AND bike_id = ? ORDER BY position;

-- name: DeleteBikeValues :exec
DELETE FROM bike_values WHERE owner_id = ? AND bike_id = ?;

-- name: AddBikeValue :exec
INSERT INTO bike_values (id, owner_id, bike_id, position, key, kind, label, value_from, value_to, unit, hint)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListBikeValueLabels :many
SELECT DISTINCT label FROM bike_values WHERE owner_id = ? ORDER BY label;

-- name: ListBikeValueUnits :many
SELECT DISTINCT unit FROM bike_values WHERE owner_id = ? AND unit != '' ORDER BY unit;
