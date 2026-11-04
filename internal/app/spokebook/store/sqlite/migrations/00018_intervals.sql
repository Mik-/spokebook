-- +goose Up
-- A step can have its own interval: it is due after so many months or
-- meters since it was last done, whatever comes first.
ALTER TABLE steps ADD COLUMN interval_months INTEGER CHECK (interval_months > 0);
ALTER TABLE steps ADD COLUMN interval_m INTEGER CHECK (interval_m > 0);

-- Tubeless sealant dries out within months, so it is checked monthly even
-- without a monthly check.
UPDATE steps SET interval_months = 1 WHERE builtin_id = 'tubeless_sealant';

-- The interval of a level as the owner set it. Without a row the built-in
-- default applies; a row without months and meters means no interval.
CREATE TABLE level_intervals (
    owner_id        TEXT NOT NULL,
    level           TEXT NOT NULL,
    interval_months INTEGER CHECK (interval_months > 0),
    interval_m      INTEGER CHECK (interval_m > 0),
    PRIMARY KEY (owner_id, level)
) STRICT;

-- +goose Down
DROP TABLE level_intervals;
ALTER TABLE steps DROP COLUMN interval_m;
ALTER TABLE steps DROP COLUMN interval_months;
