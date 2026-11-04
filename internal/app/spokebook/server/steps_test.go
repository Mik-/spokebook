package server_test

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/spokebook/spokebook/internal/app/spokebook/auth"
)

// builtinStepID returns the ID of the owner's copy of a built-in step.
func builtinStepID(t *testing.T, c *client, builtin string) string {
	t.Helper()
	steps, err := c.st.Steps().ListSteps(context.Background(), auth.FirstUserID)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range steps {
		if st.BuiltinID == builtin {
			return st.ID
		}
	}
	t.Fatalf("no built-in step %q", builtin)
	return ""
}

func TestSteps(t *testing.T) {
	c := newClient(t)
	_, _, body := c.do("GET", "/steps", nil)
	mustContain(t, body, "Noch keine Wartungsschritte.", `href="/steps" aria-current="page"`)

	_, _, body = c.do("GET", "/steps/new?phase=inspection&assembly=brakes", nil)
	mustContain(t, body, `<option value="inspection" selected>Prüfen &amp; Messen</option>`,
		`<option value="brakes" selected>Bremsen</option>`, `<option value="tighten">Anziehen</option>`,
		`<input type="checkbox" name="level" value="monthly"> Monatscheck`,
		`<input type="checkbox" name="component_type" value="shift_brake_lever"> Schalt-/Bremshebel`,
		`<input type="checkbox" name="component_kind" value="torque"> Drehmoment`, `name="value_label"`,
		"<legend>Was</legend>", "<legend>Wo</legend>", "<legend>Wann</legend>", "<legend>Für welche Räder</legend>")

	for name, form := range map[string]url.Values{
		"no title":      {"title": {""}, "phase": {"inspection"}},
		"no phase":      {"title": {"x"}},
		"unknown phase": {"title": {"x"}, "phase": {"nope"}},
		"unknown level": {"title": {"x"}, "phase": {"inspection"}, "level": {"weekly"}},
		"unknown type":  {"title": {"x"}, "phase": {"inspection"}, "component_type": {"nope"}},
	} {
		if status, _, body := c.do("POST", "/steps", form); status != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d", name, status)
		} else {
			mustContain(t, body, `role="alert"`)
		}
	}

	// Saving shows the step on its own page.
	_, path, body := c.do("POST", "/steps", valueForm(url.Values{
		"title": {"Hebel prüfen"}, "instructions": {"Schelle prüfen"}, "phase": {"inspection"}, "assembly": {"brakes"},
		"action": {"check"}, "level": {"major", "monthly"}, "component_type": {"shift_brake_lever", "brake_pads"},
		"component_kind": {"torque"},
	}, [4]string{"Belagstärke", "1,2", "", "mm"}))
	hebelID := strings.TrimPrefix(path, "/steps/")
	if hebelID == path || hebelID == "" {
		t.Fatalf("after create: %s", path)
	}
	mustContain(t, body, "<h1>Hebel prüfen</h1>", "Schelle prüfen", "Prüfen &amp; Messen", "Bremsen", "Prüfen",
		`aria-label="Monatscheck, Große Inspektion"`, "min. 1,2 mm", "Grenzwert",
		"Dazu die Werte der verbauten Komponenten: Bremsbeläge, Schalt-/Bremshebel (Drehmoment)")

	// The catalog lists it in a table per phase, the levels as chips.
	_, _, body = c.do("GET", "/steps", nil)
	mustContain(t, body, "<h2>Prüfen &amp; Messen</h2>", `<th scope="rowgroup" colspan="5">Bremsen</th>`,
		`<a href="/steps/`+hebelID+`">Hebel prüfen</a>`, "1 Schritt · Mon 1 · Gro 1")
	_, _, body = c.do("GET", "/steps/"+hebelID+"/edit", nil)
	mustContain(t, body, `value="brake_pads" checked>`, `value="shift_brake_lever" checked>`, `value="chain">`,
		`value="monthly" checked>`, `value="pre_ride">`, `value="torque" checked>`, `<option value="check" selected>`)

	c.do("POST", "/steps", url.Values{"title": {"Scheiben reinigen"}, "phase": {"inspection"}, "assembly": {"brakes"}})
	c.do("POST", "/steps", url.Values{"title": {"Rad waschen"}, "phase": {"cleaning"}})
	_, _, body = c.do("POST", "/steps", url.Values{"title": {"Probefahrt"}, "phase": {"inspection"}, "assembly": {"brakes"}})
	mustContain(t, body, "Bei Bedarf")
	_, _, body = c.do("GET", "/steps", nil)
	mustContain(t, body, "<h2>Reinigung</h2>")

	// order returns the step titles in the order they appear on the page.
	order := func(body string) string {
		titles := []string{"Rad waschen", "Hebel prüfen", "Scheiben reinigen", "Probefahrt"}
		pos := func(title string) int { return strings.Index(body, `">`+title+"</a>") }
		slices.SortFunc(titles, func(a, b string) int { return pos(a) - pos(b) })
		return strings.Join(titles, "|")
	}
	if got := order(body); got != "Rad waschen|Hebel prüfen|Scheiben reinigen|Probefahrt" {
		t.Fatalf("order %s", got)
	}
	// The first step of a group can't move up, the last not down.
	if n := strings.Count(body, "/up\" hx-post"); n != 2 {
		t.Errorf("%d up buttons, want 2", n)
	}
	stepID := between(body, `<tr id="step-`, `"`, ">Probefahrt</a>")
	_, path, body = c.do("POST", "/steps/"+stepID+"/up", url.Values{})
	if path != "/steps" {
		t.Errorf("after move: %s", path)
	}
	if got := order(body); got != "Rad waschen|Hebel prüfen|Probefahrt|Scheiben reinigen" {
		t.Errorf("after up: %s", got)
	}
	// Only the phase of the moved step is expanded; htmx swaps its group.
	if n := strings.Count(body, `<details class="phase" open>`); n != 1 || strings.Index(body, `<details class="phase" open>`) > strings.Index(body, "Prüfen &amp; Messen") {
		t.Errorf("%d expanded phases", n)
	}
	mustContain(t, body, `hx-select="#group-inspection-brakes"`)

	// Editing keeps the values; moving to another phase goes to its end.
	_, _, body = c.do("GET", "/steps/"+stepID+"/edit", nil)
	mustContain(t, body, `value="Probefahrt"`, "Schritt löschen")
	_, path, body = c.do("POST", "/steps/"+stepID, valueForm(url.Values{"title": {"Probefahrt"}, "phase": {"test_ride"}},
		[4]string{"Bremsweg", "", "5", "m"}))
	if path != "/steps/"+stepID {
		t.Errorf("after update: %s", path)
	}
	mustContain(t, body, "Probefahrt &amp; Dokumentation", "max. 5 m")
	_, _, body = c.do("GET", "/steps", nil)
	mustContain(t, body, "<h2>Probefahrt &amp; Dokumentation</h2>")
	if got := order(body); got != "Rad waschen|Hebel prüfen|Scheiben reinigen|Probefahrt" {
		t.Errorf("after phase change: %s", got)
	}
	if status, _, _ := c.do("POST", "/steps/"+stepID, url.Values{"title": {"x"}, "phase": {"nope"}}); status != http.StatusUnprocessableEntity {
		t.Errorf("unknown phase on update: %d", status)
	}

	_, _, body = c.do("GET", "/steps/"+stepID+"/delete", nil)
	mustContain(t, body, "Den Schritt „Probefahrt“ löschen?")
	_, path, body = c.do("POST", "/steps/"+stepID+"/delete", url.Values{})
	if path != "/steps" || strings.Contains(body, ">Probefahrt</a>") {
		t.Errorf("after delete: %s", path)
	}
	for _, p := range []string{"/edit", ""} {
		if status, _, _ := c.do("GET", "/steps/"+stepID+p, nil); status != http.StatusNotFound {
			t.Errorf("deleted step%s: %d", p, status)
		}
	}
}

