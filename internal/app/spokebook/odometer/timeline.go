package odometer

import (
	"cmp"
	"context"
	"slices"
	"sort"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// Timeline computes a bike's odometer from its manual readings and its
// rides. A reading is the odometer at the end of its day and an anchor:
// on a later day the odometer is the last reading plus the rides after its
// day. It never exceeds the next reading, so readings win over rides that
// don't fit and the odometer only counts up.
//
// Before the first reading the odometer counts from the origin: the
// odometer at going into service, else the first reading minus the rides
// up to it, else 0 at the first ride.
type Timeline struct {
	hasOrigin bool
	// fixedOrigin means the bike's odometer at going into service is known.
	fixedOrigin bool
	origin      time.Time
	originM     int64
	// anchors are the readings by day, the highest one per day.
	anchors []store.OdometerReading
	days    []store.DayDistance
	// cum[i] is the distance of days[:i].
	cum []int64
}

// NewTimeline builds the timeline of bike from its readings and daily
// distances in any order.
func NewTimeline(bike store.Bike, readings []store.OdometerReading, days []store.DayDistance) Timeline {
	var t Timeline
	t.anchors = slices.Clone(readings)
	slices.SortFunc(t.anchors, func(a, b store.OdometerReading) int {
		if c := a.RecordedAt.Compare(b.RecordedAt); c != 0 {
			return c
		}
		return cmp.Compare(b.DistanceM, a.DistanceM)
	})
	t.anchors = slices.CompactFunc(t.anchors, func(a, b store.OdometerReading) bool {
		return a.RecordedAt.Equal(b.RecordedAt)
	})
	t.days = slices.Clone(days)
	slices.SortFunc(t.days, func(a, b store.DayDistance) int { return a.Day.Compare(b.Day) })
	t.cum = make([]int64, len(t.days)+1)
	for i, d := range t.days {
		t.cum[i+1] = t.cum[i] + d.DistanceM
	}
	switch {
	case bike.InServiceM != nil:
		t.hasOrigin, t.fixedOrigin, t.originM = true, true, *bike.InServiceM
		if bike.InServiceAt != nil {
			t.origin = *bike.InServiceAt
		}
	case len(t.anchors) > 0:
		first := t.anchors[0]
		t.hasOrigin, t.origin = true, first.RecordedAt
		if len(t.days) > 0 && t.days[0].Day.Before(t.origin) {
			t.origin = t.days[0].Day
		}
		t.originM = max(first.DistanceM-t.upTo(first.RecordedAt)+t.before(t.origin), 0)
	case len(t.days) > 0:
		t.hasOrigin, t.origin = true, t.days[0].Day
	}
	return t
}

// upTo returns the distance ridden up to and including day.
func (t Timeline) upTo(day time.Time) int64 {
	return t.cum[sort.Search(len(t.days), func(i int) bool { return t.days[i].Day.After(day) })]
}

// before returns the distance ridden before day.
func (t Timeline) before(day time.Time) int64 {
	return t.cum[sort.Search(len(t.days), func(i int) bool { return !t.days[i].Day.Before(day) })]
}

// At returns the odometer at the end of day; ok is false if it is unknown.
func (t Timeline) At(day time.Time) (m int64, ok bool) {
	if !t.hasOrigin {
		return 0, false
	}
	i := sort.Search(len(t.anchors), func(i int) bool { return t.anchors[i].RecordedAt.After(day) })
	switch {
	case i > 0:
		a := t.anchors[i-1]
		m = a.DistanceM + t.upTo(day) - t.upTo(a.RecordedAt)
	case day.Before(t.origin):
		if !t.fixedOrigin {
			return 0, false
		}
		return t.originM, true
	default:
		m = t.originM + t.upTo(day) - t.before(t.origin)
	}
	if i < len(t.anchors) {
		m = min(m, t.anchors[i].DistanceM)
	}
	return m, true
}

// Current returns the odometer after the last reading and ride.
func (t Timeline) Current() (int64, bool) {
	return t.At(time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC))
}

