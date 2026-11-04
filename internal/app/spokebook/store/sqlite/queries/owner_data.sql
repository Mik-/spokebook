-- Deleting bikes, components, steps and types cascades to their readings,
-- installations, maintenances, values and step details.

-- Rides and gear mappings point to bikes without cascading.
-- name: DeleteOwnerRides :exec
DELETE FROM rides WHERE owner_id = ?;

-- name: DeleteOwnerRideGears :exec
DELETE FROM ride_gears WHERE owner_id = ?;

-- name: DeleteOwnerBikes :exec
DELETE FROM bikes WHERE owner_id = ?;

-- name: DeleteOwnerComponents :exec
DELETE FROM components WHERE owner_id = ?;

-- name: DeleteOwnerSteps :exec
DELETE FROM steps WHERE owner_id = ?;

-- name: DeleteOwnerComponentTypes :exec
DELETE FROM component_types WHERE owner_id = ?;

-- name: DeleteOwnerLevelIntervals :exec
DELETE FROM level_intervals WHERE owner_id = ?;

-- name: DeleteOwnerAttachments :exec
DELETE FROM attachments WHERE owner_id = ?;

-- name: DeleteOwnerShoppingItems :exec
DELETE FROM shopping_items WHERE owner_id = ?;