// between returns the text after the last start before marker, up to end.
func between(body, start, end, marker string) string {
	i := strings.LastIndex(body[:strings.Index(body, marker)], start) + len(start)
	return body[i : i+strings.Index(body[i:], end)]
}

func TestBuiltinSteps(t *testing.T) {
	c := newSeededClient(t)
	_, _, body := c.do("GET", "/steps", nil)
	mustContain(t, body, "<h2>Aufnahme</h2>", ">Kettenlängung messen</a>",
		`<th scope="rowgroup" colspan="5">Tretlager, Kurbel &amp; Pedale</th>`)
	for _, raw := range []string{"catalog.", "phase.", "assembly.", "step.action.", "level.", "component.type."} {
		if strings.Contains(body, raw) {
			t.Errorf("untranslated %q on the catalog page", raw)
		}
	}
	id := builtinStepID(t, c, "chain_wear")
	_, _, body = c.do("GET", "/steps/"+id, nil)
	mustContain(t, body, "Mit einer Kettenlehre messen", "Dazu die Werte der verbauten Komponenten: Kette (Grenzwert)")
	c.lang = "en"
	_, _, body = c.do("GET", "/steps", nil)
	mustContain(t, body, ">Measure chain wear</a>", "<h2>Intake</h2>")
	c.lang = "de"

	// Saving a built-in step unchanged keeps it translated; a changed
	// title is kept as entered.
	_, _, edit := c.do("GET", "/steps/"+id+"/edit", nil)
	mustContain(t, edit, `value="Kettenlängung messen"`)
	form := url.Values{
		"title": {"Kettenlängung messen"}, "instructions": {between(edit, `rows="4">`, `<`, "</textarea>")},
		"phase": {"inspection"}, "assembly": {"drivetrain"}, "level": {"monthly"}, "component_type": {"chain"},
	}
	c.do("POST", "/steps/"+id, form)
	c.lang = "en"
	_, _, body = c.do("GET", "/steps/"+id, nil)
	mustContain(t, body, "<h1>Measure chain wear</h1>", "Measure with a chain checker")
	c.lang = "de"
	form.Set("title", "Kette messen")
	_, _, body = c.do("POST", "/steps/"+id, form)
	mustContain(t, body, "<h1>Kette messen</h1>", "Mit einer Kettenlehre messen")

	// A maintenance shows the translated copies.
	bikePath := c.createBike("Gravel")
	_, _, body = c.do("POST", bikePath+"/maintenances", url.Values{"level": {"major"}, "date": {"2026-10-26"}, "km": {"0"}})
	mustContain(t, body, "<h1>Große Inspektion</h1>", "<strong>Bremsflächen reinigen</strong>", "Isopropanol")
	if strings.Contains(body, "catalog.") {
		t.Error("untranslated step in maintenance")
	}

	// Deleting a built-in step hides it; it can be restored.
	_, _, body = c.do("POST", "/steps/"+id+"/delete", url.Values{})
	if strings.Contains(body, ">Kette messen</a>") {
		t.Error("deleted built-in step still shown")
	}
	mustContain(t, body, "Ausgeblendete mitgelieferte Schritte (1)", "<td>Kette messen</td>", `action="/steps/`+id+`/restore"`)
	_, _, body = c.do("POST", "/steps/"+id+"/restore", url.Values{})
	mustContain(t, body, ">Kette messen</a>")
	if strings.Contains(body, "Ausgeblendete mitgelieferte Schritte") {
		t.Error("restored step still listed as hidden")
	}
}

