package catalog

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

// Builtin is a step every owner starts with (see docs/konzept-schritte.md).
// Title and instructions are translated via the message IDs
// "catalog.<id>.title" and "catalog.<id>.instructions". A module step has
// the bike feature Module and may replace the built-in step Replaces.
// IntervalMonths and IntervalKM are its own interval, if any. Findings and
// Always are the built-in IDs of its follow-ups of either kind.
type Builtin struct {
	ID       string
	Phase    string
	Assembly string
	Action   string
	Levels   []string
	Types    []string
	Kinds    []string
	Module   string
	Replaces string

	IntervalMonths int64
	IntervalKM     int64

	Findings, Always []string
}

// Shorthands for the levels of built-in steps.
var (
	vmkg = []string{"pre_ride", "monthly", "minor", "major"}
	mkg  = []string{"monthly", "minor", "major"}
	kg   = []string{"minor", "major"}
	g    = []string{"major"}
)

var (
	torque       = []string{store.KindTorque}
	limit        = []string{store.KindLimit}
	target       = []string{store.KindTarget}
	torqueTarget = []string{store.KindTorque, store.KindTarget}
)

// Builtins are the built-in steps in catalog order.
var Builtins = []Builtin{
	{ID: "record_odometer", Phase: "intake", Levels: kg},
	{ID: "brake_function", Phase: "intake", Assembly: "brakes", Action: "check", Levels: vmkg, Findings: []string{"adjust_brakes"}},
	{ID: "headset_play", Phase: "intake", Assembly: "headset_cockpit", Action: "check", Levels: mkg, Findings: []string{"adjust_headset"}},
	{ID: "tire_pressure_axles", Phase: "intake", Assembly: "wheels_tires", Action: "check", Levels: vmkg, Types: []string{"tire"}, Kinds: target},
	{ID: "noise_check", Phase: "intake", Action: "check", Levels: kg},

	{ID: "degrease_drivetrain", Phase: "cleaning", Assembly: "drivetrain", Action: "clean", Levels: kg},
	{ID: "wash_bike", Phase: "cleaning", Action: "clean", Levels: kg},
	{ID: "clean_braking_surfaces", Phase: "cleaning", Assembly: "brakes", Action: "clean", Levels: g},

	{ID: "inspect_frame_fork", Phase: "inspection", Assembly: "frame_fork", Action: "check", Levels: mkg},
	{ID: "derailleur_hanger", Phase: "inspection", Assembly: "frame_fork", Action: "check", Levels: g},
	{ID: "inspect_handlebar_stem", Phase: "inspection", Assembly: "headset_cockpit", Action: "check", Levels: mkg},
	{ID: "grips_tape", Phase: "inspection", Assembly: "headset_cockpit", Action: "check", Levels: kg},
	{ID: "inspect_tires", Phase: "inspection", Assembly: "wheels_tires", Action: "check", Levels: mkg, Findings: []string{"replace_tires"}},
	{ID: "wheel_true_spokes", Phase: "inspection", Assembly: "wheels_tires", Action: "check", Levels: kg, Findings: []string{"true_wheel"}},
	{ID: "hub_bearings", Phase: "inspection", Assembly: "wheels_tires", Action: "check", Levels: kg},
	{ID: "rim_wear", Phase: "inspection", Assembly: "wheels_tires", Action: "measure", Levels: g, Types: []string{"rim"}, Kinds: limit},
	{ID: "tube_valve", Phase: "inspection", Assembly: "wheels_tires", Action: "check", Levels: kg},
	{ID: "brake_pad_thickness", Phase: "inspection", Assembly: "brakes", Action: "measure", Levels: mkg, Types: []string{"brake_pads"}, Kinds: limit, Findings: []string{"replace_brake_pads"}},
	{ID: "rotor_thickness", Phase: "inspection", Assembly: "brakes", Action: "measure", Levels: g, Types: []string{"brake_rotor"}, Kinds: limit},
	{ID: "brake_cables_hoses", Phase: "inspection", Assembly: "brakes", Action: "check", Levels: kg},
	{ID: "chain_wear", Phase: "inspection", Assembly: "drivetrain", Action: "measure", Levels: mkg, Types: []string{"chain"}, Kinds: limit, Findings: []string{"replace_chain"}},
	{ID: "cassette_chainrings", Phase: "inspection", Assembly: "drivetrain", Action: "check", Levels: g, Findings: []string{"replace_cassette_chainring"}},
	{ID: "shift_cables", Phase: "inspection", Assembly: "drivetrain", Action: "check", Levels: kg, Findings: []string{"replace_cables"}},
	{ID: "derailleur_pulleys", Phase: "inspection", Assembly: "drivetrain", Action: "check", Levels: g},
	{ID: "bottom_bracket_play", Phase: "inspection", Assembly: "bottom_bracket_cranks_pedals", Action: "check", Levels: kg},
	{ID: "pedals", Phase: "inspection", Assembly: "bottom_bracket_cranks_pedals", Action: "check", Levels: kg},
	{ID: "saddle_seatpost", Phase: "inspection", Assembly: "saddle_seatpost", Action: "check", Levels: kg},
	{ID: "lights", Phase: "inspection", Assembly: "electrics", Action: "check", Levels: vmkg},
	{ID: "accessories", Phase: "inspection", Assembly: "accessories", Action: "check", Levels: kg},

	{ID: "replace_chain", Phase: "replacement", Assembly: "drivetrain", Action: "replace", Always: []string{"adjust_derailleur"}},
	{ID: "replace_cassette_chainring", Phase: "replacement", Assembly: "drivetrain", Action: "replace", Types: []string{"cassette", "chainring"}, Kinds: torque, Always: []string{"adjust_derailleur"}},
	{ID: "replace_brake_pads", Phase: "replacement", Assembly: "brakes", Action: "replace", Always: []string{"adjust_brakes"}},
	{ID: "replace_cables", Phase: "replacement", Assembly: "drivetrain", Action: "replace", Always: []string{"adjust_derailleur"}},
	{ID: "service_seatpost", Phase: "replacement", Assembly: "saddle_seatpost", Action: "lubricate", Levels: g, Types: []string{"seatpost_clamp"}, Kinds: torque},
	{ID: "replace_tires", Phase: "replacement", Assembly: "wheels_tires", Action: "replace"},

	{ID: "adjust_headset", Phase: "adjustment", Assembly: "headset_cockpit", Action: "adjust", Types: []string{"stem"}, Kinds: torque, Always: []string{"torque_cockpit"}},
	{ID: "true_wheel", Phase: "adjustment", Assembly: "wheels_tires", Action: "adjust"},
	{ID: "adjust_brakes", Phase: "adjustment", Assembly: "brakes", Action: "adjust", Levels: kg, Types: []string{"brake_caliper"}, Kinds: torque},
	{ID: "adjust_derailleur", Phase: "adjustment", Assembly: "drivetrain", Action: "adjust", Levels: kg},
	{ID: "adjust_front_derailleur", Phase: "adjustment", Assembly: "drivetrain", Action: "adjust", Levels: kg},
	{ID: "lubricate_chain", Phase: "adjustment", Assembly: "drivetrain", Action: "lubricate", Levels: mkg},
	{ID: "lubricate_pivots", Phase: "adjustment", Action: "lubricate", Levels: g},

	{ID: "torque_cockpit", Phase: "final_check", Assembly: "headset_cockpit", Action: "tighten", Levels: mkg, Types: []string{"stem", "handlebar", "shift_brake_lever", "shift_lever", "brake_lever"}, Kinds: torque},
	{ID: "torque_saddle", Phase: "final_check", Assembly: "saddle_seatpost", Action: "tighten", Levels: mkg, Types: []string{"seatpost_clamp", "seatpost", "suspension_seatpost", "saddle"}, Kinds: torque},
	{ID: "torque_cranks_pedals", Phase: "final_check", Assembly: "bottom_bracket_cranks_pedals", Action: "tighten", Levels: kg, Types: []string{"crankset", "chainring", "pedal"}, Kinds: torque},
	{ID: "torque_brakes", Phase: "final_check", Assembly: "brakes", Action: "tighten", Levels: kg, Types: []string{"brake_caliper", "brake_rotor"}, Kinds: torque},
	{ID: "torque_derailleurs", Phase: "final_check", Assembly: "drivetrain", Action: "tighten", Levels: kg, Types: []string{"derailleur", "front_derailleur"}, Kinds: torque},
	{ID: "axles_tire_pressure", Phase: "final_check", Assembly: "wheels_tires", Action: "tighten", Levels: kg, Types: []string{"axle", "tire"}, Kinds: torqueTarget},
	{ID: "torque_accessories", Phase: "final_check", Assembly: "accessories", Action: "tighten", Levels: g, Types: []string{"fender", "rack", "kickstand"}, Kinds: torque},

	{ID: "brake_test", Phase: "test_ride", Assembly: "brakes", Action: "check", Levels: kg, Findings: []string{"adjust_brakes"}},
	{ID: "shift_test", Phase: "test_ride", Assembly: "drivetrain", Action: "check", Levels: kg, Findings: []string{"adjust_derailleur"}},
	{ID: "ride_noises", Phase: "test_ride", Action: "check", Levels: kg},
	{ID: "finish", Phase: "test_ride", Levels: kg},

	// Module carbon.
	{ID: "carbon_stand", Phase: "intake", Module: "carbon", Levels: kg},
	{ID: "carbon_hotspots", Phase: "inspection", Assembly: "frame_fork", Action: "check", Module: "carbon", Levels: mkg},
	{ID: "carbon_knock_test", Phase: "inspection", Assembly: "frame_fork", Action: "check", Module: "carbon", Levels: g},
	{ID: "carbon_cable_rub", Phase: "inspection", Assembly: "frame_fork", Action: "check", Module: "carbon", Levels: kg},
	{ID: "carbon_crash_check", Phase: "inspection", Assembly: "frame_fork", Action: "check", Module: "carbon"},
	{ID: "carbon_min_insertion", Phase: "inspection", Assembly: "saddle_seatpost", Action: "check", Module: "carbon", Levels: kg},

	// Module tubeless.
	{ID: "tubeless_sealant", Phase: "inspection", Assembly: "wheels_tires", Action: "check", Module: "tubeless", Levels: mkg, Replaces: "tube_valve", IntervalMonths: 1, Findings: []string{"tubeless_top_up"}},
	{ID: "tubeless_sealant_leaks", Phase: "inspection", Assembly: "wheels_tires", Action: "check", Module: "tubeless", Levels: kg},
	{ID: "tubeless_after_co2", Phase: "inspection", Assembly: "wheels_tires", Action: "check", Module: "tubeless"},
	{ID: "tubeless_top_up", Phase: "replacement", Assembly: "wheels_tires", Module: "tubeless"},
	{ID: "tubeless_sealant_change", Phase: "replacement", Assembly: "wheels_tires", Action: "replace", Module: "tubeless", Levels: g},
	{ID: "tubeless_valve_core", Phase: "replacement", Assembly: "wheels_tires", Action: "clean", Module: "tubeless", Levels: g},

	// Module ebike.
	{ID: "ebike_battery", Phase: "inspection", Assembly: "electrics", Action: "check", Module: "ebike", Levels: mkg},
	{ID: "ebike_battery_mount", Phase: "inspection", Assembly: "electrics", Action: "check", Module: "ebike", Levels: kg},
	{ID: "ebike_wiring", Phase: "inspection", Assembly: "electrics", Action: "check", Module: "ebike", Levels: kg},
	{ID: "ebike_display", Phase: "inspection", Assembly: "electrics", Action: "check", Module: "ebike", Levels: kg},
	{ID: "ebike_speed_sensor", Phase: "inspection", Assembly: "electrics", Action: "check", Module: "ebike", Levels: kg},
	{ID: "ebike_diagnostics", Phase: "inspection", Assembly: "electrics", Action: "check", Module: "ebike", Levels: g},
	{ID: "ebike_charger", Phase: "inspection", Assembly: "electrics", Action: "check", Module: "ebike", Levels: g},
	{ID: "ebike_motor_mount", Phase: "final_check", Assembly: "electrics", Action: "tighten", Module: "ebike", Levels: g, Types: []string{"motor"}, Kinds: torque},
	{ID: "ebike_assist_test", Phase: "test_ride", Assembly: "electrics", Action: "check", Module: "ebike", Levels: kg},
	{ID: "ebike_battery_data", Phase: "test_ride", Assembly: "electrics", Action: "measure", Module: "ebike", Levels: g, Types: []string{"battery"}, Kinds: limit},

	// Module suspension.
	{ID: "suspension_clean", Phase: "cleaning", Assembly: "suspension", Action: "clean", Module: "suspension", Levels: mkg},
	{ID: "suspension_seals", Phase: "inspection", Assembly: "suspension", Action: "check", Module: "suspension", Levels: mkg, Findings: []string{"suspension_service"}},
	{ID: "suspension_play", Phase: "inspection", Assembly: "suspension", Action: "check", Module: "suspension", Levels: kg, Findings: []string{"suspension_service"}},
	{ID: "suspension_service", Phase: "replacement", Assembly: "suspension", Action: "replace", Module: "suspension"},
	{ID: "suspension_sag", Phase: "adjustment", Assembly: "suspension", Action: "adjust", Module: "suspension", Levels: mkg, Types: []string{"fork", "shock", "suspension_seatpost"}, Kinds: target},
	{ID: "torque_suspension", Phase: "final_check", Assembly: "suspension", Action: "tighten", Module: "suspension", Levels: kg, Types: []string{"fork", "shock", "suspension_seatpost"}, Kinds: torque},

	// Module hydraulic_brakes.
	{ID: "hydraulic_leaks", Phase: "inspection", Assembly: "brakes", Action: "check", Module: "hydraulic_brakes", Levels: kg, Replaces: "brake_cables_hoses", Findings: []string{"hydraulic_bleed"}},
	{ID: "hydraulic_pressure_point", Phase: "inspection", Assembly: "brakes", Action: "check", Module: "hydraulic_brakes", Levels: mkg, Findings: []string{"hydraulic_bleed"}},
	{ID: "hydraulic_bleed", Phase: "replacement", Assembly: "brakes", Action: "replace", Module: "hydraulic_brakes", IntervalMonths: 12},

	// Module hub_gear.
	{ID: "hub_gear_check", Phase: "inspection", Assembly: "drivetrain", Action: "check", Module: "hub_gear", Levels: kg},
	{ID: "hub_gear_oil", Phase: "replacement", Assembly: "drivetrain", Action: "replace", Module: "hub_gear", IntervalMonths: 12, IntervalKM: 5000},
	{ID: "hub_gear_adjust", Phase: "adjustment", Assembly: "drivetrain", Action: "adjust", Module: "hub_gear", Levels: kg, Replaces: "adjust_derailleur"},

	// Module belt.
	{ID: "belt_clean", Phase: "cleaning", Assembly: "drivetrain", Action: "clean", Module: "belt", Levels: kg, Replaces: "degrease_drivetrain"},
	{ID: "belt_wear", Phase: "inspection", Assembly: "drivetrain", Action: "check", Module: "belt", Levels: mkg, Replaces: "chain_wear", Findings: []string{"replace_belt"}},
	{ID: "belt_sprockets", Phase: "inspection", Assembly: "drivetrain", Action: "check", Module: "belt", Levels: g, Replaces: "cassette_chainrings"},
	{ID: "replace_belt", Phase: "replacement", Assembly: "drivetrain", Action: "replace", Module: "belt", Replaces: "replace_chain", Always: []string{"belt_tension"}},
	{ID: "belt_tension", Phase: "adjustment", Assembly: "drivetrain", Action: "adjust", Module: "belt", Levels: kg, Replaces: "lubricate_chain", Types: []string{"belt"}, Kinds: target},

	// Module electronic_shifting.
	{ID: "eshift_battery", Phase: "inspection", Assembly: "drivetrain", Action: "check", Module: "electronic_shifting", Levels: mkg, Replaces: "shift_cables", Types: []string{"shift_battery"}, Kinds: limit},
	{ID: "eshift_wiring", Phase: "inspection", Assembly: "drivetrain", Action: "check", Module: "electronic_shifting", Levels: kg},
	{ID: "eshift_firmware", Phase: "inspection", Assembly: "drivetrain", Action: "check", Module: "electronic_shifting", Levels: g},
}

