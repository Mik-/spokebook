package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestFollowUpFlow(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Gravel")
	newStep := func(form url.Values) string {
		t.Helper()
		_, path, _ := c.do("POST", "/steps", form)
		if !strings.HasPrefix(path, "/steps/") {
			t.Fatalf("step not created: %s", path)
		}
		return strings.TrimPrefix(path, "/steps/")
	}
	adjust := newStep(url.Values{"title": {"Bremse einstellen"}, "phase": {"adjustment"}, "assembly": {"brakes"}})
	replace := newStep(url.Values{
		"title": {"Beläge wechseln"}, "phase": {"replacement"}, "assembly": {"brakes"},
		"follow_up_step": {adjust}, "follow_up_kind": {"always"},
	})
	newStep(valueForm(url.Values{
		"title": {"Belag messen"}, "phase": {"inspection"}, "assembly": {"brakes"}, "level": {"monthly"},
		"follow_up_step": {replace}, "follow_up_kind": {"finding"},
	}, [4]string{"Belagstärke", "1", "", "mm"}))

	// The measuring step offers replacing; saving swaps the offers too.
	_, mPath, body := c.do("POST", bikePath+"/maintenances", url.Values{"level": {"monthly"}, "date": {"2026-11-01"}, "km": {"0"}})
	measure := between(body, `id="mstep-`, `"`, "<strong>Belag messen</strong>")
	button := `name="follow_up" value="` + measure + ":" + replace + `"`
	live := `hx-target="#maintenance-live" hx-select="#maintenance-live" hx-swap="outerHTML"`
	mustContain(t, body, `formaction="`+mPath+`/follow-up" `+button+` hx-post="`+mPath+`/follow-up" `+live,
		`class="outline">+ Beläge wechseln</button>`, `hx-select-oob="#follow-ups-`+measure+`"`, `class="default-submit"`,
		`<div id="maintenance-live">`, `hx-disinherit="*"`)
	value := between(body, `name="measured_`, `"`, `aria-label="Gemessen: Belagstärke"`)

	// Below the limit it is needed.
	_, _, body = c.do("POST", mPath, url.Values{"measured_" + value: {"0,8"}})
	mustContain(t, body, `hx-sync="closest form:queue last">+ Beläge wechseln</button>`, `<span class="badge danger">nötig</span>`)

	// Adding it saves the form first and brings what always follows; with
	// htmx the page after the redirect is swapped in.
	c.htmx = true
	status, path, body := c.do("POST", mPath+"/follow-up", url.Values{
		"follow_up": {measure + ":" + replace}, "done_" + measure: {"1"}, "measured_" + value: {"0,8"},
	})
	c.htmx = false
	if status != http.StatusOK || path != mPath {
		t.Fatalf("after adding: %d %s", status, path)
	}
	mustContain(t, body, "<strong>Beläge wechseln</strong>", "<strong>Bremse einstellen</strong>",
		"Folgeschritt von: Belag messen", "Folgeschritt von: Beläge wechseln", `name="done_`+measure+`" value="1" checked>`,
		`<a href="#mstep-`)
	if strings.Contains(body, button) {
		t.Error("added follow-up still offered")
	}
	if status, _, _ := c.do("POST", mPath+"/follow-up", url.Values{"follow_up": {measure + ":" + adjust}}); status != http.StatusNotFound {
		t.Errorf("follow-up not offered: %d", status)
	}

	// Completed without replacing, both stay open work of the bike.
	c.do("POST", mPath, url.Values{"done_" + measure: {"1"}, "complete": {"1"}})
	_, _, body = c.do("GET", mPath, nil)
	if n := strings.Count(body, `<span class="badge danger">Offen</span>`); n != 2 {
		t.Errorf("%d open steps, want 2", n)
	}
	_, _, body = c.do("GET", bikePath, nil)
	mustContain(t, body, "<strong>Beläge wechseln</strong> <span class=\"badge danger\">Offen</span>", "offen seit 01.11.2026 (aus: Belag messen)",
		`href="`+bikePath+`/maintenances/new?step=`+replace+`" role="button" class="outline">Nachholen`, ">Verwerfen</button>")

	// Catching up preselects only that step.
	_, _, body = c.do("GET", bikePath+"/maintenances/new?step="+replace, nil)
	mustContain(t, body, `name="step" value="`+replace+`" checked> Beläge wechseln`)
	if strings.Contains(body, `<option value="monthly" selected>`) {
		t.Error("level preselected when catching up")
	}

	// Dismissing adjusting removes it; catching up the replacement (which
	// brings adjusting again) the rest.
	_, _, body = c.do("GET", mPath, nil)
	rest := body[strings.Index(body, "✗ Bremse einstellen"):]
	rest = rest[strings.Index(rest, mPath+"/steps/")+len(mPath+"/steps/"):]
	adjustStep := rest[:strings.Index(rest, "/dismiss")]
	_, path, body = c.do("POST", mPath+"/steps/"+adjustStep+"/dismiss", url.Values{"back": {"bike"}})
	if path != bikePath || strings.Contains(body, "<strong>Bremse einstellen</strong>") {
		t.Errorf("after dismissing: %s", path)
	}
	if status, _, _ := c.do("POST", mPath+"/steps/"+measure+"/dismiss", url.Values{}); status != http.StatusNotFound {
		t.Errorf("dismissing a step that is no follow-up: %d", status)
	}
	_, m2, body := c.do("POST", bikePath+"/maintenances", url.Values{"step": {replace}, "date": {"2026-11-08"}, "km": {"0"}})
	replaced := between(body, `name="done_`, `"`, "<strong>Beläge wechseln</strong>")
	adjusted := between(body, `name="done_`, `"`, "<strong>Bremse einstellen</strong>")
	c.do("POST", m2, url.Values{"done_" + replaced: {"1"}, "done_" + adjusted: {"1"}, "complete": {"1"}})
	_, _, body = c.do("GET", bikePath, nil)
	if strings.Contains(body, "offen seit") {
		t.Error("open work after catching up")
	}
}