func TestModuleSteps(t *testing.T) {
	c := newSeededClient(t)
	_, _, body := c.do("GET", "/steps", nil)
	mustContain(t, body, ">Dichtmilch prüfen</a>", `<span class="badge module">Tubeless-Reifen</span>`,
		"ersetzt „Schlauch &amp; Ventil“")
	_, _, body = c.do("GET", "/steps/"+builtinStepID(t, c, "torque_saddle"), nil)
	mustContain(t, body, `<p class="hint"><small>Carbon: Drehmomentschlüssel Pflicht`)
	_, _, body = c.do("GET", "/steps/new", nil)
	mustContain(t, body, `<option value="carbon">Carbon-Rahmen</option>`)

	plain := c.createBike("Stadtrad")
	_, bikePath, _ := c.do("POST", "/bikes", url.Values{"name": {"Gravel"}, "feature": {"carbon", "tubeless"}})

	// The start form counts and offers only the steps for the bike.
	_, _, body = c.do("GET", plain+"/maintenances/new", nil)
	if strings.Contains(body, "Nach Sturz") || strings.Contains(body, "Dichtmilch") {
		t.Error("module steps offered for a bike without features")
	}
	_, _, body = c.do("GET", bikePath+"/maintenances/new", nil)
	mustContain(t, body, "Nach Sturz: Tiefenprüfung", "Dichtmilch nachfüllen")

	_, _, body = c.do("POST", bikePath+"/maintenances", url.Values{"level": {"minor"}, "date": {"2026-10-28"}, "km": {"0"}})
	mustContain(t, body, "<strong>Dichtmilch prüfen</strong>", "<strong>Zug-Scheuerstellen</strong>",
		`<p class="hint"><small>Carbon: Drehmomentschlüssel Pflicht`, `<p class="hint"><small>Tubeless: Niedrigerer Druck`)
	if strings.Contains(body, "<strong>Schlauch &amp; Ventil</strong>") {
		t.Error("replaced step in the maintenance")
	}
	_, _, body = c.do("POST", plain+"/maintenances", url.Values{"level": {"minor"}, "date": {"2026-10-28"}, "km": {"0"}})
	mustContain(t, body, "<strong>Schlauch &amp; Ventil</strong>")
	if strings.Contains(body, `class="hint"`) || strings.Contains(body, "Dichtmilch") {
		t.Error("module steps or hints for a bike without features")
	}
}

