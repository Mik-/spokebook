-- name: CreateShoppingItem :one
INSERT INTO shopping_items (id, owner_id, text, bought_at, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetShoppingItem :one
SELECT * FROM shopping_items WHERE owner_id = ? AND id = ?;

-- Open items first, then bought ones; each in the order they were added.
-- name: ListShoppingItems :many
SELECT * FROM shopping_items
WHERE owner_id = ?
ORDER BY bought_at IS NOT NULL, created_at, id;

-- name: UpdateShoppingItem :execrows
UPDATE shopping_items SET text = ? WHERE owner_id = ? AND id = ?;

-- name: SetShoppingItemBought :execrows
UPDATE shopping_items SET bought_at = ? WHERE owner_id = ? AND id = ?;

-- name: DeleteShoppingItem :execrows
DELETE FROM shopping_items WHERE owner_id = ? AND id = ?;

-- name: DeleteBoughtShoppingItems :execrows
DELETE FROM shopping_items WHERE owner_id = ? AND bought_at IS NOT NULL;
