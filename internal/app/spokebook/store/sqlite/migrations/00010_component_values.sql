-- +goose Up
-- Values of a component such as torques or wear limits. value_from alone is
-- a minimum, value_to alone a maximum, both a range (equal: a fixed value).
CREATE TABLE component_values (
    id           TEXT PRIMARY KEY,
    owner_id     TEXT NOT NULL,
    component_id TEXT NOT NULL,
    position     INTEGER NOT NULL,
    label        TEXT NOT NULL,
    value_from   REAL,
    value_to     REAL,
    unit         TEXT NOT NULL DEFAULT '',
    CHECK (value_from IS NOT NULL OR value_to IS NOT NULL),
    CHECK (value_from IS NULL OR value_to IS NULL OR value_to >= value_from),
    FOREIGN KEY (component_id, owner_id) REFERENCES components (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE INDEX component_values_component_idx ON component_values (component_id, position);

-- +goose Down
DROP TABLE component_values;
