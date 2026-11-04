package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestDue(t *testing.T) {
	c := newClient(t)
	_, bikePath, _ := c.do("POST", "/bikes", url.Values{"name": {"Gravel"}, "in_service_at": {"2025-01-01"}, "in_service_km": {"0"}})
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2025-03-01"}, "km": {"480"}})
	c.do("POST", "/steps", url.Values{"title": {"Sichtprüfung"}, "phase": {"inspection"}, "level": {"monthly"}})
	_, stepPath, body := c.do("POST", "/steps", url.Values{
		"title": {"Federsattelstütze warten"}, "phase": {"replacement"}, "assembly": {"suspension"}, "interval_km": {"500"},
	})
	seatpostID := strings.TrimPrefix(stepPath, "/steps/")
	mustContain(t, body, "Bei Bedarf", "alle 500 km")

	// The monthly check counts from going into service; the seatpost is at
	// 480 of 500 km. Levels without steps don't show.
	_, _, body = c.do("GET", bikePath, nil)
	mustContain(t, body, "<h2>Fällig</h2>", "<strong>Monatscheck</strong> <span class=\"badge danger\">Überfällig</span>",
		"fällig seit 01.02.2025", "seit Inbetriebnahme 01.01.2025",
		"<strong>Federsattelstütze warten</strong> <span class=\"badge open\">Bald fällig</span> <small>alle 500 km</small>",
		"noch 20 km", `href="`+bikePath+`/maintenances/new" role="button">Wartung starten`, "<small>jeden Monat</small>")
	if strings.Contains(body, "%!") {
		t.Error("bad format in body")
	}
	if strings.Contains(body, "Kleine Inspektion") {
		t.Error("level without steps shown")
	}
	_, _, body = c.do("GET", "/", nil)
	mustContain(t, body, "<dt>Als Nächstes fällig</dt>", ">Monatscheck</a>")

	// Starting preselects both.
	_, _, body = c.do("GET", bikePath+"/maintenances/new", nil)
	mustContain(t, body, `<option value="monthly" selected>`, "<legend>Fällige Schritte</legend>",
		`name="step" value="`+seatpostID+`" checked> Federsattelstütze warten`)
	if strings.Count(body, `value="`+seatpostID+`"`) != 1 {
		t.Error("due step also listed on demand")
	}

	// Done in a completed maintenance, the seatpost starts over; the
	// monthly check is still overdue as only the seatpost was done.
	_, mPath, body := c.do("POST", bikePath+"/maintenances", url.Values{"step": {seatpostID}, "date": {"2025-03-02"}, "km": {"490"}})
	msID := between(body, `name="done_`, `"`, "Federsattelstütze warten")

	// While it is open, the bike page continues it instead of starting one.
	_, _, body = c.do("GET", bikePath, nil)
	mustContain(t, body, "<small>Offene Wartung</small>", `href="`+mPath+`" role="button">Fortsetzen`)
	if strings.Contains(body, "role=\"button\">Wartung starten") {
		t.Error("start shown while a maintenance is open")
	}
	c.do("POST", mPath, url.Values{"done_" + msID: {"on"}, "complete": {"1"}})
	_, _, body = c.do("GET", bikePath, nil)
	mustContain(t, body, "<strong>Federsattelstütze warten</strong>  <small>alle 500 km</small>", "noch 500 km",
		"zuletzt 02.03.2025", "Überfällig")
}

func TestIntervals(t *testing.T) {
	c := newClient(t)
	_, _, body := c.do("GET", "/steps", nil)
	mustContain(t, body, `href="/intervals" role="button" class="outline">Intervalle</a>`)
	_, _, body = c.do("GET", "/intervals", nil)
	// The page belongs to the catalog in the menu.
	mustContain(t, body, `<a href="/steps" aria-current="page"`)
	mustContain(t, body, `name="monthly_months" min="1" max="120" step="1" inputmode="numeric" aria-label="Monatscheck: Monate" value="1"`,
		`name="minor_km" min="0.1" step="0.1" inputmode="decimal" aria-label="Kleine Inspektion: km" value="1500"`,
		`name="major_months" min="1" max="120" step="1" inputmode="numeric" aria-label="Große Inspektion: Monate" value="12"`)
	if strings.Contains(body, "pre_ride_months") {
		t.Error("pre_ride has an interval")
	}

	for _, bad := range []url.Values{{"minor_months": {"0"}}, {"minor_months": {"1.5"}}, {"major_km": {"x"}}} {
		if status, _, body := c.do("POST", "/intervals", bad); status != http.StatusUnprocessableEntity {
			t.Errorf("%v: status %d", bad, status)
		} else {
			mustContain(t, body, "Intervall ungültig")
		}
	}

	// Empty fields mean no limit.
	status, path, _ := c.do("POST", "/intervals", url.Values{"minor_months": {"4"}, "major_km": {"5000"}})
	if status != http.StatusOK || path != "/steps" {
		t.Fatalf("save: %d %s", status, path)
	}
	_, _, body = c.do("GET", "/intervals", nil)
	mustContain(t, body, `aria-label="Monatscheck: Monate" value=""`, `aria-label="Kleine Inspektion: Monate" value="4"`,
		`aria-label="Kleine Inspektion: km" value=""`, `aria-label="Große Inspektion: km" value="5000"`)
}

func TestStepIntervalInvalid(t *testing.T) {
	c := newClient(t)
	status, _, body := c.do("POST", "/steps", url.Values{"title": {"X"}, "phase": {"inspection"}, "interval_months": {"-1"}})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status %d", status)
	}
	mustContain(t, body, "Intervall ungültig", `name="interval_months" min="1" max="120" step="1" inputmode="numeric" value="-1"`)
}
