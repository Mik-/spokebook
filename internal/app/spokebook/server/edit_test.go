package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// valueAfter returns the value of the attribute attr following marker in body.
func valueAfter(t *testing.T, body, marker, attr string) string {
	t.Helper()
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("marker %q not found", marker)
	}
	rest := body[i:]
	j := strings.Index(rest, attr+`="`)
	if j < 0 {
		t.Fatalf("no %s after %q", attr, marker)
	}
	v := rest[j+len(attr)+2:]
	return v[:strings.Index(v, `"`)]
}

func TestEditAndDelete(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Altes Rad")
	bikeID := strings.TrimPrefix(bikePath, "/bikes/")

	// Edit the bike.
	_, _, body := c.do("GET", bikePath+"/edit", nil)
	mustContain(t, body, `value="Altes Rad"`)
	status, path, body := c.do("POST", bikePath, url.Values{"name": {"Rennrad"}, "type": {"Road"}})
	if status != http.StatusOK || path != bikePath {
		t.Fatalf("update bike: %d %s", status, path)
	}
	mustContain(t, body, "<h1>Rennrad</h1>", "Road")
	if status, _, _ := c.do("POST", bikePath, url.Values{"name": {" "}}); status != http.StatusUnprocessableEntity {
		t.Errorf("empty name: status %d", status)
	}

	// Edit a reading: the edit form is prefilled, conflicts are reported.
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-01-01"}, "km": {"100"}})
	_, _, body = c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-02-01"}, "km": {"200,5"}})
	readingEdit := valueAfter(t, body, `<td data-label="Kilometerstand">200,5 km</td>`, "href")
	_, _, body = c.do("GET", readingEdit, nil)
	mustContain(t, body, `value="2026-02-01"`, `value="200.5"`)
	readingPath := strings.TrimSuffix(readingEdit, "/edit")
	status, _, body = c.do("POST", readingPath, url.Values{"date": {"2026-02-01"}, "km": {"50"}})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("lower than previous: status %d", status)
	}
	mustContain(t, body, "nicht unter 100 km liegen")
	_, _, body = c.do("POST", readingPath, url.Values{"date": {"2026-02-01"}, "km": {"250"}})
	mustContain(t, body, "250 km")

	// Delete the reading via the confirmation page.
	_, _, body = c.do("GET", readingPath+"/delete", nil)
	mustContain(t, body, "Wirklich löschen?", "Den Kilometerstand von 250 km vom 01.02.2026 löschen?")
	_, _, body = c.do("POST", readingPath+"/delete", url.Values{})
	if strings.Contains(body, "250 km") {
		t.Error("reading not deleted")
	}

	// Component with dealer suggestion and link.
	c.do("POST", "/components", url.Values{"type": {"tire"}, "manufacturer": {"Schwalbe"}, "dealer": {"Bike-Shop Nord"}})
	_, _, body = c.do("GET", "/components/new", nil)
	mustContain(t, body,
		`<datalist id="dealers"><option value="Bike-Shop Nord"></datalist>`,
		`<datalist id="manufacturers"><option value="Schwalbe"></datalist>`)
	status, _, body = c.do("POST", "/components", url.Values{"type": {"chain"}, "url": {"javascript:alert(1)"}})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("javascript URL: status %d", status)
	}
	mustContain(t, body, "Bitte einen vollständigen Link")
	_, compPath, _ := c.do("POST", "/components", url.Values{
		"type": {"chain"}, "manufacturer": {"KMC"}, "part_number": {"X11-93"}, "url": {"https://example.com/x11.pdf"},
	})
	_, _, body = c.do("GET", compPath, nil)
	mustContain(t, body, "<dt>Artikelnummer</dt><dd>X11-93</dd>")
	_, _, body = c.do("GET", compPath+"/edit", nil)
	mustContain(t, body, `value="KMC"`, `value="X11-93"`, `value="https://example.com/x11.pdf"`, `<option value="chain" selected>`)
	_, _, body = c.do("POST", compPath, url.Values{"type": {"chain"}, "manufacturer": {"KMC"}, "model": {"X11"}, "price": {"30"}, "currency": {"EUR"}})
	mustContain(t, body, "KMC X11", "30,00 EUR")
	if strings.Contains(body, "example.com") {
		t.Error("cleared URL still shown")
	}

	// Edit an installation, then the bike can't be deleted anymore.
	_, _, body = c.do("POST", compPath+"/install", url.Values{"bike": {bikeID}, "date": {"2026-01-01"}, "km": {"100"}})
	instEdit := firstLink(t, body, "/installations/")
	status, _, body = c.do("POST", strings.TrimSuffix(instEdit, "/edit"), url.Values{
		"bike": {bikeID}, "installed_at": {"2026-01-01"}, "installed_km": {"100"}, "removed_km": {"200"},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("incomplete removal: status %d", status)
	}
	mustContain(t, body, "Ein Kilometerstand beim Ausbau braucht ein Datum")
	_, _, body = c.do("POST", strings.TrimSuffix(instEdit, "/edit"), url.Values{
		"bike": {bikeID}, "position": {"front"}, "installed_at": {"2026-01-01"}, "installed_km": {"100"},
		"removed_at": {"2026-01-10"}, "removed_km": {"180"},
	})
	mustContain(t, body, "Nicht verbaut.", "vorn", "80 km")

	status, _, body = c.do("POST", bikePath+"/delete", url.Values{})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("delete bike with installation: status %d", status)
	}
	mustContain(t, body, "Das Fahrrad stattdessen ausmustern.")

	// Deleting the component removes its installations; then the bike goes.
	_, _, body = c.do("POST", compPath+"/delete", url.Values{})
	if strings.Contains(body, "KMC X11") {
		t.Error("component not deleted")
	}
	_, path, body = c.do("POST", bikePath+"/delete", url.Values{})
	if path != "/" || strings.Contains(body, ">Rennrad</a></h2>") {
		t.Errorf("bike not deleted: %s", path)
	}
}

