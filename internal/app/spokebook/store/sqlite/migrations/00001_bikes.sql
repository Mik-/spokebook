-- +goose Up
CREATE TABLE bikes (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL DEFAULT '',
    notes      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
) STRICT;

CREATE INDEX bikes_owner_idx ON bikes (owner_id);

-- +goose Down
DROP TABLE bikes;
