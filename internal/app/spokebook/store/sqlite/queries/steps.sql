-- name: CreateStep :exec
INSERT INTO steps (id, owner_id, builtin_id, position, title, instructions, phase, assembly, action, module,
                   replaces, interval_months, interval_m, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListSteps :many
SELECT * FROM steps WHERE owner_id = ? ORDER BY position, created_at;

-- name: UpdateStep :execrows
UPDATE steps SET title = ?, instructions = ?, phase = ?, assembly = ?, action = ?, module = ?, position = ?,
                 interval_months = ?, interval_m = ?
WHERE owner_id = ? AND id = ?;

-- name: SetStepPosition :execrows
UPDATE steps SET position = ? WHERE owner_id = ? AND id = ?;

-- name: HideStep :execrows
UPDATE steps SET hidden = 1 WHERE owner_id = ? AND id = ?;

-- name: ShowStep :execrows
UPDATE steps SET hidden = 0 WHERE owner_id = ? AND id = ?;

-- name: DeleteStep :execrows
DELETE FROM steps WHERE owner_id = ? AND id = ?;

-- name: ListStepLevels :many
-- In the order they were added.
SELECT * FROM step_levels WHERE owner_id = ? ORDER BY rowid;

-- name: DeleteStepLevels :exec
DELETE FROM step_levels WHERE owner_id = ? AND step_id = ?;

-- name: AddStepLevel :exec
INSERT INTO step_levels (step_id, owner_id, level) VALUES (?, ?, ?);

-- name: ListStepComponentTypes :many
SELECT * FROM step_component_types WHERE owner_id = ? ORDER BY component_type;

-- name: DeleteStepComponentTypes :exec
DELETE FROM step_component_types WHERE owner_id = ? AND step_id = ?;

-- name: AddStepComponentType :exec
INSERT INTO step_component_types (step_id, owner_id, component_type) VALUES (?, ?, ?);

-- name: ListStepValueKinds :many
-- In the order they were added.
SELECT * FROM step_value_kinds WHERE owner_id = ? ORDER BY rowid;

-- name: DeleteStepValueKinds :exec
DELETE FROM step_value_kinds WHERE owner_id = ? AND step_id = ?;

-- name: AddStepValueKind :exec
INSERT INTO step_value_kinds (step_id, owner_id, kind) VALUES (?, ?, ?);

-- name: ListStepValues :many
SELECT * FROM step_values WHERE owner_id = ? ORDER BY position;

-- name: DeleteStepValues :exec
DELETE FROM step_values WHERE owner_id = ? AND step_id = ?;

-- name: AddStepValue :exec
INSERT INTO step_values (id, owner_id, step_id, position, key, kind, label, value_from, value_to, unit, hint)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListStepFollowUps :many
SELECT * FROM step_follow_ups WHERE owner_id = ? ORDER BY position, rowid;

-- name: DeleteStepFollowUps :exec
DELETE FROM step_follow_ups WHERE owner_id = ? AND step_id = ?;

-- name: AddStepFollowUp :exec
INSERT INTO step_follow_ups (step_id, owner_id, follow_up_id, kind, position) VALUES (?, ?, ?, ?, ?);

-- name: ListLevelIntervals :many
SELECT * FROM level_intervals WHERE owner_id = ? ORDER BY level;

-- name: SetLevelInterval :exec
INSERT INTO level_intervals (owner_id, level, interval_months, interval_m) VALUES (?, ?, ?, ?)
ON CONFLICT (owner_id, level) DO UPDATE SET interval_months = excluded.interval_months, interval_m = excluded.interval_m;
