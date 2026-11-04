// Package due computes when the levels and the steps with their own
// interval are due on a bike: after the interval in months or kilometers
// since they were last done, whatever comes first. Follow-ups left undone
// in a maintenance are due as open work.
package due

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/catalog"
	"github.com/Mik-/spokebook/internal/app/spokebook/odometer"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// State says how urgent an item is.
type State int

const (
	OK State = iota
	Soon
	Overdue
)

// String returns "ok", "soon" or "overdue".
func (s State) String() string { return [...]string{"ok", "soon", "overdue"}[s] }

// SoonPercent is the share of the interval from which an item is due soon,
// as for the wear of components.
const SoonPercent = 80

// Item is a level, a step with its own interval or open work. The interval
// counts from the last completed maintenance that did it (Done), else from
// the bike going into service.
type Item struct {
	// Level is set for a level, Step for a step with its own interval or
	// open work, Open for open work.
	Level    string
	Step     store.Step
	Open     *OpenWork
	Interval store.Interval
	Done     bool
	SinceAt  time.Time
	// DueAt is set for an interval in months, Expired once it has passed;
	// LeftM (negative when exceeded) for one in meters once the odometer is
	// known.
	DueAt   *time.Time
	Expired bool
	LeftM   *int64
	Percent int
	State   State
}

// OpenWork is a follow-up left undone in a completed maintenance: Step is
// the undone copy, Origin the step that led to it (empty if unknown) and
// At the date of the maintenance. It stays open until a later completed
// maintenance does the catalog step or it is dismissed.
type OpenWork struct {
	MaintenanceID string
	Step          store.MaintenanceStep
	Origin        store.MaintenanceStep
	At            time.Time
}

// RemainM returns the meters left, 0 if exceeded or unknown.
func (it Item) RemainM() int64 {
	if it.LeftM == nil {
		return 0
	}
	return max(*it.LeftM, 0)
}

// OverM returns the meters beyond the interval, 0 if not exceeded.
func (it Item) OverM() int64 {
	if it.LeftM == nil {
		return 0
	}
	return max(-*it.LeftM, 0)
}

// Input is what Compute needs to know about a bike.
type Input struct {
	Bike store.Bike
	// Steps are the catalog steps applicable to the bike.
	Steps  []store.Step
	Levels map[string]store.Interval
	// Maintenances are the bike's maintenances with their steps.
	Maintenances []store.Maintenance
	Odometer     odometer.Timeline
}

// since is a point an interval counts from.
type since struct {
	done bool
	at   time.Time
	m    *int64
}

// Compute returns the items of the bike at time now, the most urgent
// first. A level counts only if it has steps; one done in a higher level
// counts as done (its steps include those of the lower ones). Items whose
// interval can't be measured yet (only kilometers, but no odometer) are
// left out.
func Compute(in Input, now time.Time) []Item {
	start := inService(in)
	var current *int64
	if m, ok := in.Odometer.Current(); ok {
		current = &m
	}
	var items []Item
	for _, l := range catalog.Levels {
		iv, ok := in.Levels[l]
		if !ok || !slices.ContainsFunc(in.Steps, func(s store.Step) bool { return slices.Contains(s.Levels, l) }) {
			continue
		}
		rank := slices.Index(catalog.Levels, l)
		from := last(in.Maintenances, start, func(m store.Maintenance) bool {
			return slices.Index(catalog.Levels, m.Level) >= rank
		})
		if it, ok := measure(iv, from, current, now); ok {
			it.Level = l
			items = append(items, it)
		}
	}
	open := openWork(in)
	items = append(items, open...)
	for _, s := range in.Steps {
		if s.Interval.IsZero() || slices.ContainsFunc(open, func(it Item) bool { return it.Step.ID == s.ID }) {
			continue
		}
		from := last(in.Maintenances, start, func(m store.Maintenance) bool {
			return slices.ContainsFunc(m.Steps, func(ms store.MaintenanceStep) bool {
				return ms.Done && ms.CatalogStepID == s.ID
			})
		})
		if it, ok := measure(s.Interval, from, current, now); ok {
			it.Step = s
			items = append(items, it)
		}
	}
	slices.SortStableFunc(items, func(a, b Item) int {
		return cmp.Or(cmp.Compare(b.State, a.State), cmp.Compare(b.Percent, a.Percent))
	})
	return items
}

// openWork returns the follow-ups left undone in completed maintenances
// that no later one did, once per catalog step (the oldest), as overdue.
func openWork(in Input) []Item {
	var items []Item
	for _, m := range in.Maintenances {
		if m.Open() {
			continue
		}
		for _, ms := range m.Steps {
			if ms.OriginID == "" || ms.Done || ms.Dismissed {
				continue
			}
			i := slices.IndexFunc(in.Steps, func(s store.Step) bool { return s.ID == ms.CatalogStepID })
			if i < 0 || doneSince(in.Maintenances, m, ms.CatalogStepID) {
				continue
			}
			ow := &OpenWork{MaintenanceID: m.ID, Step: ms, At: m.PerformedAt}
			if j := slices.IndexFunc(m.Steps, func(o store.MaintenanceStep) bool { return o.ID == ms.OriginID }); j >= 0 {
				ow.Origin = m.Steps[j]
			}
			k := slices.IndexFunc(items, func(it Item) bool { return it.Step.ID == ms.CatalogStepID })
			switch {
			case k < 0:
				items = append(items, Item{Step: in.Steps[i], Open: ow, SinceAt: m.PerformedAt, Percent: 100, State: Overdue})
			case m.PerformedAt.Before(items[k].Open.At):
				items[k].Open, items[k].SinceAt = ow, m.PerformedAt
			}
		}
	}
	return items
}

