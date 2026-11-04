package components_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/spokebook/spokebook/internal/app/spokebook/components"
	"github.com/spokebook/spokebook/internal/app/spokebook/odometer"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

func newComponent(typ string) store.Component {
	return store.Component{ID: uuid.NewString(), OwnerID: owner, Type: typ, CreatedAt: time.Now()}
}

func TestAddToBikeAndReplace(t *testing.T) {
	f := setup(t)
	bike := f.bikes[0]
	readings := func() int {
		list, err := f.st.Odometer().ListReadings(f.ctx, owner, bike)
		if err != nil {
			t.Fatal(err)
		}
		return len(list)
	}

	// Adding to a bike without readings records the entered odometer.
	chain := newComponent("chain")
	m := components.Mount{OwnerID: owner, BikeID: bike, At: day("2026-03-01"), DistanceM: ptr(int64(1000))}
	if err := components.AddToBike(f.ctx, f.st, chain, nil, m); err != nil {
		t.Fatal(err)
	}
	if readings() != 1 {
		t.Fatalf("readings = %d, want 1", readings())
	}

	// Replacing at the same odometer adds no reading; the old chain is kept.
	spare := newComponent("chain")
	insts, _ := f.st.Components().ListInstallations(f.ctx, owner, chain.ID)
	m.At = day("2026-03-01")
	if err := components.Replace(f.ctx, f.st, insts[0].ID, "", spare, nil, m); err != nil {
		t.Fatal(err)
	}
	if readings() != 1 {
		t.Errorf("same odometer recorded again: %d readings", readings())
	}
	if c, _ := f.st.Components().GetComponent(f.ctx, owner, chain.ID); !c.Active() {
		t.Error("old chain retired without reason")
	}

	// Replacing later with retirement records the new odometer.
	insts, _ = f.st.Components().ListInstallations(f.ctx, owner, spare.ID)
	next := newComponent("chain")
	m = components.Mount{OwnerID: owner, At: day("2026-08-01"), DistanceM: ptr(int64(4000))}
	if err := components.Replace(f.ctx, f.st, insts[0].ID, "worn", next, nil, m); err != nil {
		t.Fatal(err)
	}
	if readings() != 2 {
		t.Errorf("readings = %d, want 2", readings())
	}
	old, _ := f.st.Components().GetComponent(f.ctx, owner, spare.ID)
	if old.Active() || old.RetireReason != "worn" {
		t.Errorf("old component not retired: %+v", old)
	}
	oldInsts, _ := f.st.Components().ListInstallations(f.ctx, owner, spare.ID)
	if oldInsts[0].Open() || *oldInsts[0].RemovedM != 4000 {
		t.Errorf("old installation not closed at 4000 m: %+v", oldInsts[0])
	}
	newInsts, _ := f.st.Components().ListInstallations(f.ctx, owner, next.ID)
	if len(newInsts) != 1 || !newInsts[0].Open() || newInsts[0].BikeID != bike || newInsts[0].InstalledM != 4000 {
		t.Errorf("new component not installed: %+v", newInsts)
	}

	// An odometer below the recorded one fails and changes nothing.
	var low *odometer.TooLowError
	err := components.Replace(f.ctx, f.st, newInsts[0].ID, "", newComponent("chain"), nil,
		components.Mount{OwnerID: owner, At: day("2026-09-01"), DistanceM: ptr(int64(3000))})
	if !errors.As(err, &low) {
		t.Fatalf("lower odometer: %v", err)
	}
	if got, _ := f.st.Components().ListInstallations(f.ctx, owner, next.ID); !got[0].Open() {
		t.Error("failed replacement removed the component")
	}
	comps, _ := f.st.Components().ListComponents(f.ctx, owner)
	if len(comps) != 4 { // fixture wheel + chain, spare, next
		t.Errorf("failed replacement left a component behind: %d components", len(comps))
	}
}

