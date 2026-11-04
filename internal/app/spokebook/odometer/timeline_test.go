package odometer_test

import (
	"context"
	"testing"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/odometer"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
	"github.com/spokebook/spokebook/internal/app/spokebook/store/sqlite"
)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr[T any](v T) *T { return &v }

func reading(date string, m int64) store.OdometerReading {
	return store.OdometerReading{RecordedAt: day(date), DistanceM: m}
}

func ride(date string, m int64) store.DayDistance {
	return store.DayDistance{Day: day(date), DistanceM: m}
}

func TestTimeline(t *testing.T) {
	inService := store.Bike{InServiceAt: ptr(day("2026-01-01")), InServiceM: ptr(int64(100))}
	tests := []struct {
		name     string
		bike     store.Bike
		readings []store.OdometerReading
		rides    []store.DayDistance
		at       string
		want     int64
		ok       bool
	}{
		{"nothing known", store.Bike{}, nil, nil, "2026-05-01", 0, false},
		{"in service only", inService, nil, nil, "2026-05-01", 100, true},
		{"before in service", inService, nil, nil, "2025-05-01", 100, true},
		{"rides from in service", inService, nil, []store.DayDistance{ride("2026-01-01", 10), ride("2026-02-01", 20)}, "2026-03-01", 130, true},
		{
			"reading counts for its day", inService,
			[]store.OdometerReading{reading("2026-02-01", 500)},
			[]store.DayDistance{ride("2026-02-01", 20), ride("2026-02-02", 30)},
			"2026-02-01", 500, true,
		},
		{
			"rides after reading", inService,
			[]store.OdometerReading{reading("2026-02-01", 500)},
			[]store.DayDistance{ride("2026-02-01", 20), ride("2026-02-02", 30)},
			"2026-02-02", 530, true,
		},
		{
			"next reading caps rides", inService,
			[]store.OdometerReading{reading("2026-01-01", 1000), reading("2026-03-01", 1150)},
			[]store.DayDistance{ride("2026-02-01", 200)},
			"2026-02-01", 1150, true,
		},
		{
			"next reading wins", inService,
			[]store.OdometerReading{reading("2026-01-01", 1000), reading("2026-03-01", 1150)},
			[]store.DayDistance{ride("2026-02-01", 200), ride("2026-03-02", 10)},
			"2026-03-02", 1160, true,
		},
		{
			"origin from first reading minus rides",
			store.Bike{},
			[]store.OdometerReading{reading("2026-03-01", 1000)},
			[]store.DayDistance{ride("2026-02-01", 200), ride("2026-03-01", 50)},
			"2026-02-01", 950, true,
		},
		{"before first reading without rides", store.Bike{}, []store.OdometerReading{reading("2026-03-01", 1000)}, nil, "2026-02-01", 0, false},
		{"rides only start at 0", store.Bike{}, nil, []store.DayDistance{ride("2026-02-01", 200), ride("2026-03-01", 50)}, "2026-03-01", 250, true},
		{"highest reading of a day", store.Bike{}, []store.OdometerReading{reading("2026-03-01", 1000), reading("2026-03-01", 1200)}, nil, "2026-03-01", 1200, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := odometer.NewTimeline(tt.bike, tt.readings, tt.rides).At(day(tt.at))
			if got != tt.want || ok != tt.ok {
				t.Fatalf("At = %d, %v; want %d, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

// An installation without entered odometer follows rides imported later;
// an entered one stays.
func TestRefreshInstallations(t *testing.T) {
	ctx := context.Background()
	st, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = st.Bikes().CreateBike(ctx, store.Bike{
		ID: "b1", OwnerID: "o1", Name: "b", InServiceAt: ptr(day("2026-01-01")), InServiceM: ptr(int64(0)), CreatedAt: day("2026-01-01"),
	})
	must(err)
	for _, id := range []string{"c1", "c2"} {
		_, err = st.Components().CreateComponent(ctx, store.Component{ID: id, OwnerID: "o1", Type: "chain", CreatedAt: day("2026-01-01")})
		must(err)
		_, err = st.Components().AddInstallation(ctx, store.Installation{
			ID: "i" + id, OwnerID: "o1", ComponentID: id, BikeID: "b1", InstalledAt: day("2026-03-01"),
			InstalledAuto: id == "c1", CreatedAt: day("2026-03-01"),
		})
		must(err)
	}
	_, err = st.Rides().CreateRide(ctx, store.Ride{
		ID: "r1", OwnerID: "o1", BikeID: "b1", Source: "gpx", ExternalID: "x", StartedAt: day("2026-02-01"),
		EndedAt: day("2026-02-01"), RiddenOn: day("2026-02-01"), DistanceM: 40_000, CreatedAt: day("2026-03-02"),
	})
	must(err)
	must(st.WithTx(ctx, func(s store.Stores) error { return odometer.RefreshTx(ctx, s, "o1", "b1") }))

	for id, want := range map[string]int64{"ic1": 40_000, "ic2": 0} {
		i, err := st.Components().GetInstallation(ctx, "o1", id)
		must(err)
		if i.InstalledM != want {
			t.Errorf("%s: InstalledM = %d, want %d", id, i.InstalledM, want)
		}
	}
}
