-- +goose Up
-- The manufacturer's part number (MPN).
ALTER TABLE components ADD COLUMN part_number TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE components DROP COLUMN part_number;
