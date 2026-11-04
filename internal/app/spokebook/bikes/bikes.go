// Package bikes holds rules for bike data.
package bikes

import (
	"context"
	"errors"
	"slices"

	"github.com/Mik-/spokebook/internal/app/spokebook/odometer"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/values"
)

// Origins are the ways a bike can be acquired, translated via
// "bike.origin.<origin>". An empty origin means unknown.
var Origins = []string{"bought", "bought_used", "self_built"}

// Features are the properties of a bike that switch on modules of the
// step catalog, translated via "feature.<feature>".
var Features = []string{
	"carbon", "tubeless", "ebike", "suspension", "hydraulic_brakes", "hub_gear", "belt", "electronic_shifting",
}

var (
	// ErrInvalidOrigin means the origin is not one of Origins.
	ErrInvalidOrigin = errors.New("bikes: invalid origin")
	// ErrInvalidFeature means a feature is not one of Features.
	ErrInvalidFeature = errors.New("bikes: invalid feature")
	// ErrReadingsBeforeService means the bike has odometer readings dated
	// before it went into service.
	ErrReadingsBeforeService = errors.New("bikes: readings before going into service")
	// ErrRidesBeforeService means rides of the bike are dated before it
	// went into service.
	ErrRidesBeforeService = errors.New("bikes: rides before going into service")
	// ErrReadingsBelowService means the bike has odometer readings below its
	// odometer at going into service.
	ErrReadingsBelowService = errors.New("bikes: readings below the odometer at going into service")
)

// Validate checks the bike's fields that have a fixed set of values.
func Validate(b store.Bike) error {
	if b.Origin != "" && !slices.Contains(Origins, b.Origin) {
		return ErrInvalidOrigin
	}
	for _, f := range b.Features {
		if !slices.Contains(Features, f) {
			return ErrInvalidFeature
		}
	}
	return nil
}

// Update saves b after checking it and that its odometer readings fit the
// date and odometer at going into service.
func Update(ctx context.Context, st store.Store, b store.Bike) error {
	if err := Validate(b); err != nil {
		return err
	}
	return st.WithTx(ctx, func(s store.Stores) error {
		readings, err := s.Odometer().ListReadings(ctx, b.OwnerID, b.ID)
		if err != nil {
			return err
		}
		for _, r := range readings {
			if b.InServiceAt != nil && r.RecordedAt.Before(*b.InServiceAt) {
				return ErrReadingsBeforeService
			}
			if b.InServiceM != nil && r.DistanceM < *b.InServiceM {
				return ErrReadingsBelowService
			}
		}
		if b.InServiceAt != nil {
			rides, err := s.Rides().ListRides(ctx, b.OwnerID)
			if err != nil {
				return err
			}
			for _, r := range rides {
				if r.BikeID == b.ID && r.RiddenOn.Before(*b.InServiceAt) {
					return ErrRidesBeforeService
				}
			}
		}
		if err := s.Bikes().UpdateBike(ctx, b); err != nil {
			return err
		}
		return odometer.RefreshTx(ctx, s, b.OwnerID, b.ID)
	})
}

// SetValues replaces the measurements of a bike (saddle height, stack ...).
// They are documentation only, so every value becomes an info.
func SetValues(ctx context.Context, st store.Store, ownerID, bikeID string, vs []store.Value) error {
	for i := range vs {
		vs[i].Kind = store.KindInfo
	}
	vs, err := values.Prepare(ownerID, vs)
	if err != nil {
		return err
	}
	return st.WithTx(ctx, func(s store.Stores) error {
		if _, err := s.Bikes().GetBike(ctx, ownerID, bikeID); err != nil {
			return err
		}
		return s.Bikes().ReplaceBikeValues(ctx, ownerID, bikeID, vs)
	})
}

// SortFeatures returns fs in the order of Features without duplicates.
func SortFeatures(fs []string) []string {
	var out []string
	for _, f := range Features {
		if slices.Contains(fs, f) {
			out = append(out, f)
		}
	}
	for _, f := range fs {
		if !slices.Contains(Features, f) && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}
