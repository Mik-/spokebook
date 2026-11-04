package sqlite

import (
	"context"
	"database/sql"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite/db"
)

func nullFloat(f *float64) sql.NullFloat64 {
	if f == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *f, Valid: true}
}

func floatPtr(f sql.NullFloat64) *float64 {
	if !f.Valid {
		return nil
	}
	return &f.Float64
}

func toComponentValue(r db.ComponentValue) (store.Value, error) {
	return store.Value{
		ID: r.ID, OwnerID: r.OwnerID, Position: int(r.Position), Key: r.Key, Kind: r.Kind, Label: r.Label,
		From: floatPtr(r.ValueFrom), To: floatPtr(r.ValueTo), Unit: r.Unit, Hint: r.Hint,
	}, nil
}

func toTypeValue(r db.ComponentTypeValue) (store.Value, error) {
	return store.Value{
		ID: r.ID, OwnerID: r.OwnerID, Position: int(r.Position), Key: r.Key, Kind: r.Kind, Label: r.Label,
		From: floatPtr(r.ValueFrom), To: floatPtr(r.ValueTo), Unit: r.Unit, Hint: r.Hint,
	}, nil
}

func (s componentStore) ListComponentValues(ctx context.Context, ownerID, componentID string) ([]store.Value, error) {
	rows, err := s.q.ListComponentValues(ctx, db.ListComponentValuesParams{OwnerID: ownerID, ComponentID: componentID})
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, toComponentValue)
}

func (s componentStore) ReplaceComponentValues(ctx context.Context, ownerID, componentID string, vs []store.Value) error {
	if err := s.q.DeleteComponentValues(ctx, db.DeleteComponentValuesParams{OwnerID: ownerID, ComponentID: componentID}); err != nil {
		return mapErr(err)
	}
	for i, v := range vs {
		err := s.q.AddComponentValue(ctx, db.AddComponentValueParams{
			ID: v.ID, OwnerID: ownerID, ComponentID: componentID, Position: int64(i), Key: v.Key, Kind: v.Kind,
			Label: v.Label, ValueFrom: nullFloat(v.From), ValueTo: nullFloat(v.To), Unit: v.Unit, Hint: v.Hint,
		})
		if err != nil {
			return mapErr(err)
		}
	}
	return nil
}

func (s componentStore) ListTypeValues(ctx context.Context, ownerID, typeID string) ([]store.Value, error) {
	rows, err := s.q.ListTypeValues(ctx, db.ListTypeValuesParams{OwnerID: ownerID, TypeID: typeID})
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, toTypeValue)
}

func (s componentStore) ReplaceTypeValues(ctx context.Context, ownerID, typeID string, vs []store.Value) error {
	if err := s.q.DeleteTypeValues(ctx, db.DeleteTypeValuesParams{OwnerID: ownerID, TypeID: typeID}); err != nil {
		return mapErr(err)
	}
	for i, v := range vs {
		err := s.q.AddTypeValue(ctx, db.AddTypeValueParams{
			ID: v.ID, OwnerID: ownerID, TypeID: typeID, Position: int64(i), Key: v.Key, Kind: v.Kind,
			Label: v.Label, ValueFrom: nullFloat(v.From), ValueTo: nullFloat(v.To), Unit: v.Unit, Hint: v.Hint,
		})
		if err != nil {
			return mapErr(err)
		}
	}
	return nil
}

func toBikeValue(r db.BikeValue) (store.Value, error) {
	return store.Value{
		ID: r.ID, OwnerID: r.OwnerID, Position: int(r.Position), Key: r.Key, Kind: r.Kind, Label: r.Label,
		From: floatPtr(r.ValueFrom), To: floatPtr(r.ValueTo), Unit: r.Unit, Hint: r.Hint,
	}, nil
}

func (s bikeStore) ListBikeValues(ctx context.Context, ownerID, bikeID string) ([]store.Value, error) {
	rows, err := s.q.ListBikeValues(ctx, db.ListBikeValuesParams{OwnerID: ownerID, BikeID: bikeID})
	if err != nil {
		return nil, mapErr(err)
	}
	return convertAll(rows, toBikeValue)
}

func (s bikeStore) ReplaceBikeValues(ctx context.Context, ownerID, bikeID string, vs []store.Value) error {
	if err := s.q.DeleteBikeValues(ctx, db.DeleteBikeValuesParams{OwnerID: ownerID, BikeID: bikeID}); err != nil {
		return mapErr(err)
	}
	for i, v := range vs {
		err := s.q.AddBikeValue(ctx, db.AddBikeValueParams{
			ID: v.ID, OwnerID: ownerID, BikeID: bikeID, Position: int64(i), Key: v.Key, Kind: v.Kind,
			Label: v.Label, ValueFrom: nullFloat(v.From), ValueTo: nullFloat(v.To), Unit: v.Unit, Hint: v.Hint,
		})
		if err != nil {
			return mapErr(err)
		}
	}
	return nil
}

func (s bikeStore) ListBikeValueLabels(ctx context.Context, ownerID string) ([]string, error) {
	labels, err := s.q.ListBikeValueLabels(ctx, ownerID)
	return labels, mapErr(err)
}

func (s bikeStore) ListBikeValueUnits(ctx context.Context, ownerID string) ([]string, error) {
	units, err := s.q.ListBikeValueUnits(ctx, ownerID)
	return units, mapErr(err)
}

func (s componentStore) ListValueLabels(ctx context.Context, ownerID string) ([]string, error) {
	labels, err := s.q.ListValueLabels(ctx, ownerID)
	return labels, mapErr(err)
}

func (s componentStore) ListValueUnits(ctx context.Context, ownerID string) ([]string, error) {
	units, err := s.q.ListValueUnits(ctx, ownerID)
	return units, mapErr(err)
}