func TestStepPage(t *testing.T) {
	c := newSeededClient(t)
	sealant, tube := builtinStepID(t, c, "tubeless_sealant"), builtinStepID(t, c, "tube_valve")

	// A module step links the step it replaces and has its own interval.
	_, _, body := c.do("GET", "/steps/"+sealant, nil)
	mustContain(t, body, "<h1>Dichtmilch prüfen</h1>", `<a href="/steps/`+tube+`">Schlauch &amp; Ventil</a>`,
		"<dd>Tubeless-Reifen</dd>", "jeden Monat", `href="/steps/`+sealant+`/edit"`, `<nav aria-label="breadcrumb">`)
	// ... and the replaced step links back.
	_, _, body = c.do("GET", "/steps/"+tube, nil)
	mustContain(t, body, "Ersetzt durch", `<a href="/steps/`+sealant+`">Dichtmilch prüfen</a> <small>(Tubeless-Reifen)</small>`,
		"<dd>Alle Räder</dd>")

	// The level chips are fixed, the lit ones belong to the step.
	_, _, body = c.do("GET", "/steps/"+builtinStepID(t, c, "chain_wear"), nil)
	mustContain(t, body, `aria-label="Monatscheck, Kleine Inspektion, Große Inspektion"`)
	if n := strings.Count(body, `class="lv on"`); n != 3 {
		t.Errorf("%d lit chips, want 3", n)
	}
	if n := strings.Count(body, `class="lv"`); n != 1 {
		t.Errorf("%d dim chips, want 1", n)
	}

	_, _, body = c.do("GET", "/steps/"+builtinStepID(t, c, "derailleur_hanger"), nil)
	mustContain(t, body, "Entfällt bei", "Nabenschaltung")
	_, _, body = c.do("GET", "/steps/"+builtinStepID(t, c, "torque_saddle"), nil)
	mustContain(t, body, `<p class="hint">`)

	if status, _, _ := c.do("GET", "/steps/does-not-exist", nil); status != http.StatusNotFound {
		t.Errorf("unknown step: %d", status)
	}
}

func TestStepFilter(t *testing.T) {
	c := newSeededClient(t)
	has := func(query string, builtin string) bool {
		t.Helper()
		_, _, body := c.do("GET", "/steps?"+query, nil)
		return strings.Contains(body, `<tr id="step-`+builtinStepID(t, c, builtin)+`"`)
	}
	for _, tc := range []struct {
		query string
		want  map[string]bool
	}{
		{"level=pre_ride", map[string]bool{"brake_function": true, "wash_bike": false, "replace_chain": false}},
		{"level=on_demand", map[string]bool{"replace_chain": true, "brake_function": false}},
		{"module=tubeless", map[string]bool{"tubeless_sealant": true, "wash_bike": false}},
		{"module=none", map[string]bool{"wash_bike": true, "tubeless_sealant": false}},
		{"assembly=brakes&action=measure", map[string]bool{"brake_pad_thickness": true, "brake_function": false, "chain_wear": false}},
		{"q=KETTENLEHRE", map[string]bool{"chain_wear": true, "wash_bike": false}},
		{"level=minor&q=sealant", map[string]bool{}},
	} {
		for builtin, want := range tc.want {
			if got := has(tc.query, builtin); got != want {
				t.Errorf("?%s: %s listed = %v, want %v", tc.query, builtin, got, want)
			}
		}
	}

	_, _, body := c.do("GET", "/steps?level=pre_ride&q=bremsen", nil)
	mustContain(t, body, `<details class="filter" open>`, `<option value="pre_ride" selected>`, `value="bremsen"`,
		`<details class="phase" open>`, `href="/steps" role="button" class="outline">Zurücksetzen`)
	if strings.Contains(body, `/up" hx-post`) || strings.Contains(body, `/down" hx-post`) {
		t.Error("move buttons while filtering")
	}
	_, _, body = c.do("GET", "/steps?q=zzzz", nil)
	mustContain(t, body, "Keine Schritte passen zum Filter.")
	_, _, body = c.do("GET", "/steps", nil)
	mustContain(t, body, `<details class="filter">`, `/up" hx-post`)
}

