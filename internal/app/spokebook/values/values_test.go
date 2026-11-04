package values_test

import (
	"errors"
	"testing"

	"github.com/spokebook/spokebook/internal/app/spokebook/store"
	"github.com/spokebook/spokebook/internal/app/spokebook/values"
)

func f(v float64) *float64 { return &v }

func TestPrepare(t *testing.T) {
	got, err := values.Prepare("o1", []store.Value{
		{Label: " Torque ", Kind: store.KindTorque, From: f(5), To: f(6), Unit: " Nm ", Hint: " paste "},
		{Label: "Pads", Kind: store.KindLimit, From: f(1.2), Unit: "mm"},
		{Label: "Chain", Kind: store.KindLimit, To: f(0.5), Unit: "%"},
		{Label: "Stem", Kind: store.KindTorque, From: f(5), To: f(5), Unit: "Nm"},
		{Label: "Pads", Kind: store.KindTarget, From: f(1), To: f(2), Unit: "mm"},
		{Key: "kept", Label: "Renamed", Kind: store.KindTarget, From: f(1)},
		{Label: "Guessed torque", From: f(5), To: f(6), Unit: "Nm"},
		{Label: "Guessed limit", From: f(1), Unit: "mm"},
		{Label: "Guessed target", From: f(2), To: f(3.5), Unit: "bar"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Label != "Torque" || got[0].Unit != "Nm" || got[0].Hint != "paste" || got[0].OwnerID != "o1" ||
		got[0].ID == "" || got[3].Position != 3 {
		t.Errorf("got %+v", got)
	}
	for i, want := range []string{store.BoundsRange, store.BoundsMin, store.BoundsMax, store.BoundsExact} {
		if b := got[i].Bounds(); b != want {
			t.Errorf("value %d: bounds %s, want %s", i, b, want)
		}
	}
	for i, want := range []string{"torque", "pads", "chain", "stem", "pads_2", "kept"} {
		if got[i].Key != want {
			t.Errorf("value %d: key %q, want %q", i, got[i].Key, want)
		}
	}

	for i, want := range []string{store.KindTorque, store.KindLimit, store.KindTarget} {
		if k := got[6+i].Kind; k != want {
			t.Errorf("value %d: kind %s, want %s", 6+i, k, want)
		}
	}

	for _, tc := range []struct {
		v    store.Value
		want error
	}{
		{store.Value{Label: "", Kind: store.KindTarget, From: f(1)}, values.ErrInvalid},
		{store.Value{Label: "x", Kind: store.KindTarget}, values.ErrInvalid},
		{store.Value{Label: "x", Kind: store.KindTarget, From: f(2), To: f(1)}, values.ErrRange},
		{store.Value{Label: "x", Kind: "min", From: f(1)}, values.ErrKind},
		{store.Value{Label: "x", Kind: store.KindLimit, From: f(1), To: f(2)}, values.ErrLimit},
	} {
		if _, err := values.Prepare("o1", []store.Value{tc.v}); !errors.Is(err, tc.want) {
			t.Errorf("%+v: got %v, want %v", tc.v, err, tc.want)
		}
	}
}

func TestPrepareInfo(t *testing.T) {
	got, err := values.Prepare("o1", []store.Value{
		{Label: "Saddle height", Kind: store.KindInfo, From: f(745), Unit: "mm"},
		{Label: "Reach", Kind: store.KindInfo, To: f(400), Unit: "mm"},
		{Label: "Tire width", Kind: store.KindInfo, From: f(28), To: f(32), Unit: "mm"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{store.BoundsExact, store.BoundsExact, store.BoundsRange} {
		if b := got[i].Bounds(); b != want {
			t.Errorf("value %d: bounds %s, want %s", i, b, want)
		}
	}
	if *got[0].To != 745 || *got[1].From != 400 || got[0].Measurable() {
		t.Errorf("got %+v", got)
	}
}

func TestKey(t *testing.T) {
	for label, want := range map[string]string{
		"Lenkerklemmung":        "lenkerklemmung",
		" Pad thickness ":       "pad_thickness",
		"Belagstärke (min.)":    "belagstärke_min",
		"Torque - stem/steerer": "torque_stem_steerer",
		"--":                    "",
	} {
		if got := values.Key(label); got != want {
			t.Errorf("Key(%q) = %q, want %q", label, got, want)
		}
	}
}

func TestEffective(t *testing.T) {
	defaults := []store.Value{{Key: "a", Label: "A default"}, {Key: "b", Label: "B default"}}
	own := []store.Value{{Key: "c", Label: "C own"}, {Key: "b", Label: "B own"}}
	var got []string
	for _, v := range values.Effective(defaults, own) {
		got = append(got, v.Label)
	}
	want := []string{"A default", "B own", "C own"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
