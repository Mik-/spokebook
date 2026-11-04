-- +goose Up
-- Features of a bike such as 'carbon' or 'tubeless', separated by spaces.
-- They switch on the modules of the step catalog.
ALTER TABLE bikes ADD COLUMN features TEXT NOT NULL DEFAULT '';

-- A module step applies only to bikes with the feature module; replaces is
-- the ID of the catalog step it replaces on such bikes.
ALTER TABLE steps ADD COLUMN module TEXT NOT NULL DEFAULT '';
ALTER TABLE steps ADD COLUMN replaces TEXT NOT NULL DEFAULT '';

-- Copied with the step: its module and the IDs of the module hints that
-- applied, separated by spaces.
ALTER TABLE maintenance_steps ADD COLUMN module TEXT NOT NULL DEFAULT '';
ALTER TABLE maintenance_steps ADD COLUMN hints TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE maintenance_steps DROP COLUMN hints;
ALTER TABLE maintenance_steps DROP COLUMN module;
ALTER TABLE steps DROP COLUMN replaces;
ALTER TABLE steps DROP COLUMN module;
ALTER TABLE bikes DROP COLUMN features;
