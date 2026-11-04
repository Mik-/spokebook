package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Mik-/spokebook/internal/app/spokebook/components"
	"github.com/Mik-/spokebook/internal/app/spokebook/i18n"
	"github.com/Mik-/spokebook/web"
)

// firstLink returns the path of the first href in body that starts with prefix,
// skipping links to "new" forms.
func firstLink(t *testing.T, body, prefix string) string {
	t.Helper()
	for rest := body; ; {
		i := strings.Index(rest, `href="`+prefix)
		if i < 0 {
			t.Fatalf("no link to %s", prefix)
		}
		link := rest[i+len(`href="`):]
		link = link[:strings.Index(link, `"`)]
		link, _, _ = strings.Cut(link, "?")
		if !strings.HasSuffix(link, "/new") {
			return link
		}
		rest = rest[i+1:]
	}
}

func TestComponentTypesTranslated(t *testing.T) {
	b, err := i18n.Load(web.Locales, "locales")
	if err != nil {
		t.Fatal(err)
	}
	keys := b.Keys(i18n.Fallback)
	for _, typ := range components.BuiltinTypes {
		id := "component.type." + typ
		found := false
		for _, k := range keys {
			found = found || k == id
		}
		if !found {
			t.Errorf("missing message %q", id)
		}
	}
}

func TestComponentLifecycle(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Rennrad")
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-03-01"}, "km": {"1000"}})

	// Validation keeps the entered values.
	status, _, body := c.do("POST", "/components", url.Values{
		"type": {"chain"}, "manufacturer": {"Shimano"}, "price": {"abc"}, "currency": {"EUR"},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid price: status %d", status)
	}
	mustContain(t, body, "Bitte einen gültigen Preis angeben.", `value="Shimano"`, `<option value="chain" selected>`)

	status, compPath, body := c.do("POST", "/components", url.Values{
		"type": {"chain"}, "manufacturer": {"Shimano"}, "model": {"CN-M8100"},
		"price": {"39,90"}, "currency": {"eur"}, "max_km": {"3000"},
	})
	if status != http.StatusOK || !strings.HasPrefix(compPath, "/components/") {
		t.Fatalf("create: status %d, path %s", status, compPath)
	}
	mustContain(t, body, "Shimano CN-M8100", "39,90 EUR", "3.000 km", "Nicht verbaut.")

	// Install without km uses the bike's current odometer.
	bikeID := strings.TrimPrefix(bikePath, "/bikes/")
	_, _, body = c.do("POST", compPath+"/install", url.Values{"bike": {bikeID}, "date": {"2026-03-01"}})
	mustContain(t, body, "Verbaut an Rennrad seit 01.03.2026 bei 1.000 km.")

	status, _, _ = c.do("POST", compPath+"/install", url.Values{"bike": {bikeID}, "date": {"2026-03-02"}})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("second install: status %d", status)
	}

	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-09-01"}, "km": {"4000"}})
	_, _, body = c.do("GET", bikePath, nil)
	mustContain(t, body, "Verbaute Komponenten", "Shimano CN-M8100", "3.000 km · 100 % der Grenze", "Grenze erreicht", `class="alert"`)

	i := strings.Index(body, `name="installation"`)
	if i >= 0 {
		t.Fatal("bike page should not contain the removal form")
	}
	_, _, body = c.do("GET", compPath, nil)
	mustContain(t, body, `href="`+compPath+`/remove" role="button"`)
	_, _, body = c.do("GET", compPath+"/remove", nil)
	mustContain(t, body, "Verbaut an Rennrad seit 01.03.2026")
	i = strings.Index(body, `name="installation" value="`)
	instID := body[i+len(`name="installation" value="`):]
	instID = instID[:strings.Index(instID, `"`)]

	status, _, body = c.do("POST", compPath+"/remove", url.Values{"installation": {instID}, "date": {"2026-02-01"}})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("removal before installation: status %d", status)
	}
	mustContain(t, body, "Der Ausbau kann nicht vor dem Einbau liegen", "<h1>Ausbauen: Shimano CN-M8100</h1>")

	_, _, body = c.do("POST", compPath+"/remove", url.Values{"installation": {instID}, "date": {"2026-09-01"}})
	mustContain(t, body, "Nicht verbaut.", "Verlauf", "01.09.2026", "4.000 km")

	_, _, body = c.do("GET", "/components", nil)
	mustContain(t, body, "Shimano CN-M8100", "Kette", "3.000 km")
}

