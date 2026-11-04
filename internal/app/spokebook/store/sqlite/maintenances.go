package sqlite

import (
	"context"
	"strings"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite/db"
)

type maintenanceStore struct{ q *db.Queries }

func toMaintenance(r db.Maintenance) (store.Maintenance, error) {
	m := store.Maintenance{
		ID: r.ID, OwnerID: r.OwnerID, BikeID: r.BikeID, Title: r.Title, Level: r.Level,
		DistanceM: r.DistanceM, Notes: r.Notes,
	}
	var err error
	if m.PerformedAt, err = parseTime(r.PerformedAt); err != nil {
		return store.Maintenance{}, err
	}
	if m.CompletedAt, err = timePtr(r.CompletedAt); err != nil {
		return store.Maintenance{}, err
	}
	if m.CreatedAt, err = parseTime(r.CreatedAt); err != nil {
		return store.Maintenance{}, err
	}
	return m, nil
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func (s maintenanceStore) CreateMaintenance(ctx context.Context, m store.Maintenance) error {
	err := s.q.CreateMaintenance(ctx, db.CreateMaintenanceParams{
		ID: m.ID, OwnerID: m.OwnerID, BikeID: m.BikeID, Title: m.Title, Level: m.Level,
		PerformedAt: formatTime(m.PerformedAt), DistanceM: m.DistanceM, Notes: m.Notes, CreatedAt: formatTime(m.CreatedAt),
	})
	if err != nil {
		return mapErr(err)
	}
	return s.AddSteps(ctx, m.OwnerID, m.ID, m.Steps)
}

func (s maintenanceStore) AddSteps(ctx context.Context, ownerID, maintenanceID string, steps []store.MaintenanceStep) error {
	for _, st := range steps {
		err := s.q.AddMaintenanceStep(ctx, db.AddMaintenanceStepParams{
			ID: st.ID, OwnerID: ownerID, MaintenanceID: maintenanceID, Position: int64(st.Position),
			CatalogStepID: st.CatalogStepID, BuiltinID: st.BuiltinID, Title: st.Title, Instructions: st.Instructions,
			Phase: st.Phase, Assembly: st.Assembly, Action: st.Action, Module: st.Module,
			Hints: strings.Join(st.Hints, " "), OriginID: st.OriginID, Added: boolInt(st.Added),
		})
		if err != nil {
			return mapErr(err)
		}
		for j, v := range st.Values {
			err := s.q.AddMaintenanceValue(ctx, db.AddMaintenanceValueParams{
				ID: v.ID, OwnerID: ownerID, StepID: st.ID, Position: int64(j),
				ComponentID: v.ComponentID, ComponentName: v.ComponentName, ComponentType: v.ComponentType,
				ComponentPosition: v.ComponentPosition, Key: v.Key, Kind: v.Kind, Label: v.Label,
				ValueFrom: nullFloat(v.From), ValueTo: nullFloat(v.To), Unit: v.Unit, Hint: v.Hint,
			})
			if err != nil {
				return mapErr(err)
			}
		}
	}
	return nil
}

func (s maintenanceStore) GetMaintenance(ctx context.Context, ownerID, id string) (store.Maintenance, error) {
	r, err := s.q.GetMaintenance(ctx, db.GetMaintenanceParams{OwnerID: ownerID, ID: id})
	if err != nil {
		return store.Maintenance{}, mapErr(err)
	}
	m, err := toMaintenance(r)
	if err != nil {
		return store.Maintenance{}, err
	}
	steps, err := s.q.ListMaintenanceSteps(ctx, db.ListMaintenanceStepsParams{OwnerID: ownerID, MaintenanceID: id})
	if err != nil {
		return store.Maintenance{}, mapErr(err)
	}
	byID := make(map[string]int, len(steps))
	for i, st := range steps {
		byID[st.ID] = i
		m.Steps = append(m.Steps, store.MaintenanceStep{
			ID: st.ID, Position: int(st.Position), CatalogStepID: st.CatalogStepID, BuiltinID: st.BuiltinID, Title: st.Title,
			Instructions: st.Instructions, Phase: st.Phase, Assembly: st.Assembly, Action: st.Action,
			Module: st.Module, Hints: strings.Fields(st.Hints), OriginID: st.OriginID, Added: st.Added != 0, Done: st.Done != 0,
			Dismissed: st.Dismissed != 0, Note: st.Note,
		})
	}
	vals, err := s.q.ListMaintenanceValues(ctx, db.ListMaintenanceValuesParams{OwnerID: ownerID, MaintenanceID: id})
	if err != nil {
		return store.Maintenance{}, mapErr(err)
	}
	for _, v := range vals {
		st := &m.Steps[byID[v.StepID]]
		st.Values = append(st.Values, store.MaintenanceValue{
			Value: store.Value{
				ID: v.ID, OwnerID: v.OwnerID, Position: int(v.Position), Key: v.Key, Kind: v.Kind, Label: v.Label,
				From: floatPtr(v.ValueFrom), To: floatPtr(v.ValueTo), Unit: v.Unit, Hint: v.Hint,
			},
			ComponentID: v.ComponentID, ComponentName: v.ComponentName, ComponentType: v.ComponentType,
			ComponentPosition: v.ComponentPosition, Measured: floatPtr(v.Measured),
		})
	}
	return m, nil
}

func (s maintenanceStore) ListMaintenances(ctx context.Context, ownerID, bikeID string) ([]store.Maintenance, error) {
	rows, err := s.q.ListMaintenances(ctx, db.ListMaintenancesParams{OwnerID: ownerID, BikeID: bikeID})
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, toMaintenance)
}

