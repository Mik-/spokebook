// Package catalog handles the owner's catalog of maintenance steps, ordered
// by phase, assembly group and position.
package catalog

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"github.com/spokebook/spokebook/internal/app/spokebook/bikes"
	"github.com/spokebook/spokebook/internal/app/spokebook/components"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
	"github.com/spokebook/spokebook/internal/app/spokebook/values"
)

// The fixed IDs of a step, in display order. They are translated via the
// message IDs "phase.<id>", "assembly.<id>", "step.action.<id>" and
// "level.<id>".
var (
	// Phases are the stages of a maintenance.
	Phases = []string{"intake", "cleaning", "inspection", "replacement", "adjustment", "final_check", "test_ride"}
	// Assemblies are the assembly groups, i.e. where on the bike a step is.
	Assemblies = []string{
		"frame_fork", "headset_cockpit", "wheels_tires", "brakes", "drivetrain",
		"bottom_bracket_cranks_pedals", "saddle_seatpost", "suspension", "electrics", "accessories",
	}
	// Actions say what a step does.
	Actions = []string{"check", "clean", "lubricate", "adjust", "measure", "replace", "tighten"}
	// Levels are the maintenance levels; a step without level is done on
	// demand.
	Levels = []string{"pre_ride", "monthly", "minor", "major"}
)

var (
	// ErrNoNeighbour means a step can't move up or down because it is
	// already first or last in its phase and assembly group.
	ErrNoNeighbour = errors.New("catalog: no step to swap with")
	// ErrInvalid means an unknown phase, assembly group, action, module,
	// level or value kind.
	ErrInvalid = errors.New("catalog: invalid choice")
)

// CompareSections orders by phase, then assembly group (none first).
// Unknown phases (as of maintenances from before the catalog) come first.
func CompareSections(phaseA, assemblyA, phaseB, assemblyB string) int {
	return cmp.Or(
		cmp.Compare(slices.Index(Phases, phaseA), slices.Index(Phases, phaseB)),
		cmp.Compare(slices.Index(Assemblies, assemblyA), slices.Index(Assemblies, assemblyB)),
	)
}

// Compare orders steps by phase, assembly group and position.
func Compare(a, b store.Step) int {
	return cmp.Or(CompareSections(a.Phase, a.Assembly, b.Phase, b.Assembly), cmp.Compare(a.Position, b.Position))
}

// Sorted returns the catalog without hidden steps in display order.
func Sorted(ctx context.Context, s store.Stores, ownerID string) ([]store.Step, error) {
	return sorted(ctx, s, ownerID, false)
}

// Hidden returns the hidden built-in steps in display order.
func Hidden(ctx context.Context, s store.Stores, ownerID string) ([]store.Step, error) {
	return sorted(ctx, s, ownerID, true)
}

