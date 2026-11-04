-- +goose Up
-- Values get a stable key (derived from the label, so that a component's
-- value can override the default of its type), a kind and a hint. Kinds:
-- 'torque' is shown only, 'limit' (one bound) and 'target' are measured.
-- Existing values: 'torque' for Nm, 'limit' for one bound, else 'target';
-- the key approximates values.Key for usual labels.
ALTER TABLE component_values ADD COLUMN key TEXT NOT NULL DEFAULT '';
ALTER TABLE component_values ADD COLUMN kind TEXT NOT NULL DEFAULT 'target'
    CHECK (kind IN ('torque', 'limit', 'target'));
ALTER TABLE component_values ADD COLUMN hint TEXT NOT NULL DEFAULT '';
UPDATE component_values SET key = lower(replace(replace(replace(replace(replace(trim(label), ' ', '_'), '-', '_'), '/', '_'), '.', ''), ',', '')),
    kind = CASE WHEN unit = 'Nm' THEN 'torque'
                WHEN value_from IS NULL OR value_to IS NULL THEN 'limit'
                ELSE 'target' END;

ALTER TABLE plan_step_values ADD COLUMN key TEXT NOT NULL DEFAULT '';
ALTER TABLE plan_step_values ADD COLUMN kind TEXT NOT NULL DEFAULT 'target'
    CHECK (kind IN ('torque', 'limit', 'target'));
ALTER TABLE plan_step_values ADD COLUMN hint TEXT NOT NULL DEFAULT '';
UPDATE plan_step_values SET key = lower(replace(replace(replace(replace(replace(trim(label), ' ', '_'), '-', '_'), '/', '_'), '.', ''), ',', '')),
    kind = CASE WHEN unit = 'Nm' THEN 'torque'
                WHEN value_from IS NULL OR value_to IS NULL THEN 'limit'
                ELSE 'target' END;

ALTER TABLE maintenance_values ADD COLUMN key TEXT NOT NULL DEFAULT '';
ALTER TABLE maintenance_values ADD COLUMN kind TEXT NOT NULL DEFAULT 'target'
    CHECK (kind IN ('torque', 'limit', 'target'));
ALTER TABLE maintenance_values ADD COLUMN hint TEXT NOT NULL DEFAULT '';
UPDATE maintenance_values SET key = lower(replace(replace(replace(replace(replace(trim(label), ' ', '_'), '-', '_'), '/', '_'), '.', ''), ',', '')),
    kind = CASE WHEN unit = 'Nm' THEN 'torque'
                WHEN value_from IS NULL OR value_to IS NULL THEN 'limit'
                ELSE 'target' END;

-- Default values of a component type; a component's value with the same
-- key overrides them.
CREATE TABLE component_type_values (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL,
    type_id    TEXT NOT NULL,
    position   INTEGER NOT NULL,
    key        TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('torque', 'limit', 'target')),
    label      TEXT NOT NULL,
    value_from REAL,
    value_to   REAL,
    unit       TEXT NOT NULL DEFAULT '',
    hint       TEXT NOT NULL DEFAULT '',
    CHECK (value_from IS NOT NULL OR value_to IS NOT NULL),
    CHECK (value_from IS NULL OR value_to IS NULL OR value_to >= value_from),
    FOREIGN KEY (owner_id, type_id) REFERENCES component_types (owner_id, id) ON DELETE CASCADE
) STRICT;

CREATE INDEX component_type_values_type_idx ON component_type_values (owner_id, type_id, position);

-- +goose Down
DROP TABLE component_type_values;
ALTER TABLE maintenance_values DROP COLUMN hint;
ALTER TABLE maintenance_values DROP COLUMN kind;
ALTER TABLE maintenance_values DROP COLUMN key;
ALTER TABLE plan_step_values DROP COLUMN hint;
ALTER TABLE plan_step_values DROP COLUMN kind;
ALTER TABLE plan_step_values DROP COLUMN key;
ALTER TABLE component_values DROP COLUMN hint;
ALTER TABLE component_values DROP COLUMN kind;
ALTER TABLE component_values DROP COLUMN key;
