// Package values checks the values (torques, wear limits ...) of
// components, component types and steps.
package values

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

var (
	// ErrInvalid means a value has no label or no number.
	ErrInvalid = errors.New("values: label and a number required")
	// ErrRange means To is below From.
	ErrRange = errors.New("values: to below from")
	// ErrKind means a value has no valid kind.
	ErrKind = errors.New("values: invalid kind")
	// ErrLimit means a limit doesn't have exactly one bound.
	ErrLimit = errors.New("values: a limit needs exactly one bound")
)

// Prepare trims and checks vs and gives them IDs, owner and positions. A
// value without kind gets one guessed (see GuessKind), a value without key
// one derived from its label; keys are made unique within vs. An info with
// one number becomes a fixed value.
func Prepare(ownerID string, vs []store.Value) ([]store.Value, error) {
	out := make([]store.Value, 0, len(vs))
	seen := map[string]bool{}
	for i, v := range vs {
		v.Label = strings.TrimSpace(v.Label)
		v.Unit = strings.TrimSpace(v.Unit)
		v.Hint = strings.TrimSpace(v.Hint)
		if v.Label == "" || (v.From == nil && v.To == nil) {
			return nil, ErrInvalid
		}
		if v.From != nil && v.To != nil && *v.To < *v.From {
			return nil, ErrRange
		}
		if v.Kind == "" {
			v.Kind = GuessKind(v)
		}
		if !slices.Contains(store.ValueKinds, v.Kind) {
			return nil, ErrKind
		}
		if v.Kind == store.KindLimit && v.From != nil && v.To != nil {
			return nil, ErrLimit
		}
		if v.Kind == store.KindInfo {
			// A single number is the value itself, not a minimum or maximum.
			if v.From == nil {
				v.From = v.To
			} else if v.To == nil {
				v.To = v.From
			}
		}
		v.Key = strings.TrimSpace(v.Key)
		if v.Key == "" {
			v.Key = Key(v.Label)
		}
		v.Key = unique(v.Key, seen)
		v.ID = uuid.Must(uuid.NewV7()).String()
		v.OwnerID = ownerID
		v.Position = i
		out = append(out, v)
	}
	return out, nil
}

// GuessKind returns the likely kind of v: a torque for the unit Nm, a
// limit for one bound, else a target.
func GuessKind(v store.Value) string {
	switch {
	case v.Unit == "Nm":
		return store.KindTorque
	case v.From == nil || v.To == nil:
		return store.KindLimit
	default:
		return store.KindTarget
	}
}

// Key derives a key from a label: lower case, with every run of other
// characters than letters and digits replaced by "_".
func Key(label string) string {
	var b strings.Builder
	gap := false
	for _, r := range strings.ToLower(label) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if gap && b.Len() > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
			gap = false
		} else {
			gap = true
		}
	}
	return b.String()
}

// unique returns key, or key with the suffix "_2", "_3" ... if seen
// already contains it, and adds the result to seen.
func unique(key string, seen map[string]bool) string {
	k := key
	for n := 2; seen[k]; n++ {
		k = key + "_" + strconv.Itoa(n)
	}
	seen[k] = true
	return k
}

// Effective returns the values that apply to a component: the defaults of
// its type, each replaced by the component's value with the same key, then
// the component's other values.
func Effective(defaults, own []store.Value) []store.Value {
	byKey := make(map[string]store.Value, len(own))
	for _, v := range own {
		byKey[v.Key] = v
	}
	out := make([]store.Value, 0, len(defaults)+len(own))
	used := map[string]bool{}
	for _, d := range defaults {
		if v, ok := byKey[d.Key]; ok {
			out = append(out, v)
			used[d.Key] = true
			continue
		}
		out = append(out, d)
	}
	for _, v := range own {
		if !used[v.Key] {
			out = append(out, v)
		}
	}
	return out
}