func sorted(ctx context.Context, s store.Stores, ownerID string, hidden bool) ([]store.Step, error) {
	steps, err := s.Steps().ListSteps(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	steps = slices.DeleteFunc(steps, func(s store.Step) bool { return s.Hidden != hidden })
	slices.SortStableFunc(steps, Compare)
	return steps, nil
}

// AddStep adds a step at the end of its phase and assembly group.
func AddStep(ctx context.Context, st store.Store, s store.Step) error {
	if err := prepare(&s); err != nil {
		return err
	}
	return st.WithTx(ctx, func(tx store.Stores) error {
		if err := checkTypes(ctx, tx, s, nil); err != nil {
			return err
		}
		if err := checkFollowUps(ctx, tx, s); err != nil {
			return err
		}
		steps, err := tx.Steps().ListSteps(ctx, s.OwnerID)
		if err != nil {
			return err
		}
		s.Position = nextPosition(steps)
		return tx.Steps().CreateStep(ctx, s)
	})
}

// UpdateStep changes a step. Moved to another phase or assembly group, it
// goes to the end there.
func UpdateStep(ctx context.Context, st store.Store, s store.Step) error {
	if err := prepare(&s); err != nil {
		return err
	}
	return st.WithTx(ctx, func(tx store.Stores) error {
		old, err := tx.Steps().GetStep(ctx, s.OwnerID, s.ID)
		if err != nil {
			return err
		}
		if err := checkTypes(ctx, tx, s, old.ComponentTypes); err != nil {
			return err
		}
		if err := checkFollowUps(ctx, tx, s); err != nil {
			return err
		}
		s.Position = old.Position
		if CompareSections(s.Phase, s.Assembly, old.Phase, old.Assembly) != 0 {
			steps, err := tx.Steps().ListSteps(ctx, s.OwnerID)
			if err != nil {
				return err
			}
			s.Position = nextPosition(steps)
		}
		return tx.Steps().UpdateStep(ctx, s)
	})
}

// prepare checks the fixed IDs and the values of s, puts levels and value
// kinds into their display order without duplicates and drops repeated
// follow-ups.
func prepare(s *store.Step) error {
	if !slices.Contains(Phases, s.Phase) ||
		(s.Assembly != "" && !slices.Contains(Assemblies, s.Assembly)) ||
		(s.Module != "" && !slices.Contains(bikes.Features, s.Module)) ||
		(s.Action != "" && !slices.Contains(Actions, s.Action)) {
		return ErrInvalid
	}
	var err error
	if s.Levels, err = subset(Levels, s.Levels); err != nil {
		return err
	}
	if s.ValueKinds, err = subset(store.StepValueKinds, s.ValueKinds); err != nil {
		return err
	}
	if err := CheckInterval(s.Interval); err != nil {
		return err
	}
	var follow []store.FollowUp
	for _, f := range s.FollowUps {
		if f.StepID == "" || f.StepID == s.ID || !slices.Contains(store.FollowUpKinds, f.Kind) {
			return ErrInvalid
		}
		if !slices.ContainsFunc(follow, func(o store.FollowUp) bool { return o.StepID == f.StepID }) {
			follow = append(follow, f)
		}
	}
	s.FollowUps = follow
	s.Values, err = values.Prepare(s.OwnerID, s.Values)
	return err
}

// subset returns the entries of all that are in ids, in the order of all,
// or ErrInvalid if ids contains others.
func subset(all, ids []string) ([]string, error) {
	for _, id := range ids {
		if !slices.Contains(all, id) {
			return nil, ErrInvalid
		}
	}
	var out []string
	for _, id := range all {
		if slices.Contains(ids, id) {
			out = append(out, id)
		}
	}
	return out, nil
}

func nextPosition(steps []store.Step) int {
	next := 0
	for _, s := range steps {
		next = max(next, s.Position+1)
	}
	return next
}

// checkTypes returns components.ErrInvalidType unless every component type
// of s is active or was already set (kept), so hiding a type doesn't break
// editing steps that use it.
func checkTypes(ctx context.Context, tx store.Stores, s store.Step, kept []string) error {
	for _, t := range s.ComponentTypes {
		if slices.Contains(kept, t) {
			continue
		}
		if _, err := components.ActiveType(ctx, tx, s.OwnerID, t); err != nil {
			return err
		}
	}
	return nil
}

// checkFollowUps returns ErrInvalid unless every follow-up of s is a step
// of its owner.
func checkFollowUps(ctx context.Context, tx store.Stores, s store.Step) error {
	for _, f := range s.FollowUps {
		if _, err := tx.Steps().GetStep(ctx, s.OwnerID, f.StepID); errors.Is(err, store.ErrNotFound) {
			return ErrInvalid
		} else if err != nil {
			return err
		}
	}
	return nil
}

// FollowUps returns the follow-ups of s of the kind among the applicable
// steps of a bike (see Applicable), in order. A follow-up replaced by a
// module step of the bike becomes that step; one that doesn't apply is
// left out.
func FollowUps(applicable []store.Step, s store.Step, kind string) []store.Step {
	var out []store.Step
	for _, f := range s.FollowUps {
		if f.Kind != kind {
			continue
		}
		i := slices.IndexFunc(applicable, func(a store.Step) bool { return a.ID == f.StepID })
		if i < 0 {
			i = slices.IndexFunc(applicable, func(a store.Step) bool { return a.Replaces == f.StepID })
		}
		if i >= 0 && applicable[i].ID != s.ID && !slices.ContainsFunc(out, func(o store.Step) bool { return o.ID == applicable[i].ID }) {
			out = append(out, applicable[i])
		}
	}
	return out
}

// DeleteStep deletes a step, or hides a built-in one so that it isn't
// seeded again.
func DeleteStep(ctx context.Context, st store.Store, ownerID, id string) error {
	return st.WithTx(ctx, func(tx store.Stores) error {
		s, err := tx.Steps().GetStep(ctx, ownerID, id)
		if err != nil {
			return err
		}
		if s.BuiltinID != "" {
			return tx.Steps().HideStep(ctx, ownerID, id)
		}
		return tx.Steps().DeleteStep(ctx, ownerID, id)
	})
}

// RestoreStep shows a hidden built-in step again, at its old place.
func RestoreStep(ctx context.Context, st store.Store, ownerID, id string) error {
	return st.Steps().ShowStep(ctx, ownerID, id)
}

// MoveStep swaps a step with the previous (delta -1) or next (delta 1) one
// in its phase and assembly group.
func MoveStep(ctx context.Context, st store.Store, ownerID, id string, delta int) error {
	return st.WithTx(ctx, func(tx store.Stores) error {
		steps, err := Sorted(ctx, tx, ownerID)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(steps, func(o store.Step) bool { return o.ID == id })
		if i < 0 {
			return store.ErrNotFound
		}
		j := i + delta
		if j < 0 || j >= len(steps) ||
			CompareSections(steps[i].Phase, steps[i].Assembly, steps[j].Phase, steps[j].Assembly) != 0 {
			return ErrNoNeighbour
		}
		steps[i], steps[j] = steps[j], steps[i]
		return setPositions(ctx, tx, steps)
	})
}

// setPositions gives the steps the positions 0, 1, 2 ... in their order.
func setPositions(ctx context.Context, tx store.Stores, steps []store.Step) error {
	for i, s := range steps {
		if s.Position == i {
			continue
		}
		if err := tx.Steps().SetStepPosition(ctx, s.OwnerID, s.ID, i); err != nil {
			return err
		}
	}
	return nil
}
