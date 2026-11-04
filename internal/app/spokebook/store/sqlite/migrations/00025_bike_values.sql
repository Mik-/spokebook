-- +goose Up
-- The value kind 'info' is only shown, never measured and never copied into
-- a maintenance (e.g. stack, reach, tire width). SQLite can't alter a CHECK,
-- so the value tables are rebuilt with the same columns. No other table
-- references them.
CREATE TABLE component_values_new (
    id           TEXT PRIMARY KEY,
    owner_id     TEXT NOT NULL,
    component_id TEXT NOT NULL,
    position     INTEGER NOT NULL,
    label        TEXT NOT NULL,
    value_from   REAL,
    value_to     REAL,
    unit         TEXT NOT NULL DEFAULT '',
    key          TEXT NOT NULL DEFAULT '',
    kind         TEXT NOT NULL DEFAULT 'target' CHECK (kind IN ('torque', 'limit', 'target', 'info')),
    hint         TEXT NOT NULL DEFAULT '',
    CHECK (value_from IS NOT NULL OR value_to IS NOT NULL),
    CHECK (value_from IS NULL OR value_to IS NULL OR value_to >= value_from),
    FOREIGN KEY (component_id, owner_id) REFERENCES components (id, owner_id) ON DELETE CASCADE
) STRICT;
INSERT INTO component_values_new (id, owner_id, component_id, position, label, value_from, value_to, unit, key, kind, hint)
SELECT id, owner_id, component_id, position, label, value_from, value_to, unit, key, kind, hint FROM component_values;
DROP TABLE component_values;
ALTER TABLE component_values_new RENAME TO component_values;
CREATE INDEX component_values_component_idx ON component_values (component_id, position);

CREATE TABLE component_type_values_new (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL,
    type_id    TEXT NOT NULL,
    position   INTEGER NOT NULL,
    key        TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('torque', 'limit', 'target', 'info')),
    label      TEXT NOT NULL,
    value_from REAL,
    value_to   REAL,
    unit       TEXT NOT NULL DEFAULT '',
    hint       TEXT NOT NULL DEFAULT '',
    CHECK (value_from IS NOT NULL OR value_to IS NOT NULL),
    CHECK (value_from IS NULL OR value_to IS NULL OR value_to >= value_from),
    FOREIGN KEY (owner_id, type_id) REFERENCES component_types (owner_id, id) ON DELETE CASCADE
) STRICT;
INSERT INTO component_type_values_new (id, owner_id, type_id, position, key, kind, label, value_from, value_to, unit, hint)
SELECT id, owner_id, type_id, position, key, kind, label, value_from, value_to, unit, hint FROM component_type_values;
DROP TABLE component_type_values;
ALTER TABLE component_type_values_new RENAME TO component_type_values;
CREATE INDEX component_type_values_type_idx ON component_type_values (owner_id, type_id, position);

CREATE TABLE step_values_new (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL,
    step_id    TEXT NOT NULL,
    position   INTEGER NOT NULL,
    key        TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('torque', 'limit', 'target', 'info')),
    label      TEXT NOT NULL,
    value_from REAL,
    value_to   REAL,
    unit       TEXT NOT NULL DEFAULT '',
    hint       TEXT NOT NULL DEFAULT '',
    CHECK (value_from IS NOT NULL OR value_to IS NOT NULL),
    CHECK (value_from IS NULL OR value_to IS NULL OR value_to >= value_from),
    FOREIGN KEY (step_id, owner_id) REFERENCES steps (id, owner_id) ON DELETE CASCADE
) STRICT;
INSERT INTO step_values_new (id, owner_id, step_id, position, key, kind, label, value_from, value_to, unit, hint)
SELECT id, owner_id, step_id, position, key, kind, label, value_from, value_to, unit, hint FROM step_values;
DROP TABLE step_values;
ALTER TABLE step_values_new RENAME TO step_values;
CREATE INDEX step_values_step_idx ON step_values (step_id, position);

CREATE TABLE maintenance_values_new (
    id                 TEXT PRIMARY KEY,
    owner_id           TEXT NOT NULL,
    step_id            TEXT NOT NULL,
    position           INTEGER NOT NULL,
    component_id       TEXT NOT NULL DEFAULT '',
    component_name     TEXT NOT NULL DEFAULT '',
    component_type     TEXT NOT NULL DEFAULT '',
    component_position TEXT NOT NULL DEFAULT '',
    label              TEXT NOT NULL,
    value_from         REAL,
    value_to           REAL,
    unit               TEXT NOT NULL DEFAULT '',
    measured           REAL,
    key                TEXT NOT NULL DEFAULT '',
    kind               TEXT NOT NULL DEFAULT 'target' CHECK (kind IN ('torque', 'limit', 'target', 'info')),
    hint               TEXT NOT NULL DEFAULT '',
    CHECK (value_from IS NOT NULL OR value_to IS NOT NULL),
    CHECK (value_from IS NULL OR value_to IS NULL OR value_to >= value_from),
    FOREIGN KEY (step_id, owner_id) REFERENCES maintenance_steps (id, owner_id) ON DELETE CASCADE
) STRICT;
INSERT INTO maintenance_values_new (id, owner_id, step_id, position, component_id, component_name, component_type,
    component_position, label, value_from, value_to, unit, measured, key, kind, hint)
