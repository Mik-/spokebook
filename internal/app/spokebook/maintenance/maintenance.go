// Package maintenance starts maintenances from the step catalog and
// records their progress.
package maintenance

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Mik-/spokebook/internal/app/spokebook/catalog"
	"github.com/Mik-/spokebook/internal/app/spokebook/components"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/values"
)

// ErrCompleted means a completed maintenance can't be changed; reopen it
// first.
var ErrCompleted = errors.New("maintenance: completed")

// ErrStepDone means a step that is done can't be removed; untick it first.
var ErrStepDone = errors.New("maintenance: step done")

// ErrNoSteps means a maintenance would have no steps (the level has none
// and no on-demand steps were chosen) or no new steps were chosen to add.
var ErrNoSteps = errors.New("maintenance: no steps")

// Start creates a maintenance of a bike at the given date and odometer,
// which is recorded as a reading if new. It copies the catalog steps of the
// level (may be empty) and the steps extraIDs (e.g. on-demand ones), see
// copySteps.
func Start(ctx context.Context, st store.Store, ownerID, bikeID, level string, extraIDs []string, at time.Time, distanceM int64) (store.Maintenance, error) {
	m := store.Maintenance{
		ID: uuid.Must(uuid.NewV7()).String(), OwnerID: ownerID, BikeID: bikeID, Level: level,
		PerformedAt: at, DistanceM: distanceM, CreatedAt: time.Now().UTC(),
	}
	if level != "" && !slices.Contains(catalog.Levels, level) {
		return store.Maintenance{}, catalog.ErrInvalid
	}
	err := st.WithTx(ctx, func(s store.Stores) error {
		bike, err := s.Bikes().GetBike(ctx, ownerID, bikeID)
		if err != nil {
			return err
		}
		if !bike.Active() {
			return components.ErrRetired
		}
		applicable, steps, err := chooseSteps(ctx, s, ownerID, bike.Features, func(cs store.Step) bool {
			return (level != "" && slices.Contains(cs.Levels, level)) || slices.Contains(extraIDs, cs.ID)
		}, extraIDs)
		if err != nil {
			return err
		}
		if len(steps) == 0 {
			return ErrNoSteps
		}
		installed, err := installedComponents(ctx, s, ownerID, bikeID, at)
		if err != nil {
			return err
		}
		m.Steps = copySteps(steps, applicable, nil, installed, bike.Features, "")
		err = components.RecordOdometerTx(ctx, s, ownerID, bikeID, at, distanceM)
		if err != nil {
			return err
		}
		return s.Maintenances().CreateMaintenance(ctx, m)
	})
	return m, err
}

// AddSteps copies the catalog steps ids to an open maintenance (see
// copySteps), with the components installed at its date. Steps it already
// contains are skipped.
func AddSteps(ctx context.Context, st store.Store, ownerID, id string, ids []string) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		m, err := s.Maintenances().GetMaintenance(ctx, ownerID, id)
		if err != nil {
			return err
		}
		if !m.Open() {
			return ErrCompleted
		}
		bike, err := s.Bikes().GetBike(ctx, ownerID, m.BikeID)
		if err != nil {
			return err
		}
		applicable, steps, err := chooseSteps(ctx, s, ownerID, bike.Features, func(cs store.Step) bool {
			return slices.Contains(ids, cs.ID) && !contains(m, cs.ID)
		}, ids)
		if err != nil {
			return err
		}
		if len(steps) == 0 {
			return ErrNoSteps
		}
		installed, err := installedComponents(ctx, s, ownerID, m.BikeID, m.PerformedAt)
		if err != nil {
			return err
		}
		copies := copySteps(steps, applicable, m.Steps, installed, bike.Features, "")
		for i := range copies {
			copies[i].Added = copies[i].OriginID == ""
		}
		return s.Maintenances().AddSteps(ctx, ownerID, id, copies)
	})
}

// contains reports whether m has a copy of the catalog step id.
func contains(m store.Maintenance, id string) bool {
	return slices.ContainsFunc(m.Steps, func(ms store.MaintenanceStep) bool { return ms.CatalogStepID == id })
}