func TestRemoveStep(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Gravel")
	newStep := func(form url.Values) string {
		t.Helper()
		_, path, _ := c.do("POST", "/steps", form)
		return strings.TrimPrefix(path, "/steps/")
	}
	adjust := newStep(url.Values{"title": {"Bremse einstellen"}, "phase": {"adjustment"}})
	replace := newStep(url.Values{"title": {"Beläge wechseln"}, "phase": {"replacement"}, "follow_up_step": {adjust}, "follow_up_kind": {"always"}})
	wash := newStep(url.Values{"title": {"Rad waschen"}, "phase": {"cleaning"}})
	newStep(url.Values{"title": {"Belag messen"}, "phase": {"inspection"}, "level": {"monthly"}, "follow_up_step": {replace}, "follow_up_kind": {"finding"}})

	_, mPath, body := c.do("POST", bikePath+"/maintenances", url.Values{"level": {"monthly"}, "date": {"2026-11-01"}, "km": {"0"}})
	measure := between(body, `id="mstep-`, `"`, "<strong>Belag messen</strong>")
	if strings.Contains(body, "Schritt entfernen") {
		t.Error("step of the level removable")
	}
	c.do("POST", mPath+"/follow-up", url.Values{"follow_up": {measure + ":" + replace}})
	_, _, body = c.do("POST", mPath+"/steps", url.Values{"step": {wash}})
	if n := strings.Count(body, ">Schritt entfernen</button>"); n != 3 {
		t.Errorf("%d remove buttons, want 3", n)
	}
	replaceStep := between(body, `id="mstep-`, `"`, "<strong>Beläge wechseln</strong>")
	washStep := between(body, `id="mstep-`, `"`, "<strong>Rad waschen</strong>")

	// Removing saves the form first and takes along what came with it.
	_, path, body := c.do("POST", mPath+"/steps/"+replaceStep+"/remove", url.Values{"done_" + measure: {"1"}})
	if path != mPath {
		t.Errorf("after removing: %s", path)
	}
	mustContain(t, body, `name="done_`+measure+`" value="1" checked>`, `value="`+measure+":"+replace+`"`)
	if strings.Contains(body, "Beläge wechseln</strong>") || strings.Contains(body, "Bremse einstellen</strong>") {
		t.Error("removed steps still shown")
	}

	// Ticked in the same go, it stays.
	status, _, body := c.do("POST", mPath+"/steps/"+washStep+"/remove", url.Values{"done_" + washStep: {"1"}})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("removing a done step: %d", status)
	}
	mustContain(t, body, "Ein abgehakter Schritt lässt sich nicht entfernen.", "<strong>Rad waschen</strong>")
	if status, _, _ := c.do("POST", mPath+"/steps/"+measure+"/remove", url.Values{}); status != http.StatusNotFound {
		t.Errorf("removing a step of the level: %d", status)
	}
}