// A self-built bike gets components before its first ride: without
// readings, the odometer at going into service is used.
func TestMountBeforeFirstRide(t *testing.T) {
	f := setup(t)
	bike, err := f.st.Bikes().CreateBike(f.ctx, store.Bike{
		ID: uuid.NewString(), OwnerID: owner, Name: "Aufbau", Origin: "self_built",
		InServiceM: ptr(int64(0)), CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	inst, err := components.Install(f.ctx, f.st, store.Installation{
		ID: uuid.NewString(), OwnerID: owner, ComponentID: f.comp, BikeID: bike.ID,
		InstalledAt: day("2026-02-01"), CreatedAt: time.Now(),
	}, nil, false)
	if err != nil || inst.InstalledM != 0 {
		t.Fatalf("install before first ride: %+v, %v", inst, err)
	}
	// Entering the known odometer (0) adds no reading.
	if err := components.AddToBike(f.ctx, f.st, newComponent("chain"), nil,
		components.Mount{OwnerID: owner, BikeID: bike.ID, At: day("2026-02-01"), DistanceM: ptr(int64(0))}); err != nil {
		t.Fatal(err)
	}
	if list, _ := f.st.Odometer().ListReadings(f.ctx, owner, bike.ID); len(list) != 0 {
		t.Errorf("readings = %d, want 0", len(list))
	}
}

// A second component of the same type at the same position needs
// confirmation; other positions, a replacement and type "other" don't.
func TestDuplicate(t *testing.T) {
	f := setup(t)
	bike := f.bikes[0]
	mount := func(c store.Component, position, date string, allow bool) error {
		return components.AddToBike(f.ctx, f.st, c, nil, components.Mount{
			OwnerID: owner, BikeID: bike, Position: position, At: day(date), DistanceM: ptr(int64(0)), AllowDuplicate: allow,
		})
	}
	front := newComponent("brake_pads")
	if err := mount(front, "front", "2026-01-01", false); err != nil {
		t.Fatal(err)
	}
	if err := mount(newComponent("brake_pads"), "rear", "2026-01-01", false); err != nil {
		t.Errorf("other position: %v", err)
	}
	var dup *components.DuplicateError
	if err := mount(newComponent("brake_pads"), "front", "2026-02-01", false); !errors.As(err, &dup) || dup.ComponentID != front.ID {
		t.Errorf("same position: want DuplicateError for %s, got %v", front.ID, err)
	}
	if err := mount(newComponent("brake_pads"), "front", "2026-02-01", true); err != nil {
		t.Errorf("confirmed duplicate: %v", err)
	}
	for range 2 {
		if err := mount(newComponent("other"), "", "2026-02-01", false); err != nil {
			t.Errorf("type other: %v", err)
		}
	}

	insts, _ := f.st.Components().ListInstallations(f.ctx, owner, front.ID)
	err := components.Replace(f.ctx, f.st, insts[0].ID, "", newComponent("brake_pads"), nil,
		components.Mount{OwnerID: owner, Position: "front", At: day("2026-03-01")})
	if !errors.As(err, &dup) {
		t.Errorf("replace with the other front pads still installed: %v", err)
	}
}

// A spare takes the place of the installed component; a failed attempt
// changes nothing.
func TestReplaceWithSpare(t *testing.T) {
	f := setup(t)
	bike := f.bikes[0]
	chain := newComponent("chain")
	if err := components.AddToBike(f.ctx, f.st, chain, nil, components.Mount{
		OwnerID: owner, BikeID: bike, At: day("2026-03-01"), DistanceM: ptr(int64(1000)),
	}); err != nil {
		t.Fatal(err)
	}
	spare := newComponent("chain")
	if _, err := f.st.Components().CreateComponent(f.ctx, spare); err != nil {
		t.Fatal(err)
	}
	open := func(id string) []store.Installation {
		list, _ := f.st.Components().ListInstallations(f.ctx, owner, id)
		var res []store.Installation
		for _, i := range list {
			if i.Open() {
				res = append(res, i)
			}
		}
		return res
	}
	m := components.Mount{OwnerID: owner, At: day("2026-06-01"), DistanceM: ptr(int64(3000))}

	// The installed component itself can't be the replacement.
	if err := components.ReplaceWithSpare(f.ctx, f.st, open(chain.ID)[0].ID, "", chain.ID, m); !errors.Is(err, components.ErrAlreadyInstalled) {
		t.Errorf("same component: want ErrAlreadyInstalled, got %v", err)
	}
	// A retired spare fails, and the old chain stays installed.
	retired := newComponent("chain")
	if _, err := f.st.Components().CreateComponent(f.ctx, retired); err != nil {
		t.Fatal(err)
	}
	if err := components.RetireComponent(f.ctx, f.st, owner, retired.ID, day("2026-02-01"), "worn", nil); err != nil {
		t.Fatal(err)
	}
	if err := components.ReplaceWithSpare(f.ctx, f.st, open(chain.ID)[0].ID, "worn", retired.ID, m); !errors.Is(err, components.ErrRetired) {
		t.Errorf("retired spare: want ErrRetired, got %v", err)
	}
	if len(open(chain.ID)) != 1 {
		t.Fatal("failed replacement removed the chain")
	}

	if err := components.ReplaceWithSpare(f.ctx, f.st, open(chain.ID)[0].ID, "worn", spare.ID, m); err != nil {
		t.Fatal(err)
	}
	if old, _ := f.st.Components().GetComponent(f.ctx, owner, chain.ID); old.Active() || len(open(chain.ID)) != 0 {
		t.Errorf("old chain not removed and retired: %+v", old)
	}
	if got := open(spare.ID); len(got) != 1 || got[0].BikeID != bike || got[0].InstalledM != 3000 {
		t.Errorf("spare not installed at 3000 m: %+v", got)
	}
}