// AddFollowUp copies the catalog step followUpID to an open maintenance as
// follow-up of its step originID (with the steps that always follow it).
// The catalog step of the origin must lead to it on a finding (see
// catalog.FollowUps), else store.ErrNotFound. A follow-up the maintenance
// already contains is left as it is.
func AddFollowUp(ctx context.Context, st store.Store, ownerID, id, originID, followUpID string) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		m, err := s.Maintenances().GetMaintenance(ctx, ownerID, id)
		if err != nil {
			return err
		}
		if !m.Open() {
			return ErrCompleted
		}
		i := slices.IndexFunc(m.Steps, func(ms store.MaintenanceStep) bool { return ms.ID == originID })
		if i < 0 {
			return store.ErrNotFound
		}
		bike, err := s.Bikes().GetBike(ctx, ownerID, m.BikeID)
		if err != nil {
			return err
		}
		applicable, _, err := chooseSteps(ctx, s, ownerID, bike.Features, func(store.Step) bool { return false }, nil)
		if err != nil {
			return err
		}
		offered := offers(applicable, m.Steps[i])
		j := slices.IndexFunc(offered, func(cs store.Step) bool { return cs.ID == followUpID })
		if j < 0 {
			return store.ErrNotFound
		}
		if contains(m, followUpID) {
			return nil
		}
		installed, err := installedComponents(ctx, s, ownerID, m.BikeID, m.PerformedAt)
		if err != nil {
			return err
		}
		copies := copySteps(offered[j:j+1], applicable, m.Steps, installed, bike.Features, originID)
		return s.Maintenances().AddSteps(ctx, ownerID, id, copies)
	})
}

// offers returns the catalog steps the maintenance step ms leads to on a
// finding, among the applicable ones.
func offers(applicable []store.Step, ms store.MaintenanceStep) []store.Step {
	i := slices.IndexFunc(applicable, func(cs store.Step) bool { return cs.ID == ms.CatalogStepID })
	if i < 0 {
		return nil
	}
	return catalog.FollowUps(applicable, applicable[i], store.FollowUpFinding)
}

// Offers returns per step of an open maintenance (by ID) the catalog steps
// it leads to on a finding that the maintenance doesn't contain yet.
func Offers(ctx context.Context, s store.Stores, ownerID string, m store.Maintenance) (map[string][]store.Step, error) {
	if !m.Open() {
		return nil, nil
	}
	bike, err := s.Bikes().GetBike(ctx, ownerID, m.BikeID)
	if err != nil {
		return nil, err
	}
	applicable, _, err := chooseSteps(ctx, s, ownerID, bike.Features, func(store.Step) bool { return false }, nil)
	if err != nil {
		return nil, err
	}
	out := map[string][]store.Step{}
	for _, ms := range m.Steps {
		for _, cs := range offers(applicable, ms) {
			if !contains(m, cs.ID) {
				out[ms.ID] = append(out[ms.ID], cs)
			}
		}
	}
	return out, nil
}

// Removable reports whether a step of an open maintenance can be removed:
// it is a follow-up or was added after the start, and is not done.
func Removable(ms store.MaintenanceStep) bool { return !ms.Done && (ms.OriginID != "" || ms.Added) }

// Remove removes a step from an open maintenance (see Removable) with the
// undone follow-ups it brought along, recursively. Other steps are
// store.ErrNotFound, a step that is done ErrStepDone.
func Remove(ctx context.Context, st store.Store, ownerID, id, stepID string) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		m, err := s.Maintenances().GetMaintenance(ctx, ownerID, id)
		if err != nil {
			return err
		}
		if !m.Open() {
			return ErrCompleted
		}
		i := slices.IndexFunc(m.Steps, func(ms store.MaintenanceStep) bool { return ms.ID == stepID })
		switch {
		case i < 0 || (m.Steps[i].OriginID == "" && !m.Steps[i].Added):
			return store.ErrNotFound
		case m.Steps[i].Done:
			return ErrStepDone
		}
		gone := []string{stepID}
		for j := 0; j < len(gone); j++ {
			for _, ms := range m.Steps {
				if ms.OriginID == gone[j] && !ms.Done && !slices.Contains(gone, ms.ID) {
					gone = append(gone, ms.ID)
				}
			}
		}
		for _, sid := range gone {
			if err := s.Maintenances().DeleteMaintenanceStep(ctx, ownerID, sid); err != nil {
				return err
			}
		}
		return nil
	})
}

