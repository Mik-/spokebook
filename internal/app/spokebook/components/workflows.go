package components

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/spokebook/spokebook/internal/app/spokebook/odometer"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
	"github.com/spokebook/spokebook/internal/app/spokebook/values"
)

// Mount says where and when a component is mounted. DistanceM is the
// bike's odometer at that time; it is recorded as a reading if it differs
// from the recorded one. Without it, the odometer at that date is used.
// AllowDuplicate skips the DuplicateError for a component of the same type
// at the same position.
type Mount struct {
	OwnerID        string
	BikeID         string
	Position       string
	At             time.Time
	DistanceM      *int64
	AllowDuplicate bool
}

// AddToBike creates c with its values and installs it on the bike in one
// step.
func AddToBike(ctx context.Context, st store.Store, c store.Component, vs []store.Value, m Mount) error {
	if m.Position != "" && !slices.Contains(Positions, m.Position) {
		return ErrInvalidPosition
	}
	vs, err := values.Prepare(c.OwnerID, vs)
	if err != nil {
		return err
	}
	return st.WithTx(ctx, func(s store.Stores) error {
		return createAndInstall(ctx, s, c, vs, m)
	})
}

// Replace removes the component of the open installation oldInstallationID
// and installs the new component c with its values in its place, at the
// same bike. With a
// retireReason the old component is also retired; otherwise it stays
// available, e.g. as a spare.
func Replace(ctx context.Context, st store.Store, oldInstallationID, retireReason string, c store.Component, vs []store.Value, m Mount) error {
	if m.Position != "" && !slices.Contains(Positions, m.Position) {
		return ErrInvalidPosition
	}
	if retireReason != "" && !slices.Contains(ComponentRetireReasons, retireReason) {
		return ErrInvalidReason
	}
	vs, err := values.Prepare(c.OwnerID, vs)
	if err != nil {
		return err
	}
	return st.WithTx(ctx, func(s store.Stores) error {
		m, _, err := removeReplacedTx(ctx, s, oldInstallationID, retireReason, m)
		if err != nil {
			return err
		}
		return createAndInstall(ctx, s, c, vs, m)
	})
}

// ReplaceWithSpare is Replace with an existing component that is not
// installed, e.g. a spare, instead of a new one.
func ReplaceWithSpare(ctx context.Context, st store.Store, oldInstallationID, retireReason, spareID string, m Mount) error {
	if m.Position != "" && !slices.Contains(Positions, m.Position) {
		return ErrInvalidPosition
	}
	if retireReason != "" && !slices.Contains(ComponentRetireReasons, retireReason) {
		return ErrInvalidReason
	}
	return st.WithTx(ctx, func(s store.Stores) error {
		m, oldComponentID, err := removeReplacedTx(ctx, s, oldInstallationID, retireReason, m)
		if err != nil {
			return err
		}
		if spareID == oldComponentID {
			return ErrAlreadyInstalled
		}
		_, err = installTx(ctx, s, store.Installation{
			ID: uuid.Must(uuid.NewV7()).String(), OwnerID: m.OwnerID, ComponentID: spareID, BikeID: m.BikeID,
			Position: m.Position, InstalledAt: m.At, CreatedAt: time.Now().UTC(),
		}, m.DistanceM, m.AllowDuplicate)
		return err
	})
}

// removeReplacedTx ends the open installation oldInstallationID and, with a
// retireReason, retires its component. It returns m set to the bike of the
// installation and the removed component's ID.
func removeReplacedTx(ctx context.Context, s store.Stores, oldInstallationID, retireReason string, m Mount) (Mount, string, error) {
	old, err := s.Components().GetInstallation(ctx, m.OwnerID, oldInstallationID)
	if err != nil {
		return m, "", err
	}
	if !old.Open() {
		return m, "", store.ErrNotFound
	}
	m.BikeID = old.BikeID
	if retireReason != "" {
		err = retireComponentTx(ctx, s, m.OwnerID, old.ComponentID, m.At, retireReason, m.DistanceM)
	} else {
		err = closeInstallation(ctx, s, old, m.At, m.DistanceM)
	}
	return m, old.ComponentID, err
}

// RecordOdometerTx records the bike's odometer at the given date unless it
// equals the known one (a reading or the odometer at going into service).
func RecordOdometerTx(ctx context.Context, s store.Stores, ownerID, bikeID string, at time.Time, distanceM int64) error {
	if known, err := distanceAt(ctx, s, ownerID, bikeID, at, nil); err == nil && known == distanceM {
		return nil
	} else if err != nil && !errors.Is(err, ErrNoOdometer) {
		return err
	}
	return odometer.EnsureTx(ctx, s, store.OdometerReading{
		ID: uuid.Must(uuid.NewV7()).String(), OwnerID: ownerID, BikeID: bikeID,
		RecordedAt: at, DistanceM: distanceM, Source: store.SourceManual, CreatedAt: time.Now().UTC(),
	})
}

// recordGiven is RecordOdometerTx for an optional odometer; nil records
// nothing.
func recordGiven(ctx context.Context, s store.Stores, ownerID, bikeID string, at time.Time, distanceM *int64) error {
	if distanceM == nil {
		return nil
	}
	return RecordOdometerTx(ctx, s, ownerID, bikeID, at, *distanceM)
}

func createAndInstall(ctx context.Context, s store.Stores, c store.Component, vs []store.Value, m Mount) error {
	if _, err := s.Components().CreateComponent(ctx, c); err != nil {
		return err
	}
	if err := s.Components().ReplaceComponentValues(ctx, c.OwnerID, c.ID, vs); err != nil {
		return err
	}
	_, err := installTx(ctx, s, store.Installation{
		ID: uuid.Must(uuid.NewV7()).String(), OwnerID: m.OwnerID, ComponentID: c.ID, BikeID: m.BikeID,
		Position: m.Position, InstalledAt: m.At, CreatedAt: time.Now().UTC(),
	}, m.DistanceM, m.AllowDuplicate)
	return err
}
