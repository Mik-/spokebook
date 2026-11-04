-- +goose Up
-- One catalog of steps per owner replaces the plans; existing plans and
-- their steps are dropped. Steps are ordered by phase, assembly group and
-- position (gaps allowed; not unique, so that two steps can swap places).
-- Phase, assembly, action and levels are fixed IDs checked in Go.
DROP TABLE plan_step_values;
DROP TABLE plan_step_component_types;
DROP TABLE plan_steps;
DROP TABLE plans;

CREATE TABLE steps (
    id           TEXT PRIMARY KEY,
    owner_id     TEXT NOT NULL,
    position     INTEGER NOT NULL,
    title        TEXT NOT NULL,
    instructions TEXT NOT NULL DEFAULT '',
    phase        TEXT NOT NULL,
    assembly     TEXT NOT NULL DEFAULT '',
    action       TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL
) STRICT;

CREATE INDEX steps_owner_idx ON steps (owner_id, position);
CREATE UNIQUE INDEX steps_id_owner_idx ON steps (id, owner_id);

-- The levels a step belongs to; a step without levels is done on demand.
CREATE TABLE step_levels (
    step_id  TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    level    TEXT NOT NULL,
    PRIMARY KEY (step_id, level),
    FOREIGN KEY (step_id, owner_id) REFERENCES steps (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE TABLE step_component_types (
    step_id        TEXT NOT NULL,
    owner_id       TEXT NOT NULL,
    component_type TEXT NOT NULL,
    PRIMARY KEY (step_id, component_type),
    FOREIGN KEY (step_id, owner_id) REFERENCES steps (id, owner_id) ON DELETE CASCADE
) STRICT;

-- The kinds of the component values that apply to a step.
CREATE TABLE step_value_kinds (
    step_id  TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    kind     TEXT NOT NULL CHECK (kind IN ('torque', 'limit', 'target')),
    PRIMARY KEY (step_id, kind),
    FOREIGN KEY (step_id, owner_id) REFERENCES steps (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE TABLE step_values (
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

CREATE INDEX step_values_step_idx ON step_values (step_id, position);

-- A maintenance follows a level (empty for one of on-demand steps only);
-- title keeps the plan name of maintenances from before the catalog.
ALTER TABLE maintenances RENAME COLUMN plan_name TO title;
ALTER TABLE maintenances DROP COLUMN plan_id;
ALTER TABLE maintenances ADD COLUMN level TEXT NOT NULL DEFAULT '';

-- Copied from the catalog step; position is the step's catalog position.
ALTER TABLE maintenance_steps ADD COLUMN catalog_step_id TEXT NOT NULL DEFAULT '';
ALTER TABLE maintenance_steps ADD COLUMN phase TEXT NOT NULL DEFAULT '';
ALTER TABLE maintenance_steps ADD COLUMN assembly TEXT NOT NULL DEFAULT '';
ALTER TABLE maintenance_steps ADD COLUMN action TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE maintenance_steps DROP COLUMN action;
ALTER TABLE maintenance_steps DROP COLUMN assembly;
ALTER TABLE maintenance_steps DROP COLUMN phase;
ALTER TABLE maintenance_steps DROP COLUMN catalog_step_id;
ALTER TABLE maintenances DROP COLUMN level;
ALTER TABLE maintenances ADD COLUMN plan_id TEXT NOT NULL DEFAULT '';
ALTER TABLE maintenances RENAME COLUMN title TO plan_name;
DROP TABLE step_values;
DROP TABLE step_value_kinds;
DROP TABLE step_component_types;
DROP TABLE step_levels;
DROP TABLE steps;

CREATE TABLE plans (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL
) STRICT;

CREATE INDEX plans_owner_idx ON plans (owner_id);
CREATE UNIQUE INDEX plans_id_owner_idx ON plans (id, owner_id);

CREATE TABLE plan_steps (
    id           TEXT PRIMARY KEY,
    owner_id     TEXT NOT NULL,
    plan_id      TEXT NOT NULL,
    position     INTEGER NOT NULL,
    title        TEXT NOT NULL,
    instructions TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    FOREIGN KEY (plan_id, owner_id) REFERENCES plans (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE INDEX plan_steps_plan_idx ON plan_steps (plan_id, position);
CREATE UNIQUE INDEX plan_steps_id_owner_idx ON plan_steps (id, owner_id);

CREATE TABLE plan_step_component_types (
    step_id        TEXT NOT NULL,
    owner_id       TEXT NOT NULL,
    component_type TEXT NOT NULL,
    PRIMARY KEY (step_id, component_type),
    FOREIGN KEY (step_id, owner_id) REFERENCES plan_steps (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE TABLE plan_step_values (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL,
    step_id    TEXT NOT NULL,
    position   INTEGER NOT NULL,
    label      TEXT NOT NULL,
    value_from REAL,
    value_to   REAL,
    unit       TEXT NOT NULL DEFAULT '',
    key        TEXT NOT NULL DEFAULT '',
    kind       TEXT NOT NULL DEFAULT 'target' CHECK (kind IN ('torque', 'limit', 'target')),
    hint       TEXT NOT NULL DEFAULT '',
    CHECK (value_from IS NOT NULL OR value_to IS NOT NULL),
    CHECK (value_from IS NULL OR value_to IS NULL OR value_to >= value_from),
    FOREIGN KEY (step_id, owner_id) REFERENCES plan_steps (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE INDEX plan_step_values_step_idx ON plan_step_values (step_id, position);
