-- +goose Up
-- A step added to a maintenance after its start (not a follow-up) can be
-- removed again while it is open and not done, as can follow-ups. For
-- existing maintenances it is unknown, so their steps stay.
ALTER TABLE maintenance_steps ADD COLUMN added INTEGER NOT NULL DEFAULT 0 CHECK (added IN (0, 1));

-- +goose Down
ALTER TABLE maintenance_steps DROP COLUMN added;
