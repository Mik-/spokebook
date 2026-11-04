-- name: SeedComponentType :exec
INSERT INTO component_types (owner_id, id, created_at) VALUES (?, ?, ?)
ON CONFLICT (owner_id, id) DO NOTHING;

-- name: CreateComponentType :one
INSERT INTO component_types (owner_id, id, name, created_at) VALUES (?, ?, ?, ?)
RETURNING *;

-- name: ListComponentTypes :many
SELECT * FROM component_types WHERE owner_id = ? ORDER BY created_at, id;

-- name: RenameComponentType :execrows
UPDATE component_types SET name = ? WHERE owner_id = ? AND id = ? AND hidden = 0;

-- name: HideComponentType :execrows
UPDATE component_types SET hidden = 1 WHERE owner_id = ? AND id = ? AND hidden = 0;

-- name: ShowComponentType :execrows
UPDATE component_types SET hidden = 0 WHERE owner_id = ? AND id = ? AND hidden = 1;

-- name: CountComponentsOfType :one
SELECT COUNT(*) FROM components WHERE owner_id = ? AND type = ?;
