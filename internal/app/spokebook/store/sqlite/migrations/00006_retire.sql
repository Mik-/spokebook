-- +goose Up
-- Retired bikes and components keep their history but are no longer in use.
ALTER TABLE bikes ADD COLUMN retired_at TEXT;
ALTER TABLE bikes ADD COLUMN retire_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE components ADD COLUMN retired_at TEXT;
ALTER TABLE components ADD COLUMN retire_reason TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE components DROP COLUMN retire_reason;
ALTER TABLE components DROP COLUMN retired_at;
ALTER TABLE bikes DROP COLUMN retire_reason;
ALTER TABLE bikes DROP COLUMN retired_at;
