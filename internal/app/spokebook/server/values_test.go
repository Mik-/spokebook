package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// valueForm returns component fields with value rows; each row is label,
// from, to, unit.
func valueForm(fields url.Values, rows ...[4]string) url.Values {
	for _, r := range rows {
		fields.Add("value_label", r[0])
		fields.Add("value_from", r[1])
		fields.Add("value_to", r[2])
		fields.Add("value_unit", r[3])
	}
	return fields
}

func TestComponentValues(t *testing.T) {
	c := newClient(t)
	_, _, body := c.do("GET", "/components/new", nil)
	mustContain(t, body, `name="value_label"`, `<datalist id="value-units"><option value="Nm">`)

	status, path, body := c.do("POST", "/components", valueForm(url.Values{"type": {"shift_brake_lever"}, "manufacturer": {"Shimano"}},
		[4]string{"Drehmoment Lenkerschelle", "6", "8", "Nm"},
		[4]string{"Hebelweg", "5,5", "5.5", "mm"},
		[4]string{"Belagstärke", "1,2", "", "mm"},
		[4]string{"Spiel", "", "0,5", "mm"},
		[4]string{"", "", "", ""},
	))
	if status != http.StatusOK || !strings.HasPrefix(path, "/components/") {
		t.Fatalf("create: %d %s", status, path)
	}
	mustContain(t, body, "Drehmoment Lenkerschelle", "6–8 Nm", "5,5 mm", "min. 1,2 mm", "max. 0,5 mm")

	// The edit form shows the values in order plus blank rows.
	_, _, body = c.do("GET", path+"/edit", nil)
	mustContain(t, body, `value="Drehmoment Lenkerschelle"`, `value="5.5"`, `value="0.5"`)
	if n := strings.Count(body, `name="value_label"`); n != 7 {
		t.Errorf("edit form has %d value rows, want 7", n)
	}

	// Duplicating copies the values.
	_, _, body = c.do("GET", path+"/duplicate", nil)
	mustContain(t, body, `value="Hebelweg"`)

	// Clearing rows removes values.
	status, _, body = c.do("POST", path, valueForm(url.Values{"type": {"shift_brake_lever"}, "manufacturer": {"Shimano"}},
		[4]string{"Hebelweg", "5", "", "mm"}, [4]string{"", "", "", ""}))
	if status != http.StatusOK || strings.Contains(body, "Lenkerschelle") {
		t.Fatalf("update: %d", status)
	}
	mustContain(t, body, "min. 5 mm")

	for name, row := range map[string][4]string{
		"no number":     {"Hebelweg", "", "", "mm"},
		"no label":      {"", "5", "", "mm"},
		"bad number":    {"Hebelweg", "abc", "", "mm"},
		"to below from": {"Hebelweg", "6", "5", "mm"},
	} {
		status, _, body := c.do("POST", path, valueForm(url.Values{"type": {"shift_brake_lever"}}, row))
		if status != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d", name, status)
		}
		// The submitted row is kept.
		mustContain(t, body, `role="alert"`, `value="`+row[1]+`"`)
	}
}

func TestTypeValues(t *testing.T) {
	c := newClient(t)
	_, _, body := c.do("GET", "/component-types", nil)
	mustContain(t, body, `href="/component-types/stem/values"`)
	_, _, body = c.do("GET", "/component-types/stem/values", nil)
	mustContain(t, body, `name="value_kind"`, `name="value_hint"`)

	form := valueForm(url.Values{}, [4]string{"Lenkerklemmung", "5", "7", "Nm"}, [4]string{"Schaftklemmung", "5", "6", "Nm"})
	form["value_kind"] = []string{"", "torque"}
	form["value_hint"] = []string{"Carbonpaste", ""}
	status, path, _ := c.do("POST", "/component-types/stem/values", form)
	if status != http.StatusOK || path != "/component-types" {
		t.Fatalf("save: %d %s", status, path)
	}
	_, _, body = c.do("GET", "/component-types/stem/values", nil)
	mustContain(t, body, `value="Lenkerklemmung"`, `value="lenkerklemmung"`, `value="Carbonpaste"`, `<option value="torque" selected>`)

	// A component of the type shows the defaults, overridden by own values.
	_, path, _ = c.do("POST", "/components", valueForm(url.Values{"type": {"stem"}, "manufacturer": {"Ritchey"}},
		[4]string{"Schaftklemmung", "4", "5", "Nm"}))
	_, _, body = c.do("GET", path, nil)
	mustContain(t, body, "Lenkerklemmung <small>(", "5–7 Nm", "Carbonpaste", "4–5 Nm")
	if strings.Contains(body, "5–6 Nm") {
		t.Error("overridden default shown")
	}

	// A limit with two bounds is rejected; unknown types are not found.
	form = valueForm(url.Values{}, [4]string{"Spiel", "1", "2", "mm"})
	form["value_kind"] = []string{"limit"}
	if status, _, body := c.do("POST", "/component-types/stem/values", form); status != http.StatusUnprocessableEntity {
		t.Errorf("limit with two bounds: %d", status)
	} else {
		mustContain(t, body, `role="alert"`)
	}
	if status, _, _ := c.do("GET", "/component-types/nope/values", nil); status != http.StatusNotFound {
		t.Errorf("unknown type: %d", status)
	}
}

func TestBikeValues(t *testing.T) {
	c := newClient(t)
	bike := c.createBike("Gravel")
	_, _, body := c.do("GET", bike, nil)
	mustContain(t, body, `href="`+bike+`/values"`, "Noch keine Maße")
	_, _, body = c.do("GET", bike+"/values", nil)
	mustContain(t, body, `name="value_hint"`, `<option value="Sattelhöhe">`, `<option value="°">`)
	if strings.Contains(body, `name="value_kind"`) {
		t.Error("kind shown for bike values")
	}

	// A single number is the value itself; every value is an info.
	form := valueForm(url.Values{}, [4]string{"Sattelhöhe", "745", "", "mm"}, [4]string{"Stack", "590", "", "mm"})
	form["value_hint"] = []string{"Mitte Tretlager bis Oberkante Sattel", ""}
	status, path, _ := c.do("POST", bike+"/values", form)
	if status != http.StatusOK || path != bike {
		t.Fatalf("save: %d %s", status, path)
	}
	_, _, body = c.do("GET", bike, nil)
	mustContain(t, body, "Sattelhöhe", "745 mm", "Mitte Tretlager bis Oberkante Sattel", "590 mm")
	if strings.Contains(body, "min. 745") {
		t.Error("single number shown as minimum")
	}
	_, _, body = c.do("GET", bike+"/values", nil)
	mustContain(t, body, `value="Sattelhöhe"`, `value="sattelhöhe"`, `name="value_to" inputmode="decimal" aria-label="Bis" value=""`)

	// A row without a number is rejected; unknown bikes are not found.
	if status, _, body := c.do("POST", bike+"/values", valueForm(url.Values{}, [4]string{"Reach", "", "", "mm"})); status != http.StatusUnprocessableEntity {
		t.Errorf("value without number: %d", status)
	} else {
		mustContain(t, body, `role="alert"`)
	}
	if status, _, _ := c.do("GET", "/bikes/nope/values", nil); status != http.StatusNotFound {
		t.Errorf("unknown bike: %d", status)
	}
	if status, _, _ := c.do("POST", "/bikes/nope/values", form); status != http.StatusNotFound {
		t.Errorf("save on unknown bike: %d", status)
	}
}
