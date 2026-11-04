package catalog

import (
	"context"
	"errors"
	"slices"

	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

// ErrInvalidInterval means an interval limit is not positive or too large.
var ErrInvalidInterval = errors.New("catalog: invalid interval")

// IntervalLevels are the levels that can have an interval; "pre_ride" is
// done before every ride instead.
var IntervalLevels = []string{"monthly", "minor", "major"}

// Upper bounds of an interval: 10 years or 100,000 km.
const (
	maxIntervalMonths = 120
	maxIntervalM      = 100_000_000
)

// DefaultIntervals are the level intervals until the owner sets their own
// (see docs/konzept-schritte.md).
var DefaultIntervals = map[string]store.Interval{
	"monthly": {Months: ptr(1)},
	"minor":   {Months: ptr(6), DistanceM: ptr(1_500_000)},
	"major":   {Months: ptr(12), DistanceM: ptr(3_000_000)},
}

func ptr(v int64) *int64 { return &v }

// LevelIntervals returns the interval of every level in IntervalLevels:
// the one the owner set, else the default. A level without any limit is
// missing.
func LevelIntervals(ctx context.Context, s store.Stores, ownerID string) (map[string]store.Interval, error) {
	set, err := s.Steps().ListLevelIntervals(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := map[string]store.Interval{}
	for _, l := range IntervalLevels {
		iv := DefaultIntervals[l]
		if i := slices.IndexFunc(set, func(li store.LevelInterval) bool { return li.Level == l }); i >= 0 {
			iv = set[i].Interval
		}
		if !iv.IsZero() {
			out[l] = iv
		}
	}
	return out, nil
}

// SetLevelIntervals stores the intervals of all IntervalLevels; a level
// missing from intervals gets none.
func SetLevelIntervals(ctx context.Context, st store.Store, ownerID string, intervals map[string]store.Interval) error {
	for l, iv := range intervals {
		if !slices.Contains(IntervalLevels, l) {
			return ErrInvalid
		}
		if err := CheckInterval(iv); err != nil {
			return err
		}
	}
	return st.WithTx(ctx, func(tx store.Stores) error {
		for _, l := range IntervalLevels {
			err := tx.Steps().SetLevelInterval(ctx, ownerID, store.LevelInterval{Level: l, Interval: intervals[l]})
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// CheckInterval returns ErrInvalidInterval unless the limits that are set are
// positive and within the upper bounds.
func CheckInterval(iv store.Interval) error {
	if (iv.Months != nil && (*iv.Months <= 0 || *iv.Months > maxIntervalMonths)) ||
		(iv.DistanceM != nil && (*iv.DistanceM <= 0 || *iv.DistanceM > maxIntervalM)) {
		return ErrInvalidInterval
	}
	return nil
}
