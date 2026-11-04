-- +goose Up
-- Follow-ups of a step: catalog steps it can lead to. A finding follow-up
-- is offered during a maintenance (worn pads -> replace pads), an always
-- follow-up comes along whenever the step is part of one (new chain ->
-- adjust the derailleur).
CREATE TABLE step_follow_ups (
    step_id      TEXT NOT NULL,
    owner_id     TEXT NOT NULL,
    follow_up_id TEXT NOT NULL,
    kind         TEXT NOT NULL CHECK (kind IN ('finding', 'always')),
    position     INTEGER NOT NULL,
    PRIMARY KEY (step_id, follow_up_id),
    FOREIGN KEY (step_id, owner_id) REFERENCES steps (id, owner_id) ON DELETE CASCADE,
    FOREIGN KEY (follow_up_id, owner_id) REFERENCES steps (id, owner_id) ON DELETE CASCADE
) STRICT;

CREATE INDEX step_follow_ups_owner_idx ON step_follow_ups (owner_id, position);

-- The follow-ups of the built-in steps (see catalog/builtin.go) for the
-- catalogs that exist already.
WITH pairs (step, follow_up, kind) AS (VALUES
    ('brake_pad_thickness', 'replace_brake_pads', 'finding'),
    ('chain_wear', 'replace_chain', 'finding'),
    ('cassette_chainrings', 'replace_cassette_chainring', 'finding'),
    ('inspect_tires', 'replace_tires', 'finding'),
    ('wheel_true_spokes', 'true_wheel', 'finding'),
    ('headset_play', 'adjust_headset', 'finding'),
    ('shift_cables', 'replace_cables', 'finding'),
    ('brake_function', 'adjust_brakes', 'finding'),
    ('brake_test', 'adjust_brakes', 'finding'),
    ('shift_test', 'adjust_derailleur', 'finding'),
    ('belt_wear', 'replace_belt', 'finding'),
    ('suspension_seals', 'suspension_service', 'finding'),
    ('suspension_play', 'suspension_service', 'finding'),
    ('hydraulic_pressure_point', 'hydraulic_bleed', 'finding'),
    ('hydraulic_leaks', 'hydraulic_bleed', 'finding'),
    ('tubeless_sealant', 'tubeless_top_up', 'finding'),
    ('replace_chain', 'adjust_derailleur', 'always'),
    ('replace_cassette_chainring', 'adjust_derailleur', 'always'),
    ('replace_cables', 'adjust_derailleur', 'always'),
    ('replace_brake_pads', 'adjust_brakes', 'always'),
    ('replace_belt', 'belt_tension', 'always'),
    ('adjust_headset', 'torque_cockpit', 'always')
)
INSERT INTO step_follow_ups (step_id, owner_id, follow_up_id, kind, position)
SELECT s.id, s.owner_id, f.id, pairs.kind, 0
FROM pairs
JOIN steps s ON s.builtin_id = pairs.step
JOIN steps f ON f.builtin_id = pairs.follow_up AND f.owner_id = s.owner_id;

-- A step added as follow-up remembers the step of the same maintenance
-- that led to it. A follow-up left undone stays open work of the bike
-- until a later maintenance does it or it is dismissed.
ALTER TABLE maintenance_steps ADD COLUMN origin_id TEXT NOT NULL DEFAULT '';
ALTER TABLE maintenance_steps ADD COLUMN dismissed INTEGER NOT NULL DEFAULT 0 CHECK (dismissed IN (0, 1));

-- +goose Down
ALTER TABLE maintenance_steps DROP COLUMN dismissed;
ALTER TABLE maintenance_steps DROP COLUMN origin_id;
DROP TABLE step_follow_ups;
