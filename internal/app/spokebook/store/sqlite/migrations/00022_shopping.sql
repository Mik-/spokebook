-- +goose Up
-- Things to buy, e.g. consumables noticed during a maintenance. A bought
-- item stays on the list (ticked) until the owner removes bought items.
CREATE TABLE shopping_items (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL,
    text       TEXT NOT NULL CHECK (text <> ''),
    bought_at  TEXT,
    created_at TEXT NOT NULL
) STRICT;

CREATE INDEX shopping_items_owner_idx ON shopping_items (owner_id, created_at);

-- +goose Down
DROP TABLE shopping_items;
