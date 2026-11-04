-- +goose Up
-- Built-in steps have a fixed builtin_id such as 'chain_wear'; while title
-- and instructions are empty they are translated. Built-in steps are
-- hidden instead of deleted, so seeding them stays idempotent.
ALTER TABLE steps ADD COLUMN builtin_id TEXT NOT NULL DEFAULT '';
ALTER TABLE steps ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0 CHECK (hidden IN (0, 1));
CREATE UNIQUE INDEX steps_builtin_idx ON steps (owner_id, builtin_id) WHERE builtin_id != '';

-- Copied with the step, so that an untranslated copy can be translated.
ALTER TABLE maintenance_steps ADD COLUMN builtin_id TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE maintenance_steps DROP COLUMN builtin_id;
DROP INDEX steps_builtin_idx;
ALTER TABLE steps DROP COLUMN hidden;
ALTER TABLE steps DROP COLUMN builtin_id;