// Hint is a note a module adds to steps: to those with the Action, or to
// the built-in step Step. It is translated via "hint.<id>".
type Hint struct {
	ID      string
	Feature string
	Action  string
	Step    string
}

// Hints are the module hints.
var Hints = []Hint{
	{ID: "carbon_torque", Feature: "carbon", Action: "tighten"},
	{ID: "tubeless_pressure", Feature: "tubeless", Step: "axles_tire_pressure"},
	{ID: "ebike_wash", Feature: "ebike", Step: "wash_bike"},
	{ID: "ebike_chain_wear", Feature: "ebike", Step: "chain_wear"},
	{ID: "suspension_wash", Feature: "suspension", Step: "wash_bike"},
	{ID: "hydraulic_pads", Feature: "hydraulic_brakes", Step: "replace_brake_pads"},
	{ID: "eshift_adjust", Feature: "electronic_shifting", Step: "adjust_derailleur"},
	{ID: "eshift_adjust", Feature: "electronic_shifting", Step: "adjust_front_derailleur"},
}

// Drop is a built-in step a module makes unnecessary without replacing it.
type Drop struct {
	Feature string
	Step    string
}

// Drops are the steps modules drop.
var Drops = []Drop{
	{Feature: "hub_gear", Step: "derailleur_hanger"},
	{Feature: "hub_gear", Step: "derailleur_pulleys"},
	{Feature: "hub_gear", Step: "adjust_front_derailleur"},
	{Feature: "hub_gear", Step: "torque_derailleurs"},
	{Feature: "electronic_shifting", Step: "replace_cables"},
}

