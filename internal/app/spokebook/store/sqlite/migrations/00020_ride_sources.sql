-- +goose Up
-- A source an owner connected to fetch rides from, e.g. Garmin Connect.
CREATE TABLE ride_sources (
    owner_id        TEXT NOT NULL,
    source          TEXT NOT NULL,
    -- Sealed with APP_SECRET_KEY (package secret), never in plain text.
    credentials     BLOB NOT NULL,
    -- Rides before this day are not fetched.
    since           TEXT NOT NULL,
    -- Start of the newest activity processed; NULL before the first.
    cursor          TEXT,
    last_run_at     TEXT,
    -- Why the last run stopped; '' if it didn't.
    last_error      TEXT NOT NULL,
    last_imported   INTEGER NOT NULL,
    last_unassigned INTEGER NOT NULL,
    -- Whether the last run hit its limit and left activities to fetch.
    more            INTEGER NOT NULL CHECK (more IN (0, 1)),
    created_at      TEXT NOT NULL,
    PRIMARY KEY (owner_id, source)
) STRICT;

-- The activities of a source already processed, also those skipped, so a
-- run doesn't fetch them again.
CREATE TABLE ride_source_items (
    owner_id   TEXT NOT NULL,
    source     TEXT NOT NULL,
    item_id    TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (owner_id, source, item_id)
) STRICT;

-- +goose Down
DROP TABLE ride_source_items;
DROP TABLE ride_sources;