// Dismiss marks a follow-up left undone in a completed maintenance as no
// longer needed, so it is no open work anymore. Other steps are
// store.ErrNotFound.
func Dismiss(ctx context.Context, st store.Store, ownerID, id, stepID string) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		m, err := s.Maintenances().GetMaintenance(ctx, ownerID, id)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(m.Steps, func(ms store.MaintenanceStep) bool { return ms.ID == stepID })
		if m.Open() || i < 0 || m.Steps[i].OriginID == "" || m.Steps[i].Done {
			return store.ErrNotFound
		}
		return s.Maintenances().SetStepDismissed(ctx, ownerID, stepID, true)
	})
}

// SortSteps puts the steps of m into display order: by phase, assembly
// group and catalog position.
func SortSteps(m *store.Maintenance) {
	slices.SortStableFunc(m.Steps, func(a, b store.MaintenanceStep) int {
		return cmp.Or(catalog.CompareSections(a.Phase, a.Assembly, b.Phase, b.Assembly), cmp.Compare(a.Position, b.Position))
	})
}

// chooseSteps returns the catalog steps applicable to a bike with the
// features (see catalog.Applicable) and those of them for which keep is
// true, both in display order, or store.ErrNotFound if one of ids is not
// among them.
func chooseSteps(ctx context.Context, s store.Stores, ownerID string, features []string, keep func(store.Step) bool, ids []string) (applicable, chosen []store.Step, err error) {
	applicable, err = catalog.Sorted(ctx, s, ownerID)
	if err != nil {
		return nil, nil, err
	}
	applicable = catalog.Applicable(applicable, features)
	for _, id := range ids {
		if !slices.ContainsFunc(applicable, func(cs store.Step) bool { return cs.ID == id }) {
			return nil, nil, store.ErrNotFound
		}
	}
	chosen = slices.DeleteFunc(slices.Clone(applicable), func(cs store.Step) bool { return !keep(cs) })
	return applicable, chosen, nil
}

// mounted is a component installed on the bike with its effective values
// (see values.Effective).
type mounted struct {
	store.Component
	Position string
	Values   []store.Value
}