SELECT id, owner_id, step_id, position, component_id, component_name, component_type,
    component_position, label, value_from, value_to, unit, measured, key, kind, hint FROM maintenance_values;
DROP TABLE maintenance_values;
ALTER TABLE maintenance_values_new RENAME TO maintenance_values;
CREATE INDEX maintenance_values_step_idx ON maintenance_values (step_id, position);

-- Measurements of a bike such as saddle height, stack or reach. They are
-- documentation only; the app gives them the kind 'info'.
CREATE TABLE bike_values (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL,
    bike_id    TEXT NOT NULL,
    position   INTEGER NOT NULL,
    key        TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('torque', 'limit', 'target', 'info')),
    label      TEXT NOT NULL,
    value_from REAL,
    value_to   REAL,
    unit       TEXT NOT NULL DEFAULT '',
    hint       TEXT NOT NULL DEFAULT '',
    CHECK (value_from IS NOT NULL OR value_to IS NOT NULL),
    CHECK (value_from IS NULL OR value_to IS NULL OR value_to >= value_from),
    FOREIGN KEY (bike_id, owner_id) REFERENCES bikes (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE INDEX bike_values_bike_idx ON bike_values (bike_id, position);

-- +goose Down
-- Fails on values of kind 'info' instead of silently losing them.
DROP TABLE bike_values;

CREATE TABLE component_values_old (
    id           TEXT PRIMARY KEY,
    owner_id     TEXT NOT NULL,
    component_id TEXT NOT NULL,
    position     INTEGER NOT NULL,
    label        TEXT NOT NULL,
    value_from   REAL,
    value_to     REAL,
    unit         TEXT NOT NULL DEFAULT '',
    key          TEXT NOT NULL DEFAULT '',
    kind         TEXT NOT NULL DEFAULT 'target' CHECK (kind IN ('torque', 'limit', 'target')),
    hint         TEXT NOT NULL DEFAULT '',
    CHECK (value_from IS NOT NULL OR value_to IS NOT NULL),
    CHECK (value_from IS NULL OR value_to IS NULL OR value_to >= value_from),
    FOREIGN KEY (component_id, owner_id) REFERENCES components (id, owner_id) ON DELETE CASCADE
) STRICT;
INSERT INTO component_values_old (id, owner_id, component_id, position, label, value_from, value_to, unit, key, kind, hint)
SELECT id, owner_id, component_id, position, label, value_from, value_to, unit, key, kind, hint FROM component_values;
DROP TABLE component_values;
ALTER TABLE component_values_old RENAME TO component_values;
CREATE INDEX component_values_component_idx ON component_values (component_id, position);

CREATE TABLE component_type_values_old (
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
INSERT INTO component_type_values_old (id, owner_id, type_id, position, key, kind, label, value_from, value_to, unit, hint)
SELECT id, owner_id, type_id, position, key, kind, label, value_from, value_to, unit, hint FROM component_type_values;
DROP TABLE component_type_values;
ALTER TABLE component_type_values_old RENAME TO component_type_values;
CREATE INDEX component_type_values_type_idx ON component_type_values (owner_id, type_id, position);

CREATE TABLE step_values_old (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL,
    step_id    TEXT NOT NULL,
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
    FOREIGN KEY (step_id, owner_id) REFERENCES steps (id, owner_id) ON DELETE CASCADE
) STRICT;
INSERT INTO step_values_old (id, owner_id, step_id, position, key, kind, label, value_from, value_to, unit, hint)
SELECT id, owner_id, step_id, position, key, kind, label, value_from, value_to, unit, hint FROM step_values;
DROP TABLE step_values;
ALTER TABLE step_values_old RENAME TO step_values;
CREATE INDEX step_values_step_idx ON step_values (step_id, position);

CREATE TABLE maintenance_values_old (
    id                 TEXT PRIMARY KEY,
    owner_id           TEXT NOT NULL,
    step_id            TEXT NOT NULL,
    position           INTEGER NOT NULL,
    component_id       TEXT NOT NULL DEFAULT '',
    component_name     TEXT NOT NULL DEFAULT '',
    component_type     TEXT NOT NULL DEFAULT '',
    component_position TEXT NOT NULL DEFAULT '',
    label              TEXT NOT NULL,
    value_from         REAL,
    value_to           REAL,
    unit               TEXT NOT NULL DEFAULT '',
    measured           REAL,
    key                TEXT NOT NULL DEFAULT '',
    kind               TEXT NOT NULL DEFAULT 'target' CHECK (kind IN ('torque', 'limit', 'target')),
    hint               TEXT NOT NULL DEFAULT '',
    CHECK (value_from IS NOT NULL OR value_to IS NOT NULL),
    CHECK (value_from IS NULL OR value_to IS NULL OR value_to >= value_from),
    FOREIGN KEY (step_id, owner_id) REFERENCES maintenance_steps (id, owner_id) ON DELETE CASCADE
) STRICT;
INSERT INTO maintenance_values_old (id, owner_id, step_id, position, component_id, component_name, component_type,
    component_position, label, value_from, value_to, unit, measured, key, kind, hint)
SELECT id, owner_id, step_id, position, component_id, component_name, component_type,
    component_position, label, value_from, value_to, unit, measured, key, kind, hint FROM maintenance_values;
DROP TABLE maintenance_values;
ALTER TABLE maintenance_values_old RENAME TO maintenance_values;
CREATE INDEX maintenance_values_step_idx ON maintenance_values (step_id, position);
