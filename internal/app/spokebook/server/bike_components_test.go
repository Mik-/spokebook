package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestAddToBikeAndReplace(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Gravel")
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-01-01"}, "km": {"1200"}})

	// Add a tire directly on the bike page; the odometer is optional.
	_, _, body := c.do("GET", bikePath, nil)
	mustContain(t, body, `href="`+bikePath+`/components" role="button"`)
	if strings.Contains(body, `href="`+bikePath+`/components/new"`) {
		t.Error("bike page links to adding a component")
	}
	_, _, body = c.do("GET", bikePath+"/components", nil)
	mustContain(t, body, `href="`+bikePath+`/components/new" role="button"`)
	_, _, body = c.do("GET", bikePath+"/components/new", nil)
	mustContain(t, body, "<h1>Komponente hinzufügen: Gravel</h1>", `name="km" min="0" step="0.1" inputmode="decimal" value="">`)

	status, _, body := c.do("POST", bikePath+"/components", url.Values{
		"date": {"2026-02-01"}, "km": {"1000"}, "type": {"tire"}, "manufacturer": {"Schwalbe"},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("odometer below recorded: status %d", status)
	}
	mustContain(t, body, "nicht unter 1.200 km liegen", `value="Schwalbe"`)

	status, path, body := c.do("POST", bikePath+"/components", url.Values{
		"date": {"2026-02-01"}, "km": {"1500"}, "position": {"front"},
		"type": {"tire"}, "manufacturer": {"Schwalbe"}, "model": {"G-One"}, "dealer": {"Rose"},
		"purchased_at": {"2026-01-20"}, "price": {"45"}, "currency": {"EUR"}, "max_km": {"4000"},
	})
	if status != http.StatusOK || path != bikePath+"/components" {
		t.Fatalf("add to bike: %d %s", status, path)
	}
	mustContain(t, body, "Schwalbe G-One", "Reifen · vorn", "01.02.2026", "1.500 km")

	// Replace it: the form is prefilled from the old tire without its dates.
	compPath := firstLink(t, body, "/components/")
	_, _, body = c.do("GET", compPath+"/replace", nil)
	mustContain(t, body, "<h1>Austauschen: Schwalbe G-One</h1>", `value="Schwalbe"`, `value="G-One"`,
		`value="Rose"`, `value="45.00"`, `value="4000"`, `<option value="front" selected>`)
	if strings.Contains(body, `value="2026-01-20"`) {
		t.Error("purchase date of the old tire copied")
	}

	status, path, body = c.do("POST", compPath+"/replace", url.Values{
		"date": {"2026-09-01"}, "km": {"5600"}, "position": {"front"}, "retire": {"1"}, "reason": {"worn"},
		"type": {"tire"}, "manufacturer": {"Schwalbe"}, "model": {"G-One RS"}, "currency": {"EUR"},
	})
	if status != http.StatusOK || path != bikePath+"/components" {
		t.Fatalf("replace: %d %s", status, path)
	}
	// The old tire moved to "previously installed", marked as retired.
	mustContain(t, body, "Schwalbe G-One RS", "Früher verbaut (1)", "ausgemustert 01.09.2026, verschlissen", "4.100 km")
	if i := strings.Index(body, "Früher verbaut"); strings.Index(body, `>Schwalbe G-One</a>`) < i {
		t.Error("old tire still listed as installed")
	}
	_, _, body = c.do("GET", bikePath, nil)
	mustContain(t, body, `<small>Kilometerstand</small><strong>5.600 km</strong>`, "1 Komponente verbaut.")

	// Without an odometer, the reading at the date is used.
	_, _, body = c.do("POST", bikePath+"/components", url.Values{
		"date": {"2026-09-02"}, "position": {"rear"}, "type": {"tire"}, "manufacturer": {"Conti"},
	})
	mustContain(t, body, "Conti", "Reifen · hinten", "02.09.2026<br><small>5.600 km</small>")

	_, _, body = c.do("GET", compPath, nil)
	mustContain(t, body, "Ausgemustert am 01.09.2026: verschlissen.", "4.100 km")
}

func TestInstallSpareAndDuplicate(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Gravel")
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-01-01"}, "km": {"100"}})

	_, _, body := c.do("GET", bikePath+"/components/install", nil)
	mustContain(t, body, "<h1>Ersatzteil einbauen: Gravel</h1>", "Es gibt kein Ersatzteil")

	c.do("POST", bikePath+"/components", url.Values{
		"date": {"2026-01-01"}, "position": {"front"}, "type": {"brake_pads"}, "manufacturer": {"Shimano"}, "model": {"L05A"},
	})
	_, sparePath, _ := c.do("POST", "/components", url.Values{"type": {"brake_pads"}, "manufacturer": {"Swissstop"}, "model": {"Disc 34"}})
	spareID := strings.TrimPrefix(sparePath, "/components/")

	// The spare is offered; installing it at the same position warns.
	_, _, body = c.do("GET", bikePath+"/components", nil)
	mustContain(t, body, `href="`+bikePath+`/components/install" role="button" class="outline"`)
	_, _, body = c.do("GET", bikePath+"/components/install", nil)
	mustContain(t, body, `<option value="`+spareID+`">Bremsbeläge: Swissstop Disc 34</option>`)
	if strings.Contains(body, "L05A") {
		t.Error("installed component offered as spare")
	}
	form := url.Values{"component": {spareID}, "date": {"2026-02-01"}, "position": {"front"}}
	status, _, body := c.do("POST", bikePath+"/components/install", form)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate: status %d", status)
	}
	mustContain(t, body, "„Shimano L05A“ ist an dieser Position bereits verbaut.", `name="allow_duplicate"`, "Stattdessen austauschen")

	form.Set("allow_duplicate", "1")
	status, path, body := c.do("POST", bikePath+"/components/install", form)
	if status != http.StatusOK || path != bikePath+"/components" {
		t.Fatalf("confirmed duplicate: %d %s", status, path)
	}
	mustContain(t, body, "Swissstop Disc 34", "01.02.2026<br><small>100 km</small>")

	// The warning also comes when installing from the component page.
	_, otherPath, _ := c.do("POST", "/components", url.Values{"type": {"brake_pads"}, "manufacturer": {"Galfer"}})
	status, _, body = c.do("POST", otherPath+"/install", url.Values{
		"bike": {strings.TrimPrefix(bikePath, "/bikes/")}, "date": {"2026-02-01"}, "position": {"front"},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate from component page: status %d", status)
	}
	mustContain(t, body, "ist an dieser Position bereits verbaut.", `name="allow_duplicate"`)
}

// A component opened from a bike's component list leads back to that list,
// also after editing or deleting it.
func TestComponentFromBike(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Gravel")
	bikeID := strings.TrimPrefix(bikePath, "/bikes/")
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-01-01"}, "km": {"0"}})
	_, _, body := c.do("POST", bikePath+"/components", url.Values{"date": {"2026-01-01"}, "km": {"0"}, "type": {"chain"}})
	compPath := firstLink(t, body, "/components/")
	q := "?bike=" + bikeID
	mustContain(t, body, `href="`+compPath+q+`"`)

	back := `<li><a href="` + bikePath + `/components">Verbaute Komponenten</a></li>`
	_, _, body = c.do("GET", compPath+q, nil)
	mustContain(t, body, `<li><a href="`+bikePath+`">Gravel</a></li>`, back, `href="`+compPath+`/edit`+q+`"`)
	_, _, body = c.do("GET", compPath, nil)
	if strings.Contains(body, back) {
		t.Error("bike context without parameter")
	}
	_, _, body = c.do("GET", compPath+"?bike=unknown", nil)
	if strings.Contains(body, back) {
		t.Error("bike context for an unknown bike")
	}

	_, _, body = c.do("GET", compPath+"/edit"+q, nil)
	mustContain(t, body, back, `action="`+compPath+q+`"`, `href="`+compPath+`/delete`+q+`"`)
	_, _, body = c.do("POST", compPath+q, url.Values{"type": {"chain"}, "manufacturer": {"KMC"}, "currency": {"EUR"}})
	mustContain(t, body, "<h1>KMC </h1>", back)

	_, _, body = c.do("GET", compPath+"/delete"+q, nil)
	mustContain(t, body, `action="`+compPath+`/delete`+q+`"`)
	status, path, _ := c.do("POST", compPath+"/delete"+q, url.Values{})
	if status != http.StatusOK || path != bikePath+"/components" {
		t.Fatalf("delete: %d %s", status, path)
	}
}

func TestReplaceWithSpare(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Gravel")
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-01-01"}, "km": {"100"}})
	_, _, body := c.do("POST", bikePath+"/components", url.Values{
		"date": {"2026-01-01"}, "position": {"rear"}, "type": {"brake_pads"}, "manufacturer": {"Shimano"}, "model": {"L05A"},
	})
	compPath := firstLink(t, body, "/components/")

	// Without a spare, the form offers no choice.
	_, _, body = c.do("GET", compPath+"/replace", nil)
	if strings.Contains(body, "?existing=1") {
		t.Error("existing component offered without spares")
	}

	_, chainPath, _ := c.do("POST", "/components", url.Values{"type": {"chain"}, "manufacturer": {"KMC"}})
	_, sparePath, _ := c.do("POST", "/components", url.Values{"type": {"brake_pads"}, "manufacturer": {"Swissstop"}})
	spareID := strings.TrimPrefix(sparePath, "/components/")
	_, _, body = c.do("GET", compPath+"/replace", nil)
	mustContain(t, body, `<a href="`+compPath+`/replace?existing=1">Stattdessen vorhandene Komponente verwenden`)

	// Spares of the same type come first; the fields of a new component are gone.
	_, _, body = c.do("GET", compPath+"/replace?existing=1", nil)
	mustContain(t, body, `<input type="hidden" name="existing" value="1">`, `<a href="`+compPath+`/replace">Stattdessen neue Komponente anlegen`)
	if i, j := strings.Index(body, "Swissstop"), strings.Index(body, "KMC"); i < 0 || j < 0 || i > j {
		t.Error("spare of the same type not first")
	}
	if strings.Contains(body, `name="manufacturer"`) {
		t.Error("fields of a new component shown")
	}

	status, _, body := c.do("POST", compPath+"/replace", url.Values{"existing": {"1"}, "date": {"2026-03-01"}, "position": {"rear"}})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("no component chosen: status %d", status)
	}
	mustContain(t, body, "Bitte aus der Liste wählen.", `<input type="hidden" name="existing" value="1">`)

	status, path, body := c.do("POST", compPath+"/replace", url.Values{
		"existing": {"1"}, "component": {spareID}, "date": {"2026-03-01"}, "position": {"rear"},
	})
	if status != http.StatusOK || path != bikePath+"/components" {
		t.Fatalf("replace with spare: %d %s", status, path)
	}
	mustContain(t, body, "Swissstop", "Früher verbaut (1)")
	if strings.Contains(body, strings.TrimPrefix(chainPath, "/components/")) {
		t.Error("chain installed")
	}
}
