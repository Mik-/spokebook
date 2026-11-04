package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestMaintenance(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Gravel")
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-01-01"}, "km": {"100"}})

	// Without steps the start page points to creating one.
	_, _, body := c.do("GET", bikePath+"/maintenances/new", nil)
	mustContain(t, body, "Es gibt noch keine Wartungsschritte.", `href="/steps/new"`)

	// Two brake levers with torques, left and right.
	for _, pos := range []string{"right", "left"} {
		c.do("POST", bikePath+"/components", valueForm(url.Values{
			"type": {"shift_brake_lever"}, "manufacturer": {"Shimano"}, "model": {"GRX " + pos},
			"date": {"2026-01-01"}, "km": {"100"}, "position": {pos},
		}, [4]string{"Drehmoment Schelle", "6", "8", "Nm"}))
	}
	c.do("POST", "/steps", url.Values{"title": {"Probefahrt"}, "phase": {"test_ride"}, "level": {"minor"}})
	c.do("POST", "/steps", valueForm(url.Values{
		"title": {"Hebel"}, "phase": {"inspection"}, "assembly": {"brakes"}, "level": {"minor"},
		"component_type": {"shift_brake_lever"},
	}, [4]string{"Belagstärke", "1,2", "", "mm"}))
	_, extraPath, _ := c.do("POST", "/steps", url.Values{"title": {"Beläge tauschen"}, "phase": {"replacement"}, "assembly": {"brakes"}})
	extraID := strings.TrimPrefix(extraPath, "/steps/")

	_, _, body = c.do("GET", bikePath+"/maintenances/new?level=minor", nil)
	mustContain(t, body, `<option value="minor" selected>Kleine Inspektion (2 Schritte)</option>`,
		`<option value="pre_ride">Vor der Fahrt (0 Schritte)</option>`, `name="step" value="`+extraID+`"`,
		"Beläge tauschen <small>(Demontage &amp; Tausch · Bremsen)</small>",
		`name="km" min="0" step="0.1" inputmode="decimal" value="100"`)
	for name, form := range map[string]url.Values{
		"nothing chosen":      {"date": {"2026-10-26"}, "km": {"120"}},
		"level without steps": {"level": {"pre_ride"}, "date": {"2026-10-26"}, "km": {"120"}},
		"unknown level":       {"level": {"weekly"}, "date": {"2026-10-26"}, "km": {"120"}},
	} {
		if status, _, body := c.do("POST", bikePath+"/maintenances", form); status != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d", name, status)
		} else {
			mustContain(t, body, `role="alert"`)
		}
	}
	status, mPath, body := c.do("POST", bikePath+"/maintenances", url.Values{
		"level": {"minor"}, "step": {extraID}, "date": {"2026-10-26"}, "km": {"120"},
	})
	if status != http.StatusOK || !strings.HasPrefix(mPath, "/maintenances/") {
		t.Fatalf("start: %d %s", status, mPath)
	}
	mustContain(t, body, "<h1>Kleine Inspektion</h1>", "26.10.2026", "120 km", "offen", "0/3 erledigt",
		"<h2>Prüfen &amp; Messen</h2>", `<h3 class="assembly">Bremsen</h3>`, "<h2>Probefahrt &amp; Dokumentation</h2>",
		"Belagstärke", "min. 1,2 mm", "Shimano GRX left (links)", "Shimano GRX right (rechts)", "6–8 Nm")
	if strings.Index(body, "GRX left") > strings.Index(body, "GRX right") {
		t.Error("left lever should come before right")
	}
	// Steps in the order of phases.
	hebel, tausch, fahrt := strings.Index(body, "<strong>Hebel</strong>"), strings.Index(body, "<strong>Beläge tauschen</strong>"),
		strings.Index(body, "<strong>Probefahrt</strong>")
	if hebel > tausch || tausch > fahrt {
		t.Error("steps not in phase order")
	}
	// Only the minimum gets a measurement field.
	if n := strings.Count(body, `name="measured_`); n != 1 {
		t.Errorf("%d measurement fields, want 1", n)
	}
	_, _, body = c.do("GET", mPath+"/edit", nil)
	mustContain(t, body, `value="2026-10-26"`, `value="120"`)
	if status, _, _ := c.do("POST", mPath+"/edit", url.Values{"date": {"2026-10-25"}, "km": {"50"}}); status != http.StatusUnprocessableEntity {
		t.Errorf("odometer going backwards: status %d", status)
	}
	_, path, body := c.do("POST", mPath+"/edit", url.Values{"date": {"2026-10-25"}, "km": {"119"}})
	if path != mPath {
		t.Errorf("after edit: %s", path)
	}
	mustContain(t, body, "25.10.2026", "119 km", "Shimano GRX left")

	measured := between(body, `name="measured_`, `"`, `aria-label="Gemessen: Belagstärke"`)
	stepID := between(body, `name="done_`, `"`, "<strong>Hebel</strong>")

	status, _, body = c.do("POST", mPath, url.Values{"measured_" + measured: {"x"}})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("invalid measurement: %d", status)
	}
	mustContain(t, body, "Bitte für „Belagstärke“ eine gültige Zahl angeben.", `value="x"`)

	// The open page keeps the screen on and saves on every change with htmx,
	// without a redirect.
	mustContain(t, body, `src="/static/wakelock.js"`, `src="/static/vendor/htmx-2.0.4.min.js"`, `hx-post="`+mPath+`"`, `hx-trigger="change"`,
		`src="/static/autosave.js"`, `data-save-error hidden>Nicht gespeichert – keine Verbindung<`)
	c.htmx = true
	status, path, body = c.do("POST", mPath, url.Values{"done_" + stepID: {"1"}})
	c.htmx = false
	if status != http.StatusOK || path != mPath {
		t.Errorf("autosave: %d %s", status, path)
	}
	mustContain(t, body, `<div id="maintenance-status" aria-live="polite">`, "1/3 erledigt · <small>gespeichert</small>")

	_, _, body = c.do("POST", mPath, url.Values{
		"done_" + stepID: {"1"}, "note_" + stepID: {"Beläge vorn neu"}, "measured_" + measured: {"1,0"}, "notes": {"Alles gut"},
	})
	mustContain(t, body, "1/3 erledigt", "außerhalb der Grenze", `value="1"`, `value="Beläge vorn neu"`, "Alles gut")
	if strings.Contains(body, "<small>gespeichert</small>") {
		t.Error("page after a plain save reports the autosave")
	}

	_, _, body = c.do("POST", mPath, url.Values{"done_" + stepID: {"1"}, "measured_" + measured: {"2"}, "complete": {"1"}})
	mustContain(t, body, "abgeschlossen", "Wieder öffnen", "✓ Hebel", "✗ Probefahrt")
	if strings.Contains(body, "htmx") || strings.Contains(body, "wakelock.js") {
		t.Error("completed maintenance loads htmx or keeps the screen on")
	}
	if strings.Contains(body, `name="measured_`) || strings.Contains(body, "außerhalb der Grenze") {
		t.Error("completed maintenance still editable or out of limit")
	}
	if status, _, _ := c.do("POST", mPath, url.Values{"notes": {"x"}}); status != http.StatusUnprocessableEntity {
		t.Errorf("save completed: %d", status)
	}

	_, _, body = c.do("GET", bikePath, nil)
	mustContain(t, body, "Wartungen", `href="`+mPath+`"`, "Kleine Inspektion", `<span class="badge done">abgeschlossen</span> 1/3 erledigt`)

	_, _, body = c.do("POST", mPath+"/reopen", url.Values{})
	mustContain(t, body, `name="measured_`)

	// Catalog steps can be deleted; the maintenance stays.
	c.do("POST", "/steps/"+extraID+"/delete", url.Values{})
	_, _, body = c.do("GET", mPath, nil)
	mustContain(t, body, "Beläge tauschen", "Belagstärke")

	_, path, body = c.do("POST", mPath+"/delete", url.Values{})
	if path != bikePath || !strings.Contains(body, "Noch keine Wartung.") {
		t.Errorf("after delete: %s", path)
	}
}