// doneSince reports whether a completed maintenance other than m, not
// before it, did the catalog step id.
func doneSince(ms []store.Maintenance, m store.Maintenance, id string) bool {
	return slices.ContainsFunc(ms, func(o store.Maintenance) bool {
		return o.ID != m.ID && !o.Open() && !o.PerformedAt.Before(m.PerformedAt) &&
			slices.ContainsFunc(o.Steps, func(s store.MaintenanceStep) bool { return s.Done && s.CatalogStepID == id })
	})
}

// inService is when the bike went into service: its date, else that of
// the first reading or ride, else when it was added; its odometer, else
// the one the odometer starts from.
func inService(in Input) since {
	s := since{at: in.Bike.CreatedAt, m: in.Bike.InServiceM}
	if first, ok := in.Odometer.First(); ok {
		s.at = first
	}
	if m, ok := in.Odometer.Origin(); ok && s.m == nil {
		s.m = &m
	}
	if in.Bike.InServiceAt != nil {
		s.at = *in.Bike.InServiceAt
	}
	return s
}

// last returns the newest completed maintenance that counts, else start.
func last(ms []store.Maintenance, start since, counts func(store.Maintenance) bool) since {
	from := start
	for _, m := range ms {
		if m.Open() || !counts(m) {
			continue
		}
		if !from.done || m.PerformedAt.After(from.at) {
			from = since{done: true, at: m.PerformedAt, m: &m.DistanceM}
		}
	}
	return from
}

// measure computes how much of the interval is used since from; ok is
// false if neither limit can be measured.
func measure(iv store.Interval, from since, current *int64, now time.Time) (Item, bool) {
	it := Item{Interval: iv, Done: from.done, SinceAt: from.at}
	ok := false
	if iv.Months != nil {
		dueAt := from.at.AddDate(0, int(*iv.Months), 0)
		it.DueAt = &dueAt
		it.Expired = !now.Before(dueAt)
		it.Percent = int(100 * now.Sub(from.at).Hours() / dueAt.Sub(from.at).Hours())
		ok = true
	}
	if iv.DistanceM != nil && current != nil && from.m != nil {
		used := *current - *from.m
		left := *iv.DistanceM - used
		it.LeftM = &left
		it.Percent = max(it.Percent, int(used*100 / *iv.DistanceM))
		ok = true
	}
	it.Percent = max(it.Percent, 0)
	switch {
	case it.Percent >= 100:
		it.State = Overdue
	case it.Percent >= SoonPercent:
		it.State = Soon
	}
	return it, ok
}

// Load computes the items of the bike from the store.
func Load(ctx context.Context, s store.Stores, ownerID string, bike store.Bike, now time.Time) ([]Item, error) {
	in := Input{Bike: bike}
	steps, err := catalog.Sorted(ctx, s, ownerID)
	if err != nil {
		return nil, err
	}
	in.Steps = catalog.Applicable(steps, bike.Features)
	if in.Levels, err = catalog.LevelIntervals(ctx, s, ownerID); err != nil {
		return nil, err
	}
	if in.Odometer, err = odometer.Load(ctx, s, bike); err != nil {
		return nil, err
	}
	list, err := s.Maintenances().ListMaintenances(ctx, ownerID, bike.ID)
	if err != nil {
		return nil, err
	}
	for _, m := range list {
		if m.Open() {
			continue
		}
		full, err := s.Maintenances().GetMaintenance(ctx, ownerID, m.ID)
		if err != nil {
			return nil, err
		}
		in.Maintenances = append(in.Maintenances, full)
	}
	return Compute(in, now), nil
}

// Pending returns the items that are due soon or overdue.
func Pending(items []Item) []Item {
	return slices.DeleteFunc(slices.Clone(items), func(it Item) bool { return it.State == OK })
}

// Next returns the item to show as next due: the highest pending level
// (it includes the lower ones), else the most urgent pending step; nil if
// nothing is pending.
func Next(items []Item) *Item {
	pending := Pending(items)
	if len(pending) == 0 {
		return nil
	}
	next := &pending[0]
	for i, it := range pending {
		if it.Level != "" && (next.Level == "" || slices.Index(catalog.Levels, it.Level) > slices.Index(catalog.Levels, next.Level)) {
			next = &pending[i]
		}
	}
	return next
}

// Suggest returns what to do in a new maintenance: the highest pending
// level (it includes the lower ones) and the pending steps that are not
// part of it.
func Suggest(items []Item) (level string, stepIDs []string) {
	for _, it := range Pending(items) {
		if it.Level != "" && slices.Index(catalog.Levels, it.Level) > slices.Index(catalog.Levels, level) {
			level = it.Level
		}
	}
	for _, it := range Pending(items) {
		if it.Step.ID != "" && !slices.Contains(it.Step.Levels, level) {
			stepIDs = append(stepIDs, it.Step.ID)
		}
	}
	return level, stepIDs
}
