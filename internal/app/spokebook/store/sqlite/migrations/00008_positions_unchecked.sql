-- +goose Up
-- Drop the fixed list of positions from the CHECK constraint; the app
-- validates positions, so new ones need no migration. SQLite can't alter a
-- CHECK, so the table is rebuilt. No other table references installations.
CREATE TABLE installations_new (
    id            TEXT PRIMARY KEY,
    owner_id      TEXT NOT NULL,
    component_id  TEXT NOT NULL,
    bike_id       TEXT NOT NULL,
    position      TEXT NOT NULL DEFAULT '',
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

INSERT INTO installations_new
    (id, owner_id, component_id, bike_id, position, installed_at, installed_m, removed_at, removed_m, created_at)
SELECT id, owner_id, component_id, bike_id, position, installed_at, installed_m, removed_at, removed_m, created_at
FROM installations;

DROP TABLE installations;
ALTER TABLE installations_new RENAME TO installations;

CREATE INDEX installations_component_idx ON installations (component_id, installed_at);
CREATE INDEX installations_bike_idx ON installations (bike_id);
CREATE UNIQUE INDEX installations_open_idx ON installations (component_id) WHERE removed_at IS NULL;

-- +goose Down
-- Fails on positions other than front/rear instead of silently losing them.
CREATE TABLE installations_old (
    id            TEXT PRIMARY KEY,
    owner_id      TEXT NOT NULL,
    component_id  TEXT NOT NULL,
    bike_id       TEXT NOT NULL,
    installed_at  TEXT NOT NULL,
    installed_m   INTEGER NOT NULL CHECK (installed_m >= 0),
    removed_at    TEXT,
    removed_m     INTEGER,
    created_at    TEXT NOT NULL,
    position      TEXT NOT NULL DEFAULT '' CHECK (position IN ('', 'front', 'rear')),
    CHECK ((removed_at IS NULL) = (removed_m IS NULL)),
    CHECK (removed_m IS NULL OR removed_m >= installed_m),
    FOREIGN KEY (component_id, owner_id) REFERENCES components (id, owner_id) ON DELETE CASCADE,
    FOREIGN KEY (bike_id, owner_id) REFERENCES bikes (id, owner_id) ON DELETE CASCADE
) STRICT;

INSERT INTO installations_old
    (id, owner_id, component_id, bike_id, installed_at, installed_m, removed_at, removed_m, created_at, position)
SELECT id, owner_id, component_id, bike_id, installed_at, installed_m, removed_at, removed_m, created_at, position
FROM installations;

DROP TABLE installations;
ALTER TABLE installations_old RENAME TO installations;

CREATE INDEX installations_component_idx ON installations (component_id, installed_at);
CREATE INDEX installations_bike_idx ON installations (bike_id);
CREATE UNIQUE INDEX installations_open_idx ON installations (component_id) WHERE removed_at IS NULL;