func TestMaintenanceAddSteps(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Gravel")
	c.do("POST", "/steps", url.Values{"title": {"Probefahrt"}, "phase": {"test_ride"}, "level": {"monthly"}})
	_, chainPath, _ := c.do("POST", "/steps", url.Values{"title": {"Kette tauschen"}, "phase": {"replacement"}, "assembly": {"drivetrain"}})
	chainID := strings.TrimPrefix(chainPath, "/steps/")
	_, mPath, body := c.do("POST", bikePath+"/maintenances", url.Values{"level": {"monthly"}, "date": {"2026-10-26"}, "km": {"0"}})
	mustContain(t, body, `href="`+mPath+`/steps"`, "0/1 erledigt")

	// Only steps not yet contained are offered.
	_, _, body = c.do("GET", mPath+"/steps", nil)
	mustContain(t, body, `<details class="phase">`, "<h2>Demontage &amp; Tausch</h2> <small>1 Schritt</small>",
		`<h3 class="assembly">Antrieb</h3>`, `name="step" value="`+chainID+`"> Kette tauschen <small>(Bei Bedarf)</small>`)
	if strings.Contains(body, "Probefahrt</label>") {
		t.Error("contained step offered")
	}
	if status, _, body := c.do("POST", mPath+"/steps", url.Values{}); status != http.StatusUnprocessableEntity {
		t.Errorf("nothing chosen: %d", status)
	} else {
		mustContain(t, body, "Bitte mindestens einen Schritt wählen.")
	}

	_, path, body := c.do("POST", mPath+"/steps", url.Values{"step": {chainID}})
	if path != mPath {
		t.Fatalf("after add: %s", path)
	}
	mustContain(t, body, "0/2 erledigt", "<strong>Kette tauschen</strong>", "<h2>Demontage &amp; Tausch</h2>")
	_, _, body = c.do("GET", mPath+"/steps", nil)
	mustContain(t, body, "Diese Wartung enthält schon alle Schritte des Katalogs.")

	c.do("POST", mPath, url.Values{"complete": {"1"}})
	if status, _, _ := c.do("POST", mPath+"/steps", url.Values{"step": {chainID}}); status != http.StatusUnprocessableEntity {
		t.Errorf("completed: %d", status)
	}
}

func TestMaintenanceNewRoundsKm(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Gravel")
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-01-01"}, "km": {"1234.5"}})
	c.do("POST", "/steps", url.Values{"title": {"Probefahrt"}, "phase": {"test_ride"}, "level": {"minor"}})
	_, _, body := c.do("GET", bikePath+"/maintenances/new", nil)
	mustContain(t, body, `name="km" min="0" step="0.1" inputmode="decimal" value="1235"`)
}
