-- +goose Up
-- Lets child tables reference (id, owner_id), so a row can never point to
-- another owner's bike.
CREATE UNIQUE INDEX bikes_id_owner_idx ON bikes (id, owner_id);

CREATE TABLE odometer_readings (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT NOT NULL,
    bike_id     TEXT NOT NULL,
    recorded_at TEXT NOT NULL,
    distance_m  INTEGER NOT NULL CHECK (distance_m >= 0),
    source      TEXT NOT NULL CHECK (source IN ('manual', 'import')),
    created_at  TEXT NOT NULL,
    FOREIGN KEY (bike_id, owner_id) REFERENCES bikes (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE INDEX odometer_readings_bike_idx ON odometer_readings (bike_id, recorded_at);

-- +goose Down
DROP TABLE odometer_readings;
DROP INDEX bikes_id_owner_idx;