func TestStepModules(t *testing.T) {
	c := newSeededClient(t)
	status, _, body := c.do("GET", "/steps/modules", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	mustContain(t, body, "<h1>Module</h1>", `href="/steps" aria-current="page"`, "<h2>Riemenantrieb</h2>",
		`<a href="/steps/`+builtinStepID(t, c, "belt_wear")+`">`, "ersetzt „Kettenlängung messen“",
		`<a href="/steps/`+builtinStepID(t, c, "derailleur_hanger")+`">Schaltauge prüfen</a> <small>entfällt mit diesem Modul</small>`,
		"Hinweis bei:", "Elektronische Schaltung: per Taste oder App einstellen")
	for _, raw := range []string{"modules.", "feature.", "hint.", "level."} {
		if strings.Contains(body, raw) {
			t.Errorf("untranslated %q on the modules page", raw)
		}
	}
	_, _, body = c.do("GET", "/steps", nil)
	mustContain(t, body, `href="/steps/modules"`)
}

func TestStepFollowUps(t *testing.T) {
	c := newSeededClient(t)
	chainWear, replaceChain := builtinStepID(t, c, "chain_wear"), builtinStepID(t, c, "replace_chain")
	adjust := builtinStepID(t, c, "adjust_derailleur")

	// The built-in links show in the catalog, on both detail pages and in
	// the form.
	_, _, body := c.do("GET", "/steps", nil)
	mustContain(t, body, "führt zu: Kette tauschen", "danach immer: Schaltwerk einstellen")
	_, _, body = c.do("GET", "/steps/"+chainWear, nil)
	mustContain(t, body, "<h2>Folgeschritte</h2>", "<dt>Bei Befund</dt>", `<a href="/steps/`+replaceChain+`">Kette tauschen</a>`)
	_, _, body = c.do("GET", "/steps/"+replaceChain, nil)
	mustContain(t, body, "<dt>Immer danach</dt>", `<a href="/steps/`+adjust+`">Schaltwerk einstellen</a>`,
		"<dt>Folgt auf</dt>", `<a href="/steps/`+chainWear+`">Kettenlängung messen</a> <small>(Bei Befund)</small>`)
	_, _, body = c.do("GET", "/steps/"+chainWear+"/edit", nil)
	mustContain(t, body, "<legend>Folgeschritte</legend>", `<option value="`+replaceChain+`" selected>Kette tauschen</option>`,
		`<optgroup label="Demontage &amp; Tausch">`)
	if strings.Contains(body, `<option value="`+chainWear+`"`) {
		t.Error("step offered as its own follow-up")
	}

	// A new step with two follow-ups; blank rows are ignored.
	_, path, body := c.do("POST", "/steps", url.Values{
		"title": {"Bremse prüfen"}, "phase": {"inspection"},
		"follow_up_step": {builtinStepID(t, c, "replace_brake_pads"), "", adjust},
		"follow_up_kind": {"finding", "finding", "always"},
	})
	if !strings.HasPrefix(path, "/steps/") {
		t.Fatalf("after create: %s", path)
	}
	mustContain(t, body, "<dt>Bei Befund</dt>", "Beläge tauschen</a>", "<dt>Immer danach</dt>", "Schaltwerk einstellen</a>")

	for name, form := range map[string]url.Values{
		"unknown step": {"title": {"x"}, "phase": {"inspection"}, "follow_up_step": {"nope"}, "follow_up_kind": {"finding"}},
		"unknown kind": {"title": {"x"}, "phase": {"inspection"}, "follow_up_step": {adjust}, "follow_up_kind": {"later"}},
	} {
		if status, _, body := c.do("POST", "/steps", form); status != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d", name, status)
		} else {
			mustContain(t, body, `role="alert"`)
		}
	}
}
