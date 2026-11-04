-- +goose Up
-- Maintenance plans with ordered steps. A step can name component types
-- whose values (from the bike's installed components) apply to it, and has
-- own values like component_values.
CREATE TABLE plans (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL
) STRICT;

CREATE INDEX plans_owner_idx ON plans (owner_id);
CREATE UNIQUE INDEX plans_id_owner_idx ON plans (id, owner_id);

-- position is not unique, so that two steps can swap places.
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

-- A table rather than a column, so a step can later have several types.
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
    CHECK (value_from IS NOT NULL OR value_to IS NOT NULL),
    CHECK (value_from IS NULL OR value_to IS NULL OR value_to >= value_from),
    FOREIGN KEY (step_id, owner_id) REFERENCES plan_steps (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE INDEX plan_step_values_step_idx ON plan_step_values (step_id, position);

-- +goose Down
DROP TABLE plan_step_values;
DROP TABLE plan_step_component_types;
DROP TABLE plan_steps;
DROP TABLE plans;
