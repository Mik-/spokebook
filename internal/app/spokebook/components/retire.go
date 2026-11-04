package components

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// Reasons for retiring, translated via "retire.reason.<reason>".
var (
	ComponentRetireReasons = []string{"worn", "broken", "sold", "lost", "other"}
	BikeRetireReasons      = []string{"sold", "stolen", "broken", "other"}
)

var (
	// ErrRetired means the bike or component is retired and can't be used.
	ErrRetired = errors.New("components: retired")
	// ErrInvalidReason means the retire reason is unknown.
	ErrInvalidReason = errors.New("components: invalid retire reason")
)

// RetireComponent retires a component. If it is installed, the
// installation ends at the same date; distanceM is the bike's odometer then
// (nil uses the reading at that date), recorded as a reading if it differs.
func RetireComponent(ctx context.Context, st store.Store, ownerID, id string, at time.Time, reason string, distanceM *int64) error {
	if !slices.Contains(ComponentRetireReasons, reason) {
		return ErrInvalidReason
	}
	return st.WithTx(ctx, func(s store.Stores) error {
		return retireComponentTx(ctx, s, ownerID, id, at, reason, distanceM)
	})
}

// retireComponentTx is RetireComponent within a running transaction.
func retireComponentTx(ctx context.Context, s store.Stores, ownerID, id string, at time.Time, reason string, distanceM *int64) error {
	c, err := s.Components().GetComponent(ctx, ownerID, id)
	if err != nil {
		return err
	}
	if !c.Active() {
		return ErrRetired
	}
	insts, err := s.Components().ListInstallations(ctx, ownerID, id)
	if err != nil {
		return err
	}
	for _, i := range insts {
		if i.Open() {
			if err := closeInstallation(ctx, s, i, at, distanceM); err != nil {
				return err
			}
		}
	}
	return s.Components().SetComponentRetired(ctx, ownerID, id, &at, reason)
}

// RetireBike retires a bike and removes all components installed on it at
// that date. distanceM is the bike's odometer then (nil uses the reading at
// that date), recorded as a reading if it differs.
func RetireBike(ctx context.Context, st store.Store, ownerID, id string, at time.Time, reason string, distanceM *int64) error {
	if !slices.Contains(BikeRetireReasons, reason) {
		return ErrInvalidReason
	}
	return st.WithTx(ctx, func(s store.Stores) error {
		b, err := s.Bikes().GetBike(ctx, ownerID, id)
		if err != nil {
			return err
		}
		if !b.Active() {
			return ErrRetired
		}
		if err := recordGiven(ctx, s, ownerID, id, at, distanceM); err != nil {
			return err
		}
		insts, err := s.Components().ListAllInstallations(ctx, ownerID)
		if err != nil {
			return err
		}
		for _, i := range insts {
			if i.Open() && i.BikeID == id {
				if err := closeInstallation(ctx, s, i, at, distanceM); err != nil {
					return err
				}
			}
		}
		return s.Bikes().SetBikeRetired(ctx, ownerID, id, &at, reason)
	})
}

// Reactivate brings a retired component back into use. Its installations
// stay closed.
func Reactivate(ctx context.Context, st store.Store, ownerID, id string) error {
	return st.Components().SetComponentRetired(ctx, ownerID, id, nil, "")
}

// ReactivateBike brings a retired bike back into use.
func ReactivateBike(ctx context.Context, st store.Store, ownerID, id string) error {
	return st.Bikes().SetBikeRetired(ctx, ownerID, id, nil, "")
}

// closeInstallation ends i at the given date with the same checks as Remove
// and records a given odometer.
func closeInstallation(ctx context.Context, s store.Stores, i store.Installation, at time.Time, distanceM *int64) error {
	if at.Before(i.InstalledAt) {
		return ErrBeforeInstallation
	}
	if err := recordGiven(ctx, s, i.OwnerID, i.BikeID, at, distanceM); err != nil {
		return err
	}
	m, err := distanceAt(ctx, s, i.OwnerID, i.BikeID, at, distanceM)
	if err != nil {
		return err
	}
	if m < i.InstalledM {
		return ErrBeforeInstallation
	}
	return s.Components().CloseInstallation(ctx, i.OwnerID, i.ID, at, m, distanceM == nil)
}
