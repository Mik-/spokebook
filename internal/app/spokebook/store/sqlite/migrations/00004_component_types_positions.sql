-- +goose Up
-- Component types per owner. Built-in types use a fixed ID such as 'chain'
-- and an empty name, which means "translate the ID"; own types use a UUID.
-- Types are hidden instead of deleted, so seeding built-ins stays idempotent.
CREATE TABLE component_types (
    owner_id   TEXT NOT NULL,
    id         TEXT NOT NULL,
    name       TEXT NOT NULL DEFAULT '',
    hidden     INTEGER NOT NULL DEFAULT 0 CHECK (hidden IN (0, 1)),
    created_at TEXT NOT NULL,
    PRIMARY KEY (owner_id, id)
) STRICT;

-- Types already used by components.
INSERT INTO component_types (owner_id, id, created_at)
SELECT DISTINCT owner_id, type, strftime('%Y-%m-%dT%H:%M:%S.000000000Z', 'now') FROM components;

ALTER TABLE installations ADD COLUMN position TEXT NOT NULL DEFAULT ''
    CHECK (position IN ('', 'front', 'rear'));

-- +goose Down
ALTER TABLE installations DROP COLUMN position;
DROP TABLE component_types;
