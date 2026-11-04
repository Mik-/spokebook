package rides_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/odometer"
	"github.com/Mik-/spokebook/internal/app/spokebook/rides"
	"github.com/Mik-/spokebook/internal/app/spokebook/rides/parse"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite"
)

const owner = "o1"

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ride(source, id, date string, km int64, gear string) parse.Ride {
	start := day(date).Add(8 * time.Hour)
	return parse.Ride{
		Source: source, ExternalID: id, GearKey: gear, GearName: gear, StartedAt: start,
		EndedAt: start.Add(time.Hour), RiddenOn: day(date), DistanceM: km * 1000,
	}
}

func setup(t *testing.T) (context.Context, store.Store) {
	t.Helper()
	ctx := context.Background()
	st, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	for _, id := range []string{"road", "gravel"} {
		_, err := st.Bikes().CreateBike(ctx, store.Bike{
			ID: id, OwnerID: owner, Name: id, InServiceAt: new(day("2026-01-01")), InServiceM: new(int64(0)),
			CreatedAt: day("2026-01-01"),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return ctx, st
}

func current(t *testing.T, ctx context.Context, st store.Store) map[string]int64 {
	t.Helper()
	latest, err := odometer.Latest(ctx, st, owner)
	if err != nil {
		t.Fatal(err)
	}
	return latest
}

func items(cands []rides.Candidate) []rides.Item {
	var out []rides.Item
	for _, c := range cands {
		if c.Status == rides.New {
			out = append(out, rides.Item{Ride: c.Ride, Auto: true})
		}
	}
	return out
}

func TestImport(t *testing.T) {
	ctx, st := setup(t)
	read := []parse.Ride{
		ride(parse.SourceFIT, "a", "2026-03-01", 40, "profile:Grizl"),
		ride(parse.SourceFIT, "b", "2026-03-02", 30, "profile:Grizl"),
		ride(parse.SourceGPX, "c", "2026-03-03", 20, ""),
		// Before the bikes went into service: stays unassigned.
		ride(parse.SourceGPX, "d", "2025-12-01", 10, ""),
	}
	cands, gears, err := rides.Preview(ctx, st, owner, read)
	if err != nil {
		t.Fatal(err)
	}
	if len(gears) != 1 || gears[0].Mapping != nil {
		t.Fatalf("gears = %+v", gears)
	}
	choice := []rides.GearChoice{{Source: parse.SourceFIT, Key: "profile:Grizl", Name: "Grizl", BikeID: "gravel", Remember: true}}
	res, err := rides.Import(ctx, st, owner, items(cands), choice, "road")
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 4 || res.Unassigned != 1 {
		t.Fatalf("result = %+v", res)
	}
	if km := current(t, ctx, st); km["gravel"] != 70_000 || km["road"] != 20_000 {
		t.Fatalf("odometer = %v", km)
	}

	// Again, plus the same ride as GPX from another device: the FIT rides
	// are known, the GPX one is a duplicate; the gear is mapped now.
	again := append(read[:2:2], ride(parse.SourceGPX, "e", "2026-03-01", 40, ""))
	cands, gears, err = rides.Preview(ctx, st, owner, again)
	if err != nil {
		t.Fatal(err)
	}
	if cands[0].Status != rides.Known || cands[1].Status != rides.Known || cands[2].Status != rides.Duplicate {
		t.Fatalf("candidates = %+v", cands)
	}
	if len(gears) != 0 {
		t.Fatalf("gears = %+v", gears)
	}
}

func TestMoveGear(t *testing.T) {
	ctx, st := setup(t)
	read := []parse.Ride{
		ride(parse.SourceFIT, "a", "2026-03-01", 40, "device:1"),
		ride(parse.SourceFIT, "b", "2026-03-02", 30, "device:1"),
	}
	choice := []rides.GearChoice{{Source: parse.SourceFIT, Key: "device:1", BikeID: "road", Remember: true}}
	cands, _, err := rides.Preview(ctx, st, owner, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rides.Import(ctx, st, owner, items(cands), choice, ""); err != nil {
		t.Fatal(err)
	}
	list, err := st.Rides().ListRides(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	// Ride "b" is the newest; assigned by hand it stays.
	if err := rides.SetBike(ctx, st, owner, list[0].ID, "road"); err != nil {
		t.Fatal(err)
	}
	gears, err := st.Rides().ListRideGears(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := rides.SetGearBike(ctx, st, owner, gears[0].ID, "gravel", true); err != nil {
		t.Fatal(err)
	}
	if km := current(t, ctx, st); km["gravel"] != 40_000 || km["road"] != 30_000 {
		t.Fatalf("odometer = %v", km)
	}

	if err := rides.SetDiscarded(ctx, st, owner, list[1].ID, true); err != nil {
		t.Fatal(err)
	}
	if km := current(t, ctx, st); km["gravel"] != 0 {
		t.Fatalf("odometer = %v", km)
	}
	if err := rides.SetBike(ctx, st, owner, list[1].ID, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown bike: %v", err)
	}

	// Deleting the mapping keeps the rides' bikes; the next import asks
	// again.
	if err := st.Rides().DeleteRideGear(ctx, owner, gears[0].ID); err != nil {
		t.Fatal(err)
	}
	if km := current(t, ctx, st); km["road"] != 30_000 {
		t.Fatalf("odometer = %v", km)
	}
	_, found, err := rides.Preview(ctx, st, owner, []parse.Ride{ride(parse.SourceFIT, "c", "2026-03-05", 10, "device:1")})
	if err != nil || len(found) != 1 || found[0].Mapping != nil {
		t.Fatalf("gears = %+v, err = %v", found, err)
	}
}

func TestImportAuto(t *testing.T) {
	ctx, st := setup(t)
	indoor := ride(parse.SourceFIT, "c", "2026-03-03", 20, "gear:1")
	indoor.Doubt = parse.Indoor
	res, err := rides.ImportAuto(ctx, st, owner, []parse.Ride{
		ride(parse.SourceFIT, "a", "2026-03-01", 40, "gear:1"), indoor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 1 || res.Unassigned != 1 || res.Skipped != 1 {
		t.Fatalf("result = %+v", res)
	}
	// The unknown gear got a mapping without bike; mapping it moves the
	// ride, and later rides follow it.
	gears, err := st.Rides().ListRideGears(ctx, owner)
	if err != nil || len(gears) != 1 || gears[0].BikeID != "" {
		t.Fatalf("gears = %+v, err = %v", gears, err)
	}
	if err := rides.SetGearBike(ctx, st, owner, gears[0].ID, "gravel", true); err != nil {
		t.Fatal(err)
	}
	res, err = rides.ImportAuto(ctx, st, owner, []parse.Ride{
		ride(parse.SourceFIT, "a", "2026-03-01", 40, "gear:1"),
		ride(parse.SourceFIT, "b", "2026-03-02", 30, "gear:1"),
		// Overlaps ride "b".
		ride(parse.SourceGPX, "d", "2026-03-02", 30, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 1 || res.Unassigned != 0 || res.Skipped != 2 {
		t.Fatalf("result = %+v", res)
	}
	if km := current(t, ctx, st); km["gravel"] != 70_000 {
		t.Fatalf("odometer = %v", km)
	}
}

func TestDeleteDiscardAll(t *testing.T) {
	ctx, st := setup(t)
	choice := []rides.GearChoice{{Source: parse.SourceFIT, Key: "gear:1", BikeID: "road", Remember: true}}
	read := []parse.Ride{
		ride(parse.SourceFIT, "a", "2026-03-01", 40, "gear:1"),
		ride(parse.SourceFIT, "b", "2026-03-02", 30, "gear:1"),
		ride(parse.SourceFIT, "c", "2026-03-03", 20, "gear:1"),
	}
	cands, _, err := rides.Preview(ctx, st, owner, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rides.Import(ctx, st, owner, items(cands), choice, ""); err != nil {
		t.Fatal(err)
	}
	list, err := st.Rides().ListRides(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	// Newest first: c, b, a.
	if err := rides.DiscardAll(ctx, st, owner, []string{list[0].ID}); err != nil {
		t.Fatal(err)
	}
	if err := rides.DeleteAll(ctx, st, owner, []string{list[1].ID, list[2].ID}); err != nil {
		t.Fatal(err)
	}
	if km := current(t, ctx, st); km["road"] != 0 {
		t.Fatalf("odometer = %v", km)
	}
	left, err := st.Rides().ListRides(ctx, owner)
	if err != nil || len(left) != 1 || !left[0].Discarded {
		t.Fatalf("rides = %+v, err = %v", left, err)
	}
	// An unknown ride rolls everything back.
	if err := rides.DeleteAll(ctx, st, owner, []string{left[0].ID, "nope"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if left, _ := st.Rides().ListRides(ctx, owner); len(left) != 1 {
		t.Fatalf("%d rides after rollback", len(left))
	}
}
