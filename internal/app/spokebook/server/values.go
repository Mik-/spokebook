package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/Mik-/spokebook/internal/app/spokebook/i18n"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/values"
)

// Values are edited as rows of repeated form fields. The form shows the
// existing rows plus blankValueRows empty ones.
const blankValueRows = 3

// defaultUnits are suggested even before they were used.
var defaultUnits = []string{"Nm", "mm", "%", "bar", "psi"}

// valueRow is one row of value fields in a form. Key is a hidden field,
// so a renamed value keeps its key.
type valueRow struct{ Key, Kind, Label, From, To, Unit, Hint string }

// blank reports whether nothing was entered in the row.
func (r valueRow) blank() bool { return strings.TrimSpace(r.Label+r.From+r.To+r.Unit+r.Hint) == "" }

// valueRows returns the value rows of form plus blank ones.
func valueRows(form url.Values) []valueRow {
	labels := form["value_label"]
	rows := make([]valueRow, 0, len(labels)+blankValueRows)
	for i, label := range labels {
		row := valueRow{
			Key: nth(form["value_key"], i), Kind: nth(form["value_kind"], i), Label: label,
			From: nth(form["value_from"], i), To: nth(form["value_to"], i), Unit: nth(form["value_unit"], i),
			Hint: nth(form["value_hint"], i),
		}
		if !row.blank() {
			rows = append(rows, row)
		}
	}
	for range blankValueRows {
		rows = append(rows, valueRow{})
	}
	return rows
}

func nth(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

// valueForm adds the fields of vs to form. A fixed info shows only "from",
// as it was entered.
func valueForm(form url.Values, vs []store.Value) url.Values {
	for _, v := range vs {
		to := v.To
		if v.Kind == store.KindInfo && v.Bounds() == store.BoundsExact {
			to = nil
		}
		form.Add("value_key", v.Key)
		form.Add("value_kind", v.Kind)
		form.Add("value_label", v.Label)
		form.Add("value_from", optValue(v.From, numberValue))
		form.Add("value_to", optValue(to, numberValue))
		form.Add("value_unit", v.Unit)
		form.Add("value_hint", v.Hint)
	}
	return form
}

func numberValue(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// parseValues reads the value rows, skipping blank ones. It returns the
// message ID of a validation error.
func parseValues(r *http.Request) ([]store.Value, string) {
	if err := r.ParseForm(); err != nil {
		return nil, "error.invalid_value"
	}
	var vs []store.Value
	for _, row := range valueRows(r.PostForm) {
		if row.blank() {
			continue
		}
		v := store.Value{Key: row.Key, Kind: row.Kind, Label: row.Label, Unit: row.Unit, Hint: row.Hint}
		var ok bool
		if v.From, ok = optional(row.From, parseNumber); !ok {
			return nil, "error.invalid_value"
		}
		if v.To, ok = optional(row.To, parseNumber); !ok {
			return nil, "error.invalid_value"
		}
		vs = append(vs, v)
	}
	return vs, ""
}

// valueErrorMessage maps validation errors of the values package.
func valueErrorMessage(l *i18n.Localizer, err error) (string, bool) {
	switch {
	case errors.Is(err, values.ErrInvalid):
		return l.T("error.invalid_value"), true
	case errors.Is(err, values.ErrRange):
		return l.T("error.invalid_range"), true
	case errors.Is(err, values.ErrKind):
		return l.T("error.invalid_value_kind"), true
	case errors.Is(err, values.ErrLimit):
		return l.T("error.limit_bounds"), true
	}
	return "", false
}

// formatValue shows a value as "5 Nm", "5–6 Nm", "min. 1.2 mm" or "max. 0.5 %".
func formatValue(l *i18n.Localizer, v store.Value) string {
	var s string
	switch v.Bounds() {
	case store.BoundsExact:
		s = l.Number(*v.From)
	case store.BoundsMin:
		s = l.T("value.min", l.Number(*v.From))
	case store.BoundsMax:
		s = l.T("value.max", l.Number(*v.To))
	default:
		s = l.Number(*v.From) + "–" + l.Number(*v.To)
	}
	if v.Unit != "" {
		s += " " + v.Unit
	}
	return s
}

// valueSuggestions returns the data for the datalists of the value fields.
func (s *Server) valueSuggestions(ctx context.Context) (map[string]any, error) {
	labels, err := s.store.Components().ListValueLabels(ctx, ownerID(ctx))
	if err != nil {
		return nil, err
	}
	units, err := s.store.Components().ListValueUnits(ctx, ownerID(ctx))
	if err != nil {
		return nil, err
	}
	return map[string]any{"ValueLabels": labels, "ValueUnits": unitSuggestions(units)}, nil
}

// unitSuggestions returns the used units followed by unused default units.
func unitSuggestions(used []string) []string {
	out := append([]string(nil), used...)
	for _, u := range defaultUnits {
		if !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	return out
}