// First returns the day of the first reading or ride.
func (t Timeline) First() (time.Time, bool) {
	var first time.Time
	if len(t.anchors) > 0 {
		first = t.anchors[0].RecordedAt
	}
	if len(t.days) > 0 && (first.IsZero() || t.days[0].Day.Before(first)) {
		first = t.days[0].Day
	}
	return first, !first.IsZero()
}

// Origin returns the odometer the timeline starts from, see Timeline.
func (t Timeline) Origin() (int64, bool) { return t.originM, t.hasOrigin }

// Load returns the timeline of bike.
func Load(ctx context.Context, s store.Stores, bike store.Bike) (Timeline, error) {
	readings, err := s.Odometer().ListReadings(ctx, bike.OwnerID, bike.ID)
	if err != nil {
		return Timeline{}, err
	}
	days, err := s.Rides().DailyDistances(ctx, bike.OwnerID)
	if err != nil {
		return Timeline{}, err
	}
	days = slices.DeleteFunc(days, func(d store.DayDistance) bool { return d.BikeID != bike.ID })
	return NewTimeline(bike, readings, days), nil
}

// LoadBike returns the timeline of the owner's bike with the given ID.
func LoadBike(ctx context.Context, s store.Stores, ownerID, bikeID string) (Timeline, error) {
	bike, err := s.Bikes().GetBike(ctx, ownerID, bikeID)
	if err != nil {
		return Timeline{}, err
	}
	return Load(ctx, s, bike)
}

// LoadAll returns the timelines of all bikes of the owner by bike ID.
func LoadAll(ctx context.Context, s store.Stores, ownerID string) (map[string]Timeline, error) {
	bikes, err := s.Bikes().ListBikes(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	readings, err := s.Odometer().ListAllReadings(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	days, err := s.Rides().DailyDistances(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	byBike := make(map[string][]store.OdometerReading)
	for _, r := range readings {
		byBike[r.BikeID] = append(byBike[r.BikeID], r)
	}
	daysByBike := make(map[string][]store.DayDistance)
	for _, d := range days {
		daysByBike[d.BikeID] = append(daysByBike[d.BikeID], d)
	}
	out := make(map[string]Timeline, len(bikes))
	for _, b := range bikes {
		out[b.ID] = NewTimeline(b, byBike[b.ID], daysByBike[b.ID])
	}
	return out, nil
}

// Latest returns the current odometer per bike ID; bikes without one are
// missing.
func Latest(ctx context.Context, s store.Stores, ownerID string) (map[string]int64, error) {
	all, err := LoadAll(ctx, s, ownerID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(all))
	for id, t := range all {
		if m, ok := t.Current(); ok {
			out[id] = m
		}
	}
	return out, nil
}

// RefreshTx updates the distances of the bike's installations that follow
// the computed odometer. Call it after the bike's readings or rides
// changed.
func RefreshTx(ctx context.Context, s store.Stores, ownerID, bikeID string) error {
	t, err := LoadBike(ctx, s, ownerID, bikeID)
	if err != nil {
		return err
	}
	insts, err := s.Components().ListAllInstallations(ctx, ownerID)
	if err != nil {
		return err
	}
	for _, i := range insts {
		if i.BikeID != bikeID || (!i.InstalledAuto && !i.RemovedAuto) {
			continue
		}
		installed, removed := i.InstalledM, i.RemovedM
		if m, ok := t.At(i.InstalledAt); ok && i.InstalledAuto {
			installed = m
		}
		if i.RemovedAuto && i.RemovedAt != nil {
			if m, ok := t.At(*i.RemovedAt); ok {
				removed = &m
			}
		}
		if installed == i.InstalledM && (removed == nil || *removed == *i.RemovedM) {
			continue
		}
		if err := s.Components().SetInstallationDistances(ctx, ownerID, i.ID, installed, removed); err != nil {
			return err
		}
	}
	return nil
}
