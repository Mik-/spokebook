// Package odometer records odometer readings. An odometer only counts up, so
// a reading must fit between its chronological neighbors.
package odometer

import (
	"context"
	"errors"
	"fmt"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// TooLowError means the reading is below an earlier (or same-day) reading.
type TooLowError struct{ Prev store.OdometerReading }

func (e *TooLowError) Error() string {
	return fmt.Sprintf("odometer: below earlier reading of %d m", e.Prev.DistanceM)
}

// BelowServiceError means the reading is below the bike's odometer when it
// went into service.
type BelowServiceError struct{ InServiceM int64 }

func (e *BelowServiceError) Error() string {
	return fmt.Sprintf("odometer: below the %d m at going into service", e.InServiceM)
}

// ErrBeforeService means the reading is dated before the bike went into
// service.
var ErrBeforeService = errors.New("odometer: before going into service")

// TooHighError means the reading is above a later reading.
type TooHighError struct{ Next store.OdometerReading }

func (e *TooHighError) Error() string {
	return fmt.Sprintf("odometer: above later reading of %d m", e.Next.DistanceM)
}

// Record adds r after checking it against the neighboring readings. Equal
// values are allowed (days without riding).
func Record(ctx context.Context, st store.Store, r store.OdometerReading) (store.OdometerReading, error) {
	var added store.OdometerReading
	err := st.WithTx(ctx, func(s store.Stores) error {
		var err error
		added, err = RecordTx(ctx, s, r)
		return err
	})
	return added, err
}

// RecordTx is Record within a running transaction.
func RecordTx(ctx context.Context, s store.Stores, r store.OdometerReading) (store.OdometerReading, error) {
	if err := check(ctx, s, r); err != nil {
		return store.OdometerReading{}, err
	}
	added, err := s.Odometer().AddReading(ctx, r)
	if err != nil {
		return store.OdometerReading{}, err
	}
	return added, RefreshTx(ctx, s, r.OwnerID, r.BikeID)
}

// EnsureTx records r unless the bike's last reading at or before
// r.RecordedAt already has the same distance. It is used where the odometer
// is entered as part of another action, e.g. replacing a component.
func EnsureTx(ctx context.Context, s store.Stores, r store.OdometerReading) error {
	prev, err := s.Odometer().PrevReading(ctx, r.OwnerID, r.BikeID, r.RecordedAt, "")
	switch {
	case err == nil && prev.DistanceM == r.DistanceM:
		return nil
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return err
	}
	_, err = RecordTx(ctx, s, r)
	return err
}

// Update changes date and distance of an existing reading with the same
// checks as Record.
func Update(ctx context.Context, st store.Store, r store.OdometerReading) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		if err := check(ctx, s, r); err != nil {
			return err
		}
		if err := s.Odometer().UpdateReading(ctx, r); err != nil {
			return err
		}
		return RefreshTx(ctx, s, r.OwnerID, r.BikeID)
	})
}

// Delete deletes the reading r.
func Delete(ctx context.Context, st store.Store, r store.OdometerReading) error {
	return st.WithTx(ctx, func(s store.Stores) error {
		if err := s.Odometer().DeleteReading(ctx, r.OwnerID, r.ID); err != nil {
			return err
		}
		return RefreshTx(ctx, s, r.OwnerID, r.BikeID)
	})
}

// check verifies that r is not before the bike went into service and fits
// between the other readings of its bike.
func check(ctx context.Context, s store.Stores, r store.OdometerReading) error {
	b, err := s.Bikes().GetBike(ctx, r.OwnerID, r.BikeID)
	if err != nil {
		return err
	}
	if b.InServiceAt != nil && r.RecordedAt.Before(*b.InServiceAt) {
		return ErrBeforeService
	}
	if b.InServiceM != nil && r.DistanceM < *b.InServiceM {
		return &BelowServiceError{InServiceM: *b.InServiceM}
	}
	odo := s.Odometer()
	prev, err := odo.PrevReading(ctx, r.OwnerID, r.BikeID, r.RecordedAt, r.ID)
	switch {
	case err == nil && r.DistanceM < prev.DistanceM:
		return &TooLowError{Prev: prev}
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return err
	}
	next, err := odo.NextReading(ctx, r.OwnerID, r.BikeID, r.RecordedAt, r.ID)
	switch {
	case err == nil && r.DistanceM > next.DistanceM:
		return &TooHighError{Next: next}
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return err
	}
	return nil
}