// DroppedBy returns the features that drop the built-in step.
func DroppedBy(builtinID string) []string {
	var out []string
	for _, d := range Drops {
		if builtinID != "" && d.Step == builtinID {
			out = append(out, d.Feature)
		}
	}
	return out
}

func dropped(s store.Step, features []string) bool {
	return slices.ContainsFunc(DroppedBy(s.BuiltinID), func(f string) bool { return slices.Contains(features, f) })
}

// HintsFor returns the IDs of the hints that apply to s on a bike with the
// features.
func HintsFor(s store.Step, features []string) []string {
	var out []string
	for _, h := range Hints {
		if !slices.Contains(features, h.Feature) {
			continue
		}
		if (h.Action != "" && h.Action == s.Action) || (h.Step != "" && h.Step == s.BuiltinID) {
			out = append(out, h.ID)
		}
	}
	return out
}

// Applicable returns the steps for a bike with the features: module steps
// only of its features, and without the steps those replace or drop.
func Applicable(steps []store.Step, features []string) []store.Step {
	replaced := map[string]bool{}
	for _, s := range steps {
		if s.Module != "" && s.Replaces != "" && slices.Contains(features, s.Module) {
			replaced[s.Replaces] = true
		}
	}
	return slices.DeleteFunc(slices.Clone(steps), func(s store.Step) bool {
		return replaced[s.ID] || dropped(s, features) || (s.Module != "" && !slices.Contains(features, s.Module))
	})
}

