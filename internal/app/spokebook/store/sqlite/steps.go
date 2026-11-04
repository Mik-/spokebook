package sqlite

import (
	"context"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite/db"
)

type stepStore struct{ q *db.Queries }

func toStep(r db.Step) (store.Step, error) {
	created, err := parseTime(r.CreatedAt)
	if err != nil {
		return store.Step{}, err
	}
	return store.Step{
		ID: r.ID, OwnerID: r.OwnerID, BuiltinID: r.BuiltinID, Hidden: r.Hidden != 0, Position: int(r.Position),
		Title: r.Title, Instructions: r.Instructions, Phase: r.Phase, Assembly: r.Assembly, Action: r.Action,
		Module: r.Module, Replaces: r.Replaces, CreatedAt: created,
		Interval: store.Interval{Months: intPtr(r.IntervalMonths), DistanceM: intPtr(r.IntervalM)},
	}, nil
}

func (s stepStore) CreateStep(ctx context.Context, st store.Step) error {
	err := s.q.CreateStep(ctx, db.CreateStepParams{
		ID: st.ID, OwnerID: st.OwnerID, BuiltinID: st.BuiltinID, Position: int64(st.Position), Title: st.Title,
		Instructions: st.Instructions, Phase: st.Phase, Assembly: st.Assembly, Action: st.Action,
		Module: st.Module, Replaces: st.Replaces, IntervalMonths: nullInt(st.Interval.Months),
		IntervalM: nullInt(st.Interval.DistanceM), CreatedAt: formatTime(st.CreatedAt),
	})
	if err != nil {
		return mapErr(err)
	}
	return s.replaceStepDetails(ctx, st)
}

func (s stepStore) UpdateStep(ctx context.Context, st store.Step) error {
	n, err := s.q.UpdateStep(ctx, db.UpdateStepParams{
		Title: st.Title, Instructions: st.Instructions, Phase: st.Phase, Assembly: st.Assembly, Action: st.Action,
		Module: st.Module, Position: int64(st.Position), IntervalMonths: nullInt(st.Interval.Months),
		IntervalM: nullInt(st.Interval.DistanceM), OwnerID: st.OwnerID, ID: st.ID,
	})
	if err := rowsOrNotFound(n, err); err != nil {
		return err
	}
	return s.replaceStepDetails(ctx, st)
}

// replaceStepDetails replaces the levels, component types, value kinds,
// follow-ups and values of a step.
func (s stepStore) replaceStepDetails(ctx context.Context, st store.Step) error {
	if err := s.q.DeleteStepLevels(ctx, db.DeleteStepLevelsParams{OwnerID: st.OwnerID, StepID: st.ID}); err != nil {
		return mapErr(err)
	}
	for _, l := range st.Levels {
		if err := s.q.AddStepLevel(ctx, db.AddStepLevelParams{StepID: st.ID, OwnerID: st.OwnerID, Level: l}); err != nil {
			return mapErr(err)
		}
	}
	if err := s.q.DeleteStepComponentTypes(ctx, db.DeleteStepComponentTypesParams{OwnerID: st.OwnerID, StepID: st.ID}); err != nil {
		return mapErr(err)
	}
	for _, t := range st.ComponentTypes {
		err := s.q.AddStepComponentType(ctx, db.AddStepComponentTypeParams{StepID: st.ID, OwnerID: st.OwnerID, ComponentType: t})
		if err != nil {
			return mapErr(err)
		}
	}
	if err := s.q.DeleteStepValueKinds(ctx, db.DeleteStepValueKindsParams{OwnerID: st.OwnerID, StepID: st.ID}); err != nil {
		return mapErr(err)
	}
	for _, k := range st.ValueKinds {
		if err := s.q.AddStepValueKind(ctx, db.AddStepValueKindParams{StepID: st.ID, OwnerID: st.OwnerID, Kind: k}); err != nil {
			return mapErr(err)
		}
	}
	if err := s.q.DeleteStepFollowUps(ctx, db.DeleteStepFollowUpsParams{OwnerID: st.OwnerID, StepID: st.ID}); err != nil {
		return mapErr(err)
	}
	for i, f := range st.FollowUps {
		err := s.q.AddStepFollowUp(ctx, db.AddStepFollowUpParams{
			StepID: st.ID, OwnerID: st.OwnerID, FollowUpID: f.StepID, Kind: f.Kind, Position: int64(i),
		})
		if err != nil {
			return mapErr(err)
		}
	}
	if err := s.q.DeleteStepValues(ctx, db.DeleteStepValuesParams{OwnerID: st.OwnerID, StepID: st.ID}); err != nil {
		return mapErr(err)
	}
	for i, v := range st.Values {
		err := s.q.AddStepValue(ctx, db.AddStepValueParams{
			ID: v.ID, OwnerID: st.OwnerID, StepID: st.ID, Position: int64(i), Key: v.Key, Kind: v.Kind,
			Label: v.Label, ValueFrom: nullFloat(v.From), ValueTo: nullFloat(v.To), Unit: v.Unit, Hint: v.Hint,
		})
		if err != nil {
			return mapErr(err)
		}
	}
	return nil
}

