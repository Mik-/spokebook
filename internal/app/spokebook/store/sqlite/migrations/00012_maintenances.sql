-- +goose Up
-- A performed maintenance of a bike. Plan name, steps and values are
-- copied at the start, so the history stays as it was when plans or
-- components change later. plan_id has no foreign key: the plan may be
-- deleted, the maintenance stays.
CREATE TABLE maintenances (
    id           TEXT PRIMARY KEY,
    owner_id     TEXT NOT NULL,
    bike_id      TEXT NOT NULL,
    plan_id      TEXT NOT NULL DEFAULT '',
    plan_name    TEXT NOT NULL,
    performed_at TEXT NOT NULL,
    distance_m   INTEGER NOT NULL CHECK (distance_m >= 0),
    notes        TEXT NOT NULL DEFAULT '',
    completed_at TEXT,
    created_at   TEXT NOT NULL,
    FOREIGN KEY (bike_id, owner_id) REFERENCES bikes (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE INDEX maintenances_bike_idx ON maintenances (bike_id, performed_at);
CREATE UNIQUE INDEX maintenances_id_owner_idx ON maintenances (id, owner_id);

CREATE TABLE maintenance_steps (
    id             TEXT PRIMARY KEY,
    owner_id       TEXT NOT NULL,
    maintenance_id TEXT NOT NULL,
    position       INTEGER NOT NULL,
    title          TEXT NOT NULL,
    instructions   TEXT NOT NULL DEFAULT '',
    done           INTEGER NOT NULL DEFAULT 0 CHECK (done IN (0, 1)),
    note           TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (maintenance_id, owner_id) REFERENCES maintenances (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE INDEX maintenance_steps_maintenance_idx ON maintenance_steps (maintenance_id, position);
CREATE UNIQUE INDEX maintenance_steps_id_owner_idx ON maintenance_steps (id, owner_id);

-- Values of a step: its own (component_id '') and those of the components
-- installed at the start, with the component's name, type and position
-- copied. measured is what was measured for a minimum or maximum.
CREATE TABLE maintenance_values (
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
    CHECK (value_from IS NOT NULL OR value_to IS NOT NULL),
    CHECK (value_from IS NULL OR value_to IS NULL OR value_to >= value_from),
    FOREIGN KEY (step_id, owner_id) REFERENCES maintenance_steps (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE INDEX maintenance_values_step_idx ON maintenance_values (step_id, position);

-- +goose Down
DROP TABLE maintenance_values;
DROP TABLE maintenance_steps;
DROP TABLE maintenances;