func (s maintenanceStore) SetMaintenanceNotes(ctx context.Context, ownerID, id, notes string) error {
	n, err := s.q.SetMaintenanceNotes(ctx, db.SetMaintenanceNotesParams{Notes: notes, OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s maintenanceStore) SetMaintenanceDate(ctx context.Context, ownerID, id string, at time.Time, distanceM int64) error {
	n, err := s.q.SetMaintenanceDate(ctx, db.SetMaintenanceDateParams{PerformedAt: formatTime(at), DistanceM: distanceM, OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s maintenanceStore) SetMaintenanceCompleted(ctx context.Context, ownerID, id string, at *time.Time) error {
	n, err := s.q.SetMaintenanceCompleted(ctx, db.SetMaintenanceCompletedParams{CompletedAt: nullTime(at), OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s maintenanceStore) SetStepResult(ctx context.Context, ownerID, stepID string, done bool, note string) error {
	n, err := s.q.SetMaintenanceStepResult(ctx, db.SetMaintenanceStepResultParams{Done: boolInt(done), Note: note, OwnerID: ownerID, ID: stepID})
	return rowsOrNotFound(n, err)
}

func (s maintenanceStore) SetStepDismissed(ctx context.Context, ownerID, stepID string, dismissed bool) error {
	n, err := s.q.SetMaintenanceStepDismissed(ctx, db.SetMaintenanceStepDismissedParams{Dismissed: boolInt(dismissed), OwnerID: ownerID, ID: stepID})
	return rowsOrNotFound(n, err)
}

func (s maintenanceStore) DeleteMaintenanceStep(ctx context.Context, ownerID, stepID string) error {
	n, err := s.q.DeleteMaintenanceStep(ctx, db.DeleteMaintenanceStepParams{OwnerID: ownerID, ID: stepID})
	return rowsOrNotFound(n, err)
}

func (s maintenanceStore) SetMeasured(ctx context.Context, ownerID, valueID string, measured *float64) error {
	n, err := s.q.SetMaintenanceMeasured(ctx, db.SetMaintenanceMeasuredParams{Measured: nullFloat(measured), OwnerID: ownerID, ID: valueID})
	return rowsOrNotFound(n, err)
}

func (s maintenanceStore) DeleteMaintenance(ctx context.Context, ownerID, id string) error {
	n, err := s.q.DeleteMaintenance(ctx, db.DeleteMaintenanceParams{OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}
