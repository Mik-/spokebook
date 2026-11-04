-- name: CreateMaintenance :exec
INSERT INTO maintenances (id, owner_id, bike_id, title, level, performed_at, distance_m, notes, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: AddMaintenanceStep :exec
INSERT INTO maintenance_steps (id, owner_id, maintenance_id, position, catalog_step_id, builtin_id, title,
                               instructions, phase, assembly, action, module, hints, origin_id, added)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: AddMaintenanceValue :exec
INSERT INTO maintenance_values (id, owner_id, step_id, position, component_id, component_name, component_type,
                                component_position, key, kind, label, value_from, value_to, unit, hint)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetMaintenance :one
SELECT * FROM maintenances WHERE owner_id = ? AND id = ?;

-- name: ListMaintenances :many
SELECT * FROM maintenances WHERE owner_id = ? AND bike_id = ? ORDER BY performed_at DESC, created_at DESC;

-- name: ListMaintenanceSteps :many
SELECT * FROM maintenance_steps WHERE owner_id = ? AND maintenance_id = ? ORDER BY position, rowid;

-- name: ListMaintenanceValues :many
SELECT maintenance_values.* FROM maintenance_values
JOIN maintenance_steps ON maintenance_steps.id = maintenance_values.step_id
WHERE maintenance_steps.owner_id = ? AND maintenance_steps.maintenance_id = ?
ORDER BY maintenance_values.position;

-- name: SetMaintenanceNotes :execrows
UPDATE maintenances SET notes = ? WHERE owner_id = ? AND id = ?;

-- name: SetMaintenanceCompleted :execrows
UPDATE maintenances SET completed_at = ? WHERE owner_id = ? AND id = ?;

-- name: SetMaintenanceStepResult :execrows
UPDATE maintenance_steps SET done = ?, note = ? WHERE owner_id = ? AND id = ?;

-- name: SetMaintenanceStepDismissed :execrows
UPDATE maintenance_steps SET dismissed = ? WHERE owner_id = ? AND id = ?;

-- name: DeleteMaintenanceStep :execrows
DELETE FROM maintenance_steps WHERE owner_id = ? AND id = ?;

-- name: SetMaintenanceMeasured :execrows
UPDATE maintenance_values SET measured = ? WHERE owner_id = ? AND id = ?;

-- name: DeleteMaintenance :execrows
DELETE FROM maintenances WHERE owner_id = ? AND id = ?;

-- name: SetMaintenanceDate :execrows
UPDATE maintenances SET performed_at = ?, distance_m = ? WHERE owner_id = ? AND id = ?;
