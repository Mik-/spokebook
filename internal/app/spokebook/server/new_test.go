package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Lists link to separate forms for new entries; errors stay on the form.
func TestNewForms(t *testing.T) {
	c := newClient(t)

	_, _, body := c.do("GET", "/", nil)
	mustContain(t, body, `href="/bikes/new" role="button"`)
	if strings.Contains(body, `action="/bikes"`) {
		t.Error("bike list still contains the create form")
	}
	status, _, body := c.do("POST", "/bikes", url.Values{"name": {""}, "notes": {"Notiz"}})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("empty name: status %d", status)
	}
	mustContain(t, body, "<h1>Neues Fahrrad</h1>", "Bitte einen Namen angeben.", ">Notiz</textarea>")

	bikePath := c.createBike("Gravel")
	_, _, body = c.do("GET", bikePath, nil)
	mustContain(t, body, `href="`+bikePath+`/odometer/new" role="button"`)
	_, _, body = c.do("GET", bikePath+"/odometer/new", nil)
	mustContain(t, body, "<h1>Kilometerstand erfassen</h1>", `action="`+bikePath+`/odometer"`)
	status, _, body = c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-10-25"}, "km": {"-5"}})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid km: status %d", status)
	}
	mustContain(t, body, "<h1>Kilometerstand erfassen</h1>", `value="-5"`)

	_, _, body = c.do("GET", "/components", nil)
	mustContain(t, body, `href="/components/new" role="button"`)
	_, _, body = c.do("GET", "/components/new", nil)
	mustContain(t, body, "<h1>Neue Komponente</h1>", `action="/components"`)
	if strings.Contains(body, "duplicate_of") {
		t.Error("new form is marked as duplicate")
	}

	_, _, body = c.do("GET", "/component-types", nil)
	mustContain(t, body, `href="/component-types/new" role="button"`)
	status, _, body = c.do("POST", "/component-types", url.Values{"name": {" "}})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("empty type name: status %d", status)
	}
	mustContain(t, body, "<h1>Neuer Typ</h1>", "Bitte einen Namen angeben.")
}

// Install, remove and retire start from buttons on the detail pages.
func TestActionPages(t *testing.T) {
	c := newClient(t)
	_, compPath, _ := c.do("POST", "/components", url.Values{"type": {"chain"}, "manufacturer": {"KMC"}, "model": {"X11"}})

	_, _, body := c.do("GET", compPath, nil)
	mustContain(t, body, `href="`+compPath+`/install" role="button"`, `href="`+compPath+`/retire" role="button"`)
	_, _, body = c.do("GET", compPath+"/install", nil)
	mustContain(t, body, "<h1>Einbauen: KMC X11</h1>", "Es gibt noch kein aktives Fahrrad.")

	// Not installed: the removal page leads back to the component.
	if _, path, _ := c.do("GET", compPath+"/remove", nil); path != compPath {
		t.Errorf("remove page of uninstalled component: ended at %s", path)
	}

	bikeID := strings.TrimPrefix(c.createBike("Gravel"), "/bikes/")
	_, _, body = c.do("GET", compPath+"/install", nil)
	mustContain(t, body, `<option value="`+bikeID+`">Gravel</option>`)
	status, _, body := c.do("POST", compPath+"/install", url.Values{"bike": {bikeID}, "date": {"2026-10-01"}})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("install without odometer: status %d", status)
	}
	mustContain(t, body, "<h1>Einbauen: KMC X11</h1>", "Das Rad hat zu diesem Datum noch keinen Kilometerstand.")

	_, _, body = c.do("GET", compPath+"/retire", nil)
	mustContain(t, body, "<h1>Komponente ausmustern: KMC X11</h1>", `<option value="worn">verschlissen</option>`)
	if strings.Contains(body, `name="km"`) {
		t.Error("retire form asks for km although the component is not installed")
	}
}

// The start page is an overview with odometer and warnings per bike.
func TestOverviewAndNavigation(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Gravel")
	bikeID := strings.TrimPrefix(bikePath, "/bikes/")
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-01-01"}, "km": {"0"}})
	c.do("POST", bikePath+"/components", url.Values{
		"date": {"2026-01-01"}, "km": {"0"}, "type": {"chain"}, "manufacturer": {"KMC"}, "max_km": {"100"},
	})
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-05-01"}, "km": {"250"}})

	req, _ := http.NewRequest("GET", c.srv.URL+"/", nil)
	req.Header.Set("Accept-Language", "de")
	res, err := c.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") || !strings.Contains(csp, "img-src 'self' data:") || strings.Contains(csp, "unsafe") {
		t.Errorf("CSP: %q", csp)
	}

	_, _, body := c.do("GET", "/", nil)
	mustContain(t, body, `href="/bikes/`+bikeID+`">Gravel</a></h2>`, `<p class="figure">250 km</p>`,
		`>KMC </a> <span class="badge danger">Grenze erreicht</span>`, `<a href="/" aria-current="page">`, `<span class="label">Fahrräder</span>`)
	_, _, body = c.do("GET", "/components", nil)
	mustContain(t, body, `<a href="/components" aria-current="page">`)
	if strings.Contains(body, `<a href="/" aria-current="page">`) {
		t.Error("bikes marked as current on the components page")
	}
}

// Row actions in tables are buttons like all other actions.
func TestRowActionsAreButtons(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Gravel")
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-01-01"}, "km": {"0"}})
	_, _, body := c.do("POST", bikePath+"/components", url.Values{"date": {"2026-01-01"}, "km": {"0"}, "type": {"chain"}})
	compPath := firstLink(t, body, "/components/")
	mustContain(t, body, `<a href="`+compPath+`/replace" role="button" class="outline">`)
	_, _, body = c.do("GET", bikePath, nil)
	mustContain(t, body, `/edit" role="button" class="outline">Bearbeiten</a></td>`)
	_, _, body = c.do("GET", compPath, nil)
	mustContain(t, body, `<td class="row-action"><a href="/installations/`, `/edit" role="button" class="outline">Bearbeiten</a></td>`)
}