// SeedSteps adds the built-in steps the owner doesn't have yet at the end
// of their sections, with their follow-ups. Hidden ones stay hidden.
func SeedSteps(ctx context.Context, st store.Store, ownerID string) error {
	now := time.Now().UTC()
	return st.WithTx(ctx, func(tx store.Stores) error {
		steps, err := tx.Steps().ListSteps(ctx, ownerID)
		if err != nil {
			return err
		}
		ids := make(map[string]string, len(steps)) // step IDs by built-in ID
		for _, s := range steps {
			if s.BuiltinID != "" {
				ids[s.BuiltinID] = s.ID
			}
		}
		next := nextPosition(steps)
		var created []store.Step
		for _, b := range Builtins {
			if ids[b.ID] != "" {
				continue
			}
			s := store.Step{
				ID: uuid.Must(uuid.NewV7()).String(), OwnerID: ownerID, BuiltinID: b.ID, Position: next,
				Phase: b.Phase, Assembly: b.Assembly, Action: b.Action, Module: b.Module, Replaces: ids[b.Replaces],
				Levels: b.Levels, ComponentTypes: b.Types, ValueKinds: b.Kinds, CreatedAt: now,
			}
			if b.IntervalMonths > 0 {
				s.Interval.Months = &b.IntervalMonths
			}
			if b.IntervalKM > 0 {
				m := b.IntervalKM * 1000
				s.Interval.DistanceM = &m
			}
			if err := tx.Steps().CreateStep(ctx, s); err != nil {
				return err
			}
			ids[b.ID] = s.ID
			created = append(created, s)
			next++
		}
		// Follow-ups once all IDs are known.
		for _, s := range created {
			b := Builtins[slices.IndexFunc(Builtins, func(b Builtin) bool { return b.ID == s.BuiltinID })]
			for _, f := range []struct {
				kind string
				ids  []string
			}{{store.FollowUpFinding, b.Findings}, {store.FollowUpAlways, b.Always}} {
				for _, id := range f.ids {
					if ids[id] != "" {
						s.FollowUps = append(s.FollowUps, store.FollowUp{StepID: ids[id], Kind: f.kind})
					}
				}
			}
			if len(s.FollowUps) == 0 {
				continue
			}
			if err := tx.Steps().UpdateStep(ctx, s); err != nil {
				return err
			}
		}
		return nil
	})
}
