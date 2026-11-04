package components

import (
	"context"
	"errors"
	"slices"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/values"
)

var (
	// ErrIncompleteRemoval means a removal distance without a date.
	ErrIncompleteRemoval = errors.New("components: removal distance without date")
	// ErrBikeHasInstallations means a bike can't be deleted because
	// components were installed on it; their mileage would be lost.
	ErrBikeHasInstallations = errors.New("components: bike has installations")
)

// UpdateInstallation changes an installation after checking that removal
// follows installation and that it doesn't overlap others of the component.
// With InstalledAuto or RemovedAuto, that distance is the bike's computed
// odometer at its date.
func UpdateInstallation(ctx context.Context, st store.Store, i store.Installation) error {
	if i.Position != "" && !slices.Contains(Positions, i.Position) {
		return ErrInvalidPosition
	}
	if i.RemovedAt == nil && (i.RemovedM != nil || i.RemovedAuto) || i.RemovedAt != nil && i.RemovedM == nil && !i.RemovedAuto {
		return ErrIncompleteRemoval
	}
	return st.WithTx(ctx, func(s store.Stores) error {
		if i.InstalledAuto {
			m, err := distanceAt(ctx, s, i.OwnerID, i.BikeID, i.InstalledAt, nil)
			if err != nil {
				return err
			}
			i.InstalledM = m
		}
		if i.RemovedAuto {
			m, err := distanceAt(ctx, s, i.OwnerID, i.BikeID, *i.RemovedAt, nil)
			if err != nil {
				return err
			}
			i.RemovedM = &m
		}
		if i.RemovedAt != nil && (i.RemovedAt.Before(i.InstalledAt) || *i.RemovedM < i.InstalledM) {
			return ErrBeforeInstallation
		}
		if err := checkOverlap(ctx, s, i); err != nil {
			return err
		}
		err := s.Components().UpdateInstallation(ctx, i)
		if errors.Is(err, store.ErrConflict) {
			return ErrAlreadyInstalled
		}
		return err
	})
}

// DeleteBike deletes a bike with its odometer readings, but only if no
// component was ever installed on it.
func DeleteBike(ctx context.Context, st store.Store, ownerID, bikeID string) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		n, err := s.Components().CountInstallationsOnBike(ctx, ownerID, bikeID)
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrBikeHasInstallations
		}
		return s.Bikes().DeleteBike(ctx, ownerID, bikeID)
	})
}

// Create adds a component with its values.
func Create(ctx context.Context, st store.Store, c store.Component, vs []store.Value) error {
	vs, err := values.Prepare(c.OwnerID, vs)
	if err != nil {
		return err
	}
	return st.WithTx(ctx, func(s store.Stores) error {
		if _, err := s.Components().CreateComponent(ctx, c); err != nil {
			return err
		}
		return s.Components().ReplaceComponentValues(ctx, c.OwnerID, c.ID, vs)
	})
}

// Update changes a component and replaces its values.
func Update(ctx context.Context, st store.Store, c store.Component, vs []store.Value) error {
	vs, err := values.Prepare(c.OwnerID, vs)
	if err != nil {
		return err
	}
	return st.WithTx(ctx, func(s store.Stores) error {
		if err := s.Components().UpdateComponent(ctx, c); err != nil {
			return err
		}
		return s.Components().ReplaceComponentValues(ctx, c.OwnerID, c.ID, vs)
	})
}
