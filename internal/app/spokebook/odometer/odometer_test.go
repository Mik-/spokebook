package odometer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/spokebook/spokebook/internal/app/spokebook/odometer"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
	"github.com/spokebook/spokebook/internal/app/spokebook/store/sqlite"
)

func TestRecordMonotonic(t *testing.T) {
	ctx := context.Background()
	st, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	bike, err := st.Bikes().CreateBike(ctx, store.Bike{ID: uuid.NewString(), OwnerID: "o1", Name: "b", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	record := func(date string, m int64) error {
		at, err := time.Parse(time.DateOnly, date)
		if err != nil {
			t.Fatal(err)
		}
		_, err = odometer.Record(ctx, st, store.OdometerReading{
			ID: uuid.NewString(), OwnerID: "o1", BikeID: bike.ID, RecordedAt: at,
			DistanceM: m, Source: store.SourceManual, CreatedAt: time.Now(),
		})
		return err
	}

	// Existing history: 1000 m on 05-01, 3000 m on 09-01.
	for _, r := range []struct {
		date string
		m    int64
	}{{"2026-05-01", 1000}, {"2026-09-01", 3000}} {
		if err := record(r.date, r.m); err != nil {
			t.Fatal(err)
		}
	}

	var low *odometer.TooLowError
	var high *odometer.TooHighError
	tests := []struct {
		name    string
		date    string
		m       int64
		wantErr any
	}{
		{"before first, lower", "2026-04-01", 500, nil},
		{"before first, higher than next", "2026-04-02", 1500, &high},
		{"between, fits", "2026-07-01", 2000, nil},
		{"between, below prev", "2026-08-01", 1999, &low},
		{"between, above next", "2026-08-01", 3001, &high},
		{"same day, equal", "2026-09-01", 3000, nil},
		{"same day, lower", "2026-09-01", 2999, &low},
		{"after last, equal", "2026-10-01", 3000, nil},
		{"after last, lower", "2026-10-02", 2500, &low},
		{"after last, higher", "2026-10-25", 5000, nil},
	}
	for _, tt := range tests {
		err := record(tt.date, tt.m)
		switch want := tt.wantErr.(type) {
		case nil:
			if err != nil {
				t.Errorf("%s: unexpected error %v", tt.name, err)
			}
		case **odometer.TooLowError:
			if !errors.As(err, want) {
				t.Errorf("%s: want TooLowError, got %v", tt.name, err)
			}
		case **odometer.TooHighError:
			if !errors.As(err, want) {
				t.Errorf("%s: want TooHighError, got %v", tt.name, err)
			}
		}
	}
	if low == nil || low.Prev.DistanceM != 3000 {
		t.Errorf("last TooLowError should name the 3000 m reading, got %+v", low)
	}
}

func TestUpdateExcludesItself(t *testing.T) {
	ctx := context.Background()
	st, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	bike, err := st.Bikes().CreateBike(ctx, store.Bike{ID: uuid.NewString(), OwnerID: "o1", Name: "b", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	var readings []store.OdometerReading
	for i, date := range []string{"2026-01-01", "2026-02-01", "2026-03-01"} {
		at, _ := time.Parse(time.DateOnly, date)
		r, err := odometer.Record(ctx, st, store.OdometerReading{
			ID: uuid.NewString(), OwnerID: "o1", BikeID: bike.ID, RecordedAt: at,
			DistanceM: int64(i+1) * 1000, Source: store.SourceManual, CreatedAt: time.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		readings = append(readings, r)
	}
	mid := readings[1]

	// Lowering the middle reading to 1500 fits; it must not collide with itself.
	mid.DistanceM = 1500
	if err := odometer.Update(ctx, st, mid); err != nil {
		t.Fatalf("update within bounds: %v", err)
	}
	mid.DistanceM = 3500
	var high *odometer.TooHighError
	if err := odometer.Update(ctx, st, mid); !errors.As(err, &high) || high.Next.ID != readings[2].ID {
		t.Fatalf("update above next: %v", err)
	}
	// Moving the first reading after the last one must respect the last.
	first := readings[0]
	first.RecordedAt = first.RecordedAt.AddDate(0, 6, 0)
	var low *odometer.TooLowError
	if err := odometer.Update(ctx, st, first); !errors.As(err, &low) {
		t.Fatalf("move behind higher reading: %v", err)
	}
}

// A used bike taken over at 5000 m on 03-01 has no readings before that
// date and none below that odometer.
func TestRecordAfterGoingIntoService(t *testing.T) {
	ctx := context.Background()
	st, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	since, _ := time.Parse(time.DateOnly, "2026-03-01")
	m := int64(5000)
	bike, err := st.Bikes().CreateBike(ctx, store.Bike{
		ID: uuid.NewString(), OwnerID: "o1", Name: "b", InServiceAt: &since, InServiceM: &m, CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	record := func(at time.Time, m int64) error {
		_, err := odometer.Record(ctx, st, store.OdometerReading{
			ID: uuid.NewString(), OwnerID: "o1", BikeID: bike.ID, RecordedAt: at,
			DistanceM: m, Source: store.SourceManual, CreatedAt: time.Now(),
		})
		return err
	}

	if err := record(since.AddDate(0, 0, -1), 6000); !errors.Is(err, odometer.ErrBeforeService) {
		t.Errorf("before going into service: %v", err)
	}
	var below *odometer.BelowServiceError
	if err := record(since.AddDate(0, 1, 0), 4999); !errors.As(err, &below) || below.InServiceM != 5000 {
		t.Errorf("below odometer at going into service: %v", err)
	}
	if err := record(since, 5000); err != nil {
		t.Errorf("at going into service: %v", err)
	}
}