// installedComponents returns the components on the bike at the time,
// ordered by position (as in components.Positions), then name.
func installedComponents(ctx context.Context, s store.Stores, ownerID, bikeID string, at time.Time) ([]mounted, error) {
	all, err := s.Components().ListAllInstallations(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	var out []mounted
	defaults := map[string][]store.Value{}
	for _, inst := range components.InstalledOn(all, bikeID, at) {
		c, err := s.Components().GetComponent(ctx, ownerID, inst.ComponentID)
		if err != nil {
			return nil, err
		}
		if _, ok := defaults[c.Type]; !ok {
			if defaults[c.Type], err = s.Components().ListTypeValues(ctx, ownerID, c.Type); err != nil {
				return nil, err
			}
		}
		vs, err := s.Components().ListComponentValues(ctx, ownerID, c.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, mounted{Component: c, Position: inst.Position, Values: values.Effective(defaults[c.Type], vs)})
	}
	slices.SortStableFunc(out, func(a, b mounted) int {
		if c := positionIndex(a.Position) - positionIndex(b.Position); c != 0 {
			return c
		}
		return strings.Compare(name(a.Component), name(b.Component))
	})
	return out, nil
}

// positionIndex orders "" first, then as in components.Positions.
func positionIndex(p string) int { return slices.Index(components.Positions, p) + 1 }

func name(c store.Component) string { return strings.TrimSpace(c.Manufacturer + " " + c.Model) }

// copySteps copies the catalog steps (see copyStep) with originID, each
// followed by the steps that always follow it (recursively, with the copy
// that led to them as origin). Steps among existing or copied already are
// skipped.
func copySteps(steps, applicable []store.Step, existing []store.MaintenanceStep, installed []mounted, features []string, originID string) []store.MaintenanceStep {
	type queued struct {
		step   store.Step
		origin string
	}
	var out []store.MaintenanceStep
	have := func(id string) bool {
		in := func(ms store.MaintenanceStep) bool { return ms.CatalogStepID == id }
		return slices.ContainsFunc(existing, in) || slices.ContainsFunc(out, in)
	}
	queue := make([]queued, 0, len(steps))
	for _, cs := range steps {
		queue = append(queue, queued{cs, originID})
	}
	for len(queue) > 0 {
		q := queue[0]
		queue = queue[1:]
		if have(q.step.ID) {
			continue
		}
		ms := copyStep(q.step, installed, features)
		ms.OriginID = q.origin
		out = append(out, ms)
		for _, f := range catalog.FollowUps(applicable, q.step, store.FollowUpAlways) {
			queue = append(queue, queued{f, ms.ID})
		}
	}
	return out
}

// copyStep copies a catalog step: first its own values, then per component
// type (in the step's order) the values of the installed components whose
// kind is one of the step's value kinds (all kinds if it has none), and the
// module hints for a bike with the features.
func copyStep(cs store.Step, installed []mounted, features []string) store.MaintenanceStep {
	ms := store.MaintenanceStep{
		ID: uuid.Must(uuid.NewV7()).String(), Position: cs.Position, CatalogStepID: cs.ID, BuiltinID: cs.BuiltinID,
		Title: cs.Title, Instructions: cs.Instructions, Phase: cs.Phase, Assembly: cs.Assembly, Action: cs.Action,
		Module: cs.Module, Hints: catalog.HintsFor(cs, features),
	}
	for _, v := range cs.Values {
		ms.Values = append(ms.Values, store.MaintenanceValue{Value: newValue(v)})
	}
	for _, t := range cs.ComponentTypes {
		for _, c := range installed {
			if c.Type != t {
				continue
			}
			for _, v := range c.Values {
				if v.Kind == store.KindInfo || len(cs.ValueKinds) > 0 && !slices.Contains(cs.ValueKinds, v.Kind) {
					continue
				}
				ms.Values = append(ms.Values, store.MaintenanceValue{
					Value: newValue(v), ComponentID: c.ID, ComponentName: name(c.Component),
					ComponentType: c.Type, ComponentPosition: c.Position,
				})
			}
		}
	}
	return ms
}

func newValue(v store.Value) store.Value {
	v.ID = uuid.Must(uuid.NewV7()).String()
	return v
}

// StepResult is what was entered for a step.
type StepResult struct {
	Done bool
	Note string
}

// Progress is what was entered on an open maintenance. Steps and Measured
// are keyed by step and value ID; missing ones stay unchanged.
type Progress struct {
	Notes    string
	Steps    map[string]StepResult
	Measured map[string]*float64
}

// Save records the progress of an open maintenance and completes it if
// complete is set.
func Save(ctx context.Context, st store.Store, ownerID, id string, p Progress, complete bool) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		m, err := s.Maintenances().GetMaintenance(ctx, ownerID, id)
		if err != nil {
			return err
		}
		if !m.Open() {
			return ErrCompleted
		}
		if err := s.Maintenances().SetMaintenanceNotes(ctx, ownerID, id, strings.TrimSpace(p.Notes)); err != nil {
			return err
		}
		for _, step := range m.Steps {
			if r, ok := p.Steps[step.ID]; ok {
				if err := s.Maintenances().SetStepResult(ctx, ownerID, step.ID, r.Done, strings.TrimSpace(r.Note)); err != nil {
					return err
				}
			}
			for _, v := range step.Values {
				if x, ok := p.Measured[v.ID]; ok && v.Measurable() {
					if err := s.Maintenances().SetMeasured(ctx, ownerID, v.ID, x); err != nil {
						return err
					}
				}
			}
		}
		if !complete {
			return nil
		}
		now := time.Now().UTC()
		return s.Maintenances().SetMaintenanceCompleted(ctx, ownerID, id, &now)
	})
}

// ChangeDate changes date and odometer of an open maintenance and records
// the odometer as a reading if new. The copied steps and values stay as
// they are.
func ChangeDate(ctx context.Context, st store.Store, ownerID, id string, at time.Time, distanceM int64) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		m, err := s.Maintenances().GetMaintenance(ctx, ownerID, id)
		if err != nil {
			return err
		}
		if !m.Open() {
			return ErrCompleted
		}
		err = components.RecordOdometerTx(ctx, s, ownerID, m.BikeID, at, distanceM)
		if err != nil {
			return err
		}
		return s.Maintenances().SetMaintenanceDate(ctx, ownerID, id, at, distanceM)
	})
}

// Reopen makes a completed maintenance editable again.
func Reopen(ctx context.Context, st store.Store, ownerID, id string) error {
	return st.Maintenances().SetMaintenanceCompleted(ctx, ownerID, id, nil)
}
