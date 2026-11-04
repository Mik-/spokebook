-- +goose Up
-- Rides imported from files or other sources. Only the distance counts; the
-- odometer of a bike is computed from its manual readings and its rides.
-- Discarded rides stay, so a new import doesn't bring them back.
CREATE TABLE rides (
    id           TEXT PRIMARY KEY,
    owner_id     TEXT NOT NULL,
    -- NULL while the ride is not assigned to a bike.
    bike_id      TEXT,
    source       TEXT NOT NULL,
    external_id  TEXT NOT NULL,
    -- The bike's key in the source, e.g. a bike profile; '' if unknown.
    gear_key     TEXT NOT NULL,
    -- Whether bike_id came from the gear mapping, so changing the mapping
    -- may move the ride.
    bike_by_gear INTEGER NOT NULL CHECK (bike_by_gear IN (0, 1)),
    started_at   TEXT NOT NULL,
    ended_at     TEXT NOT NULL,
    -- The local date of the start; the ride counts for this day.
    ridden_on    TEXT NOT NULL,
    distance_m   INTEGER NOT NULL CHECK (distance_m >= 0),
    name         TEXT NOT NULL,
    discarded    INTEGER NOT NULL CHECK (discarded IN (0, 1)),
    created_at   TEXT NOT NULL,
    UNIQUE (owner_id, source, external_id),
    FOREIGN KEY (bike_id, owner_id) REFERENCES bikes (id, owner_id)
) STRICT;

CREATE INDEX rides_bike_idx ON rides (bike_id, ridden_on);
CREATE INDEX rides_time_idx ON rides (owner_id, started_at);

-- Maps a bike's key in a source to a bike in the app.
CREATE TABLE ride_gears (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL,
    source     TEXT NOT NULL,
    gear_key   TEXT NOT NULL,
    -- The bike's name in the source, for display.
    name       TEXT NOT NULL,
    bike_id    TEXT,
    created_at TEXT NOT NULL,
    UNIQUE (owner_id, source, gear_key),
    FOREIGN KEY (bike_id, owner_id) REFERENCES bikes (id, owner_id)
) STRICT;

-- An installation whose odometer was not entered follows the bike's
-- computed odometer at its date, e.g. when rides are imported later.
ALTER TABLE installations ADD COLUMN installed_auto INTEGER NOT NULL DEFAULT 0 CHECK (installed_auto IN (0, 1));
ALTER TABLE installations ADD COLUMN removed_auto INTEGER NOT NULL DEFAULT 0 CHECK (removed_auto IN (0, 1));

-- +goose Down
ALTER TABLE installations DROP COLUMN removed_auto;
ALTER TABLE installations DROP COLUMN installed_auto;
DROP TABLE ride_gears;
DROP TABLE rides;
