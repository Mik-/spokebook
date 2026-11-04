-- +goose Up
ALTER TABLE components ADD COLUMN url TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE components DROP COLUMN url;