func TestDuplicateComponent(t *testing.T) {
	c := newClient(t)
	bikeID := strings.TrimPrefix(c.createBike("Gravel"), "/bikes/")
	c.do("POST", "/bikes/"+bikeID+"/odometer", url.Values{"date": {"2026-01-01"}, "km": {"0"}})
	_, srcPath, _ := c.do("POST", "/components", url.Values{
		"type": {"tire"}, "manufacturer": {"Schwalbe"}, "model": {"G-One"}, "part_number": {"11654"},
		"price": {"45"}, "currency": {"EUR"}, "max_km": {"4000"},
	})
	c.do("POST", srcPath+"/install", url.Values{"bike": {bikeID}, "position": {"front"}, "date": {"2026-01-01"}})

	_, _, body := c.do("GET", srcPath+"/duplicate", nil)
	mustContain(t, body, `name="duplicate_of"`, `value="Schwalbe"`, `value="G-One"`, `value="11654"`,
		`value="45.00"`, `value="4000"`, `<option value="tire" selected>`)

	// A validation error keeps the user on the duplicate page.
	srcID := strings.TrimPrefix(srcPath, "/components/")
	status, _, body := c.do("POST", "/components", url.Values{
		"duplicate_of": {srcID}, "type": {"tire"}, "manufacturer": {"Schwalbe"}, "price": {"x"},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid price: status %d", status)
	}
	mustContain(t, body, "Duplizieren", "Bitte einen gültigen Preis angeben.", `value="x"`)

	status, newPath, body := c.do("POST", "/components", url.Values{
		"duplicate_of": {srcID}, "type": {"tire"}, "manufacturer": {"Schwalbe"}, "model": {"G-One"},
		"part_number": {"11654"}, "price": {"45"}, "currency": {"EUR"}, "max_km": {"4000"},
	})
	if status != http.StatusOK || newPath == srcPath {
		t.Fatalf("create duplicate: %d %s", status, newPath)
	}
	mustContain(t, body, "Schwalbe G-One", "11654", "Nicht verbaut.")
}

func TestBikeDetails(t *testing.T) {
	c := newClient(t)
	_, _, body := c.do("GET", "/bikes/new", nil)
	mustContain(t, body, `name="in_service_km" min="0" step="0.1" inputmode="decimal" value="0"`,
		`<option value="self_built">selbst aufgebaut</option>`)

	status, bikePath, body := c.do("POST", "/bikes", url.Values{
		"name": {"Aufbau"}, "origin": {"self_built"}, "dealer": {"Bike-Shop Nord"},
		"price": {"1999"}, "currency": {"EUR"}, "in_service_km": {"0"},
	})
	if status != http.StatusOK {
		t.Fatalf("create: %d", status)
	}
	mustContain(t, body, "<dt>Herkunft</dt><dd>selbst aufgebaut</dd>", "1.999,00 EUR", "<dt>Im Einsatz seit</dt><dd>0 km</dd>")

	// Components can be added before the first ride, at 0 km.
	_, _, body = c.do("POST", bikePath+"/components", url.Values{
		"date": {"2026-02-01"}, "type": {"frame"}, "manufacturer": {"Stanton"},
	})
	mustContain(t, body, "Stanton")
	_, _, body = c.do("GET", bikePath, nil)
	mustContain(t, body, "Noch keine Kilometerstände.")

	// The bike's dealer is suggested for components, too.
	_, _, body = c.do("GET", "/components/new", nil)
	mustContain(t, body, `<option value="Bike-Shop Nord">`)

	// Edit: prefilled, then set the date it went into service.
	_, _, body = c.do("GET", bikePath+"/edit", nil)
	mustContain(t, body, `<option value="self_built" selected>`, `value="1999.00"`)
	status, _, body = c.do("POST", bikePath, url.Values{"name": {"Aufbau"}, "origin": {"stolen"}})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("invalid origin: %d", status)
	}
	mustContain(t, body, "Bitte eine gültige Herkunft wählen.")
	_, _, body = c.do("POST", bikePath, url.Values{
		"name": {"Aufbau"}, "origin": {"self_built"}, "in_service_at": {"2026-03-01"}, "in_service_km": {"0"},
	})
	mustContain(t, body, "<dt>Im Einsatz seit</dt><dd>01.03.2026 · 0 km</dd>")

	// Readings fit the date and odometer at going into service, both ways.
	status, _, body = c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-02-28"}, "km": {"10"}})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("reading before going into service: %d", status)
	}
	mustContain(t, body, "Das Datum kann nicht vor der Inbetriebnahme des Rads liegen.")
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-04-01"}, "km": {"100"}})
	status, _, body = c.do("POST", bikePath, url.Values{"name": {"Aufbau"}, "in_service_at": {"2026-03-01"}, "in_service_km": {"200"}})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("odometer at going into service above a reading: %d", status)
	}
	mustContain(t, body, "Es gibt Kilometerstände unter diesem Stand.")
}

func TestBikeFeatures(t *testing.T) {
	c := newClient(t)
	_, _, body := c.do("GET", "/bikes/new", nil)
	mustContain(t, body, `<input type="checkbox" name="feature" value="carbon"> Carbon-Rahmen`)
	_, bikePath, body := c.do("POST", "/bikes", url.Values{"name": {"Gravel"}, "feature": {"tubeless", "carbon"}})
	mustContain(t, body, "<dt>Merkmale</dt><dd>Carbon-Rahmen, Tubeless-Reifen</dd>")
	_, _, body = c.do("GET", bikePath+"/edit", nil)
	mustContain(t, body, `value="carbon" checked>`, `value="tubeless" checked>`)
	_, _, body = c.do("POST", bikePath, url.Values{"name": {"Gravel"}, "feature": {"tubeless"}})
	mustContain(t, body, "<dd>Tubeless-Reifen</dd>")
	if status, _, _ := c.do("POST", bikePath, url.Values{"name": {"Gravel"}, "feature": {"steel"}}); status != http.StatusUnprocessableEntity {
		t.Errorf("unknown feature: %d", status)
	}
}