func TestOwnTypeAndPosition(t *testing.T) {
	c := newClient(t)
	bikeID := strings.TrimPrefix(c.createBike("Gravel"), "/bikes/")

	_, _, body := c.do("POST", "/component-types", url.Values{"name": {"Pedale"}})
	mustContain(t, body, `value="Pedale"`)
	// The rename form of "Pedale" holds its ID.
	form := body[:strings.Index(body, `value="Pedale"`)]
	i := strings.LastIndex(form, `action="/component-types/`)
	typeID := form[i+len(`action="/component-types/`):]
	typeID = typeID[:strings.Index(typeID, `"`)]

	// Types are sorted by their translated name.
	_, _, body = c.do("GET", "/components/new", nil)
	if strings.Index(body, ">Bremsbeläge<") > strings.Index(body, ">Pedale<") {
		t.Error("types not sorted by name")
	}

	_, compPath, _ := c.do("POST", "/components", url.Values{"type": {typeID}, "manufacturer": {"Shimano"}, "model": {"PD-M520"}})
	_, _, body = c.do("POST", compPath+"/install", url.Values{
		"bike": {bikeID}, "position": {"rear"}, "date": {"2026-10-01"}, "km": {"100"},
	})
	mustContain(t, body, "Verbaut an Gravel seit 01.10.2026 bei 100 km. (hinten)", "Pedale")

	status, _, body := c.do("POST", "/component-types/"+typeID+"/delete", url.Values{})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("delete used type: status %d", status)
	}
	mustContain(t, body, "Dieser Typ wird von Komponenten verwendet")

	// A deleted type is listed below the types and can be restored.
	c.do("POST", "/component-types/kickstand/delete", url.Values{})
	_, _, body = c.do("GET", "/components/new", nil)
	mustNotContain(t, body, ">Ständer<")
	_, _, body = c.do("GET", "/component-types", nil)
	mustContain(t, body, "Gelöschte Typen (1)", `action="/component-types/kickstand/restore"`)
	_, _, body = c.do("POST", "/component-types/kickstand/restore", url.Values{})
	mustNotContain(t, body, "Gelöschte Typen")
	_, _, body = c.do("GET", "/components/new", nil)
	mustContain(t, body, ">Ständer<")
	if status, _, _ := c.do("POST", "/component-types/kickstand/restore", url.Values{}); status != http.StatusNotFound {
		t.Errorf("restore visible type: status %d", status)
	}
}

func TestComponentFilter(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Gravel")
	bikeID := strings.TrimPrefix(bikePath, "/bikes/")
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-01-01"}, "km": {"100"}})
	c.do("POST", bikePath+"/components", url.Values{
		"date": {"2026-01-01"}, "type": {"brake_pads"}, "manufacturer": {"Shimano"}, "model": {"L05A"},
	})
	c.do("POST", "/components", url.Values{"type": {"tire"}, "manufacturer": {"Schwalbe"}, "model": {"G-One"}})

	// The filters offer the bikes and types that occur.
	_, _, body := c.do("GET", "/components", nil)
	mustContain(t, body, `<option value="`+bikeID+`">Gravel</option>`, `<option value="tire">Reifen</option>`, "L05A", "G-One")

	for _, tc := range []struct{ query, in, out string }{
		{"bike=" + bikeID, "L05A", "G-One"},
		{"bike=none", "G-One", "L05A"},
		{"type=tire", "G-One", "L05A"},
		{"bike=none&type=brake_pads", "Keine Komponente passt zum Filter.", "L05A"},
	} {
		_, _, body = c.do("GET", "/components?"+tc.query, nil)
		list := body[strings.Index(body, `<div id="component-list">`):]
		if !strings.Contains(list, tc.in) || strings.Contains(list, tc.out) {
			t.Errorf("%s: want %q and not %q in list", tc.query, tc.in, tc.out)
		}
	}
	_, _, body = c.do("GET", "/components?type=tire", nil)
	mustContain(t, body, `<option value="tire" selected>Reifen</option>`, "1 Komponente · ")

	// Retired components follow the filters too, by the bike they were last on.
	_, compPath, _ := c.do("POST", "/components", url.Values{"type": {"tire"}, "manufacturer": {"Continental"}, "model": {"GP5000"}})
	c.do("POST", compPath+"/install", url.Values{"bike": {bikeID}, "date": {"2026-01-01"}})
	c.do("POST", compPath+"/retire", url.Values{"date": {"2026-06-01"}, "reason": {"worn"}, "km": {"900"}})
	_, compPath, _ = c.do("POST", "/components", url.Values{"type": {"chain"}, "manufacturer": {"KMC"}, "model": {"X11"}})
	c.do("POST", compPath+"/retire", url.Values{"date": {"2026-06-01"}, "reason": {"worn"}})
	for _, tc := range []struct{ query, in, out string }{
		{"", "GP5000", ""},
		{"type=tire", "GP5000", "X11"},
		{"type=brake_pads", "", "GP5000"},
		{"bike=" + bikeID, "GP5000", "X11"},
		{"bike=none", "X11", "GP5000"},
	} {
		_, _, body = c.do("GET", "/components?"+tc.query, nil)
		if (tc.in != "" && !strings.Contains(body, tc.in)) || (tc.out != "" && strings.Contains(body, tc.out)) {
			t.Errorf("retired %s: want %q and not %q", tc.query, tc.in, tc.out)
		}
	}
}
