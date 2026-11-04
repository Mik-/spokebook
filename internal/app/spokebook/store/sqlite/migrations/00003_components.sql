-- +goose Up
CREATE TABLE components (
    id             TEXT PRIMARY KEY,
    owner_id       TEXT NOT NULL,
    type           TEXT NOT NULL,
    manufacturer   TEXT NOT NULL DEFAULT '',
    model          TEXT NOT NULL DEFAULT '',
    purchased_at   TEXT,
    dealer         TEXT NOT NULL DEFAULT '',
    price_cents    INTEGER CHECK (price_cents >= 0),
    currency       TEXT NOT NULL DEFAULT '',
    max_distance_m INTEGER CHECK (max_distance_m > 0),
    replace_by     TEXT,
    notes          TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL
) STRICT;

CREATE INDEX components_owner_idx ON components (owner_id);
CREATE UNIQUE INDEX components_id_owner_idx ON components (id, owner_id);

-- A component is installed on a bike from installed_at until removed_at.
-- Distances are the bike's odometer at that time.
CREATE TABLE installations (
    id            TEXT PRIMARY KEY,
    owner_id      TEXT NOT NULL,
    component_id  TEXT NOT NULL,
    bike_id       TEXT NOT NULL,
    installed_at  TEXT NOT NULL,
    installed_m   INTEGER NOT NULL CHECK (installed_m >= 0),
    removed_at    TEXT,
    removed_m     INTEGER,
    created_at    TEXT NOT NULL,
    CHECK ((removed_at IS NULL) = (removed_m IS NULL)),
    CHECK (removed_m IS NULL OR removed_m >= installed_m),
    FOREIGN KEY (component_id, owner_id) REFERENCES components (id, owner_id) ON DELETE CASCADE,
    FOREIGN KEY (bike_id, owner_id) REFERENCES bikes (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE INDEX installations_component_idx ON installations (component_id, installed_at);
CREATE INDEX installations_bike_idx ON installations (bike_id);
-- At most one open installation per component.
CREATE UNIQUE INDEX installations_open_idx ON installations (component_id) WHERE removed_at IS NULL;

-- +goose Down
DROP TABLE installations;
DROP TABLE components;
