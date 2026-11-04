package catalog

import (
	"slices"
	"strings"

	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// The special filter values: steps without a level, steps of every bike.
const (
	OnDemand = "on_demand"
	NoModule = "none"
)

// Filter narrows the catalog. Empty fields match everything.
type Filter struct {
	Level    string // a level or OnDemand
	Module   string // a bike feature or NoModule
	Assembly string
	Action   string
	Query    string // part of the title or instructions, ignoring case
}

// Active reports whether the filter excludes anything.
func (f Filter) Active() bool { return f != Filter{} }

// Match reports whether s passes the filter. Title and instructions of s
// should be translated already.
func (f Filter) Match(s store.Step) bool {
	switch {
	case f.Level == OnDemand && len(s.Levels) > 0,
		f.Level != "" && f.Level != OnDemand && !slices.Contains(s.Levels, f.Level),
		f.Module == NoModule && s.Module != "",
		f.Module != "" && f.Module != NoModule && s.Module != f.Module,
		f.Assembly != "" && s.Assembly != f.Assembly,
		f.Action != "" && s.Action != f.Action:
		return false
	}
	q := strings.ToLower(strings.TrimSpace(f.Query))
	return q == "" || strings.Contains(strings.ToLower(s.Title), q) || strings.Contains(strings.ToLower(s.Instructions), q)
}

// Hinted is a module hint with the steps it applies to.
type Hinted struct {
	ID    string
	Steps []store.Step
}

// Effects is what a module changes in the catalog.
type Effects struct {
	Added   []store.Step // steps only for bikes with the feature
	Dropped []store.Step // built-in steps the feature makes unnecessary
	Hinted  []Hinted
}

// ModuleEffects returns what the feature adds to, drops from and notes on
// the steps, which are in catalog order.
func ModuleEffects(steps []store.Step, feature string) Effects {
	var e Effects
	for _, s := range steps {
		if s.Module == feature {
			e.Added = append(e.Added, s)
		}
		if slices.Contains(DroppedBy(s.BuiltinID), feature) {
			e.Dropped = append(e.Dropped, s)
		}
		for _, id := range HintsFor(s, []string{feature}) {
			i := slices.IndexFunc(e.Hinted, func(h Hinted) bool { return h.ID == id })
			if i < 0 {
				e.Hinted = append(e.Hinted, Hinted{ID: id})
				i = len(e.Hinted) - 1
			}
			e.Hinted[i].Steps = append(e.Hinted[i].Steps, s)
		}
	}
	return e
}
