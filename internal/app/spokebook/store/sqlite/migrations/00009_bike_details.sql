-- +goose Up
-- Origin ('bought', 'bought_used', 'self_built' or '' if unknown), purchase
-- data and when the bike went into service at which odometer.
ALTER TABLE bikes ADD COLUMN origin TEXT NOT NULL DEFAULT '';
ALTER TABLE bikes ADD COLUMN purchased_at TEXT;
ALTER TABLE bikes ADD COLUMN dealer TEXT NOT NULL DEFAULT '';
ALTER TABLE bikes ADD COLUMN price_cents INTEGER CHECK (price_cents >= 0);
ALTER TABLE bikes ADD COLUMN currency TEXT NOT NULL DEFAULT '';
ALTER TABLE bikes ADD COLUMN in_service_at TEXT;
ALTER TABLE bikes ADD COLUMN in_service_m INTEGER CHECK (in_service_m >= 0);

-- +goose Down
ALTER TABLE bikes DROP COLUMN in_service_m;
ALTER TABLE bikes DROP COLUMN in_service_at;
ALTER TABLE bikes DROP COLUMN currency;
ALTER TABLE bikes DROP COLUMN price_cents;
ALTER TABLE bikes DROP COLUMN dealer;
ALTER TABLE bikes DROP COLUMN purchased_at;
ALTER TABLE bikes DROP COLUMN origin;
