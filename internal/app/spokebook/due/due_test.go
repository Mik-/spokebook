package due_test

import (
	"testing"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/due"
	"github.com/Mik-/spokebook/internal/app/spokebook/odometer"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

func ptr(v int64) *int64 { return &v }

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func reading(at string, km int64) store.OdometerReading {
	return store.OdometerReading{RecordedAt: day(at), DistanceM: km * 1000}
}

func completed(level, at string, km int64, done ...string) store.Maintenance {
	c := day(at)
	m := store.Maintenance{Level: level, PerformedAt: c, DistanceM: km * 1000, CompletedAt: &c}
	for _, id := range done {
		m.Steps = append(m.Steps, store.MaintenanceStep{CatalogStepID: id, Done: true})
	}
	return m
}

var (
	monthly = store.Step{ID: "s1", Levels: []string{"monthly", "minor", "major"}}
	major   = store.Step{ID: "s2", Levels: []string{"major"}}
	// seatpost is done on demand every 500 km.
	seatpost = store.Step{ID: "s3", Interval: store.Interval{DistanceM: ptr(500_000)}}
)

var levels = map[string]store.Interval{
	"monthly": {Months: ptr(1)},
	"minor":   {Months: ptr(6), DistanceM: ptr(1_500_000)},
	"major":   {Months: ptr(12), DistanceM: ptr(3_000_000)},
}

func find(items []due.Item, level, stepID string) *due.Item {
	for i, it := range items {
		if it.Level == level && it.Step.ID == stepID {
			return &items[i]
		}
	}
	return nil
}

func TestLevels(t *testing.T) {
	in := due.Input{
		Bike:   store.Bike{InServiceAt: ptrTime(day("2026-01-01")), InServiceM: ptr(0)},
		Steps:  []store.Step{monthly, major},
		Levels: levels,
		// The major one also counts for the monthly check and the minor
		// inspection; an open one doesn't count.
		Maintenances: []store.Maintenance{
			completed("major", "2026-09-01", 1000),
			{Level: "monthly", PerformedAt: day("2026-10-20"), DistanceM: 1700_000},
		},
		Odometer: odometer.NewTimeline(store.Bike{InServiceM: ptr(0)}, []store.OdometerReading{reading("2026-10-20", 1700), reading("2026-01-01", 0)}, nil),
	}
	items := due.Compute(in, day("2026-10-20"))

	m := find(items, "monthly", "")
	if m == nil || m.State != due.Overdue || !m.Done || !m.SinceAt.Equal(day("2026-09-01")) {
		t.Fatalf("monthly = %+v", m)
	}
	if !m.DueAt.Equal(day("2026-10-01")) {
		t.Errorf("monthly due at %v", m.DueAt)
	}
	// 700 of 1500 km, but 49 days of ~6 months: by distance.
	minor := find(items, "minor", "")
	if minor == nil || minor.State != due.OK || *minor.LeftM != 800_000 || minor.Percent != 46 {
		t.Errorf("minor = %+v", minor)
	}
	if items[0].Level != "monthly" {
		t.Errorf("most urgent = %+v", items[0])
	}
	if find(items, "pre_ride", "") != nil {
		t.Error("pre_ride has no interval")
	}
}

func TestLevelWithoutSteps(t *testing.T) {
	in := due.Input{Bike: store.Bike{CreatedAt: day("2026-01-01")}, Steps: []store.Step{major}, Levels: levels}
	items := due.Compute(in, day("2026-02-01"))
	if len(items) != 1 || items[0].Level != "major" || items[0].Done {
		t.Fatalf("items = %+v", items)
	}
}

func TestStepInterval(t *testing.T) {
	in := due.Input{
		Bike:  store.Bike{},
		Steps: []store.Step{seatpost},
		Maintenances: []store.Maintenance{
			completed("", "2026-05-01", 200, "s3"),
			// Copied but not ticked: doesn't count.
			{Level: "", PerformedAt: day("2026-06-01"), DistanceM: 500_000, CompletedAt: ptrTime(day("2026-06-01")), Steps: []store.MaintenanceStep{{CatalogStepID: "s3"}}},
		},
		Odometer: odometer.NewTimeline(store.Bike{}, []store.OdometerReading{reading("2026-07-01", 620), reading("2026-04-01", 0)}, nil),
	}
	items := due.Compute(in, day("2026-07-01"))
	s := find(items, "", "s3")
	if s == nil || s.State != due.Soon || *s.LeftM != 80_000 || s.Percent != 84 || s.DueAt != nil {
		t.Fatalf("seatpost = %+v", s)
	}
}

func TestKilometersWithoutOdometer(t *testing.T) {
	in := due.Input{Bike: store.Bike{CreatedAt: day("2026-01-01")}, Steps: []store.Step{seatpost}}
	if items := due.Compute(in, day("2026-07-01")); len(items) != 0 {
		t.Fatalf("items = %+v", items)
	}
}

func TestInServiceFromFirstReading(t *testing.T) {
	// A used bike without in-service values counts from its first reading.
	in := due.Input{
		Bike:     store.Bike{CreatedAt: day("2026-06-01")},
		Steps:    []store.Step{seatpost},
		Odometer: odometer.NewTimeline(store.Bike{}, []store.OdometerReading{reading("2026-07-01", 5100), reading("2026-03-01", 5000)}, nil),
	}
	s := find(due.Compute(in, day("2026-07-01")), "", "s3")
	if s == nil || *s.LeftM != 400_000 || !s.SinceAt.Equal(day("2026-03-01")) {
		t.Fatalf("seatpost = %+v", s)
	}
}

func TestSuggest(t *testing.T) {
	tubeless := store.Step{ID: "s4", Levels: []string{"monthly"}, Interval: store.Interval{Months: ptr(1)}}
	items := []due.Item{
		{Level: "monthly", State: due.Overdue},
		{Level: "minor", State: due.Soon},
		{Level: "major", State: due.OK},
		{Step: seatpost, State: due.Overdue},
		{Step: tubeless, State: due.Overdue},
	}
	level, steps := due.Suggest(items)
	if level != "minor" || len(steps) != 2 || steps[0] != "s3" || steps[1] != "s4" {
		t.Errorf("Suggest = %q %v", level, steps)
	}
	items[1].State = due.OK
	items[2].State = due.OK
	if level, steps = due.Suggest(items); level != "monthly" || len(steps) != 1 {
		t.Errorf("Suggest = %q %v", level, steps)
	}
}

func TestNext(t *testing.T) {
	items := []due.Item{
		{Step: seatpost, State: due.Overdue},
		{Level: "monthly", State: due.Overdue},
		{Level: "minor", State: due.Soon},
		{Level: "major", State: due.OK},
	}
	if next := due.Next(items); next == nil || next.Level != "minor" {
		t.Errorf("Next = %+v", next)
	}
	items[1].State, items[2].State = due.OK, due.OK
	if next := due.Next(items); next == nil || next.Step.ID != "s3" {
		t.Errorf("Next = %+v", next)
	}
	items[0].State = due.OK
	if next := due.Next(items); next != nil {
		t.Errorf("Next = %+v", next)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestOpenWork(t *testing.T) {
	pads := store.Step{ID: "pads"}
	check := func(at string, steps ...store.MaintenanceStep) store.Maintenance {
		c := day(at)
		return store.Maintenance{ID: at, PerformedAt: c, CompletedAt: &c, Steps: steps}
	}
	measured := store.MaintenanceStep{ID: "m1", CatalogStepID: "measure", Title: "Measure", Done: true}
	left := store.MaintenanceStep{ID: "m2", CatalogStepID: "pads", OriginID: "m1"}
	in := due.Input{Bike: store.Bike{CreatedAt: day("2026-01-01")}, Steps: []store.Step{pads, seatpost}}

	// A follow-up left undone is overdue open work.
	in.Maintenances = []store.Maintenance{check("2026-05-01", measured, left)}
	items := due.Compute(in, day("2026-07-01"))
	it := find(items, "", "pads")
	if it == nil || it.Open == nil || it.State != due.Overdue || it.Open.MaintenanceID != "2026-05-01" ||
		it.Open.Origin.Title != "Measure" || !it.Open.At.Equal(day("2026-05-01")) {
		t.Fatalf("open work = %+v", it)
	}
	if _, steps := due.Suggest(items); len(steps) != 1 || steps[0] != "pads" {
		t.Errorf("Suggest = %v", steps)
	}

	// Left undone again later, it shows once, since the first time.
	again := store.MaintenanceStep{ID: "m3", CatalogStepID: "pads", OriginID: "m1"}
	in.Maintenances = append(in.Maintenances, check("2026-06-01", measured, again))
	if items := due.Compute(in, day("2026-07-01")); len(items) != 1 || !items[0].Open.At.Equal(day("2026-05-01")) {
		t.Errorf("twice = %+v", items)
	}

	// Done in a later maintenance, dismissed, or a plain undone step: no
	// open work.
	for name, ms := range map[string][]store.Maintenance{
		"done later":   {check("2026-05-01", measured, left), check("2026-06-01", store.MaintenanceStep{CatalogStepID: "pads", Done: true})},
		"dismissed":    {check("2026-05-01", measured, store.MaintenanceStep{CatalogStepID: "pads", OriginID: "m1", Dismissed: true})},
		"no follow-up": {check("2026-05-01", store.MaintenanceStep{CatalogStepID: "pads"})},
		"done before":  {check("2026-05-01", measured, left), check("2026-04-01", store.MaintenanceStep{CatalogStepID: "pads", Done: true})},
	} {
		in.Maintenances = ms
		got := find(due.Compute(in, day("2026-07-01")), "", "pads")
		if want := name == "done before"; (got != nil) != want {
			t.Errorf("%s: %+v", name, got)
		}
	}

	// A step with its own interval that is open work shows only as such.
	open := store.MaintenanceStep{CatalogStepID: "s3", OriginID: "m1"}
	in.Maintenances = []store.Maintenance{check("2026-05-01", measured, open)}
	in.Odometer = odometer.NewTimeline(store.Bike{}, []store.OdometerReading{reading("2026-07-01", 620), reading("2026-04-01", 0)}, nil)
	items = due.Compute(in, day("2026-07-01"))
	if n := len(items); n != 1 || items[0].Open == nil {
		t.Errorf("seatpost = %+v", items)
	}
}
