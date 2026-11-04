package components_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Mik-/spokebook/internal/app/spokebook/components"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

func TestRetireComponent(t *testing.T) {
	f := setup(t)
	f.reading(0, "2026-01-01", 1000)
	f.reading(0, "2026-05-01", 3000)
	if _, err := f.install(0, "2026-01-01", nil); err != nil {
		t.Fatal(err)
	}
	if err := components.RetireComponent(f.ctx, f.st, owner, f.comp, day("2026-05-01"), "melted", nil); !errors.Is(err, components.ErrInvalidReason) {
		t.Errorf("invalid reason: %v", err)
	}
	if err := components.RetireComponent(f.ctx, f.st, owner, f.comp, day("2026-05-01"), "worn", nil); err != nil {
		t.Fatal(err)
	}
	c, _ := f.st.Components().GetComponent(f.ctx, owner, f.comp)
	if c.Active() || c.RetireReason != "worn" || !c.RetiredAt.Equal(day("2026-05-01")) {
		t.Errorf("not retired: %+v", c)
	}
	if got := f.mileage(); got != 2000 {
		t.Errorf("mileage = %d, want 2000 (installation closed at retirement)", got)
	}
	if err := components.RetireComponent(f.ctx, f.st, owner, f.comp, day("2026-06-01"), "worn", nil); !errors.Is(err, components.ErrRetired) {
		t.Errorf("retire twice: %v", err)
	}
	if _, err := f.install(1, "2026-06-01", ptr(int64(0))); !errors.Is(err, components.ErrRetired) {
		t.Errorf("install retired component: %v", err)
	}
	if err := components.Reactivate(f.ctx, f.st, owner, f.comp); err != nil {
		t.Fatal(err)
	}
	if _, err := f.install(1, "2026-06-01", ptr(int64(0))); err != nil {
		t.Errorf("install after reactivation: %v", err)
	}
}

func TestRetireBike(t *testing.T) {
	f := setup(t)
	f.reading(0, "2026-01-01", 0)
	other, err := f.st.Components().CreateComponent(f.ctx, store.Component{
		ID: uuid.NewString(), OwnerID: owner, Type: "chain", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.install(0, "2026-01-01", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := components.Install(f.ctx, f.st, store.Installation{
		ID: uuid.NewString(), OwnerID: owner, ComponentID: other.ID, BikeID: f.bikes[0],
		InstalledAt: day("2026-01-01"), CreatedAt: time.Now(),
	}, nil, false); err != nil {
		t.Fatal(err)
	}

	if err := components.RetireBike(f.ctx, f.st, owner, f.bikes[0], day("2026-09-01"), "sold", ptr(int64(5000))); err != nil {
		t.Fatal(err)
	}
	insts, err := f.st.Components().ListAllInstallations(f.ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range insts {
		if i.Open() || *i.RemovedM != 5000 {
			t.Errorf("installation not closed at retirement: %+v", i)
		}
	}
	if _, err := f.install(0, "2026-10-01", ptr(int64(5000))); !errors.Is(err, components.ErrRetired) {
		t.Errorf("install on retired bike: %v", err)
	}
	if err := components.ReactivateBike(f.ctx, f.st, owner, f.bikes[0]); err != nil {
		t.Fatal(err)
	}
	if b, _ := f.st.Bikes().GetBike(f.ctx, owner, f.bikes[0]); !b.Active() {
		t.Error("bike not reactivated")
	}
}