func (s stepStore) GetStep(ctx context.Context, ownerID, id string) (store.Step, error) {
	steps, err := s.ListSteps(ctx, ownerID)
	if err != nil {
		return store.Step{}, err
	}
	for _, st := range steps {
		if st.ID == id {
			return st, nil
		}
	}
	return store.Step{}, store.ErrNotFound
}

func (s stepStore) ListSteps(ctx context.Context, ownerID string) ([]store.Step, error) {
	rows, err := s.q.ListSteps(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	steps, err := convertAll(rows, toStep)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*store.Step, len(steps))
	for i := range steps {
		byID[steps[i].ID] = &steps[i]
	}
	levels, err := s.q.ListStepLevels(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	for _, l := range levels {
		byID[l.StepID].Levels = append(byID[l.StepID].Levels, l.Level)
	}
	types, err := s.q.ListStepComponentTypes(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	for _, t := range types {
		byID[t.StepID].ComponentTypes = append(byID[t.StepID].ComponentTypes, t.ComponentType)
	}
	kinds, err := s.q.ListStepValueKinds(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	for _, k := range kinds {
		byID[k.StepID].ValueKinds = append(byID[k.StepID].ValueKinds, k.Kind)
	}
	follows, err := s.q.ListStepFollowUps(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	for _, f := range follows {
		byID[f.StepID].FollowUps = append(byID[f.StepID].FollowUps, store.FollowUp{StepID: f.FollowUpID, Kind: f.Kind})
	}
	vals, err := s.q.ListStepValues(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	for _, v := range vals {
		byID[v.StepID].Values = append(byID[v.StepID].Values, store.Value{
			ID: v.ID, OwnerID: v.OwnerID, Position: int(v.Position), Key: v.Key, Kind: v.Kind, Label: v.Label,
			From: floatPtr(v.ValueFrom), To: floatPtr(v.ValueTo), Unit: v.Unit, Hint: v.Hint,
		})
	}
	return steps, nil
}

func (s stepStore) SetStepPosition(ctx context.Context, ownerID, id string, position int) error {
	n, err := s.q.SetStepPosition(ctx, db.SetStepPositionParams{Position: int64(position), OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s stepStore) HideStep(ctx context.Context, ownerID, id string) error {
	n, err := s.q.HideStep(ctx, db.HideStepParams{OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s stepStore) ShowStep(ctx context.Context, ownerID, id string) error {
	n, err := s.q.ShowStep(ctx, db.ShowStepParams{OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s stepStore) DeleteStep(ctx context.Context, ownerID, id string) error {
	n, err := s.q.DeleteStep(ctx, db.DeleteStepParams{OwnerID: ownerID, ID: id})
	return rowsOrNotFound(n, err)
}

func (s stepStore) ListLevelIntervals(ctx context.Context, ownerID string) ([]store.LevelInterval, error) {
	rows, err := s.q.ListLevelIntervals(ctx, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]store.LevelInterval, 0, len(rows))
	for _, r := range rows {
		out = append(out, store.LevelInterval{
			Level: r.Level, Interval: store.Interval{Months: intPtr(r.IntervalMonths), DistanceM: intPtr(r.IntervalM)},
		})
	}
	return out, nil
}

func (s stepStore) SetLevelInterval(ctx context.Context, ownerID string, li store.LevelInterval) error {
	return mapErr(s.q.SetLevelInterval(ctx, db.SetLevelIntervalParams{
		OwnerID: ownerID, Level: li.Level, IntervalMonths: nullInt(li.Months), IntervalM: nullInt(li.DistanceM),
	}))
}
