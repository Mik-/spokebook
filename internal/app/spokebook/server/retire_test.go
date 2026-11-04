package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestRetire(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Stadtrad")
	bikeID := strings.TrimPrefix(bikePath, "/bikes/")
	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-01-01"}, "km": {"100"}})
	_, compPath, _ := c.do("POST", "/components", url.Values{"type": {"tire"}, "manufacturer": {"Schwalbe"}, "model": {"Marathon"}})
	c.do("POST", compPath+"/install", url.Values{"bike": {bikeID}, "date": {"2026-01-01"}})

	// Retiring the component removes it from the bike and the active list.
	_, _, body := c.do("POST", compPath+"/retire", url.Values{"date": {"2026-06-01"}, "reason": {"worn"}, "km": {"900"}})
	mustContain(t, body, "Ausgemustert am 01.06.2026: verschlissen.", "Reaktivieren", "800 km")
	if strings.Contains(body, `href="`+compPath+`/install"`) {
		t.Error("retired component offers installation")
	}
	_, _, body = c.do("GET", "/components", nil)
	mustContain(t, body, "Ausgemustert (1)")
	if i := strings.Index(body, "Ausgemustert (1)"); strings.Index(body, "Schwalbe Marathon") < i {
		t.Error("retired component listed as active")
	}
	status, _, _ := c.do("POST", compPath+"/install", url.Values{"bike": {bikeID}, "date": {"2026-07-01"}, "km": {"900"}})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("install retired component: status %d", status)
	}

	// Reactivate and install again, then retire the bike.
	c.do("POST", compPath+"/reactivate", url.Values{})
	c.do("POST", compPath+"/install", url.Values{"bike": {bikeID}, "date": {"2026-07-01"}, "km": {"900"}})
	_, _, body = c.do("GET", bikePath+"/retire", nil)
	mustContain(t, body, "<h1>Fahrrad ausmustern: Stadtrad</h1>", "Verbaute Komponenten werden zu diesem Datum ausgebaut.")
	status, _, body = c.do("POST", bikePath+"/retire", url.Values{"date": {"2026-09-01"}, "reason": {"sold"}, "km": {"800"}})
	// The odometer entered at the installation (900 km) was recorded.
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("retire bike below recorded odometer: status %d", status)
	}
	mustContain(t, body, "nicht unter 900 km liegen", "<h1>Fahrrad ausmustern: Stadtrad</h1>")
	_, _, body = c.do("POST", bikePath+"/retire", url.Values{"date": {"2026-09-01"}, "reason": {"sold"}, "km": {"1500"}})
	mustContain(t, body, "Ausgemustert am 01.09.2026: verkauft.", "Keine Komponenten verbaut.")
	if strings.Contains(body, `href="`+bikePath+`/odometer/new"`) {
		t.Error("retired bike offers odometer entry")
	}

	_, _, body = c.do("GET", "/", nil)
	mustContain(t, body, "Ausgemustert (1)", ">Stadtrad</a>", "<td>01.09.2026<br><small>verkauft</small></td>")
	_, _, body = c.do("GET", compPath, nil)
	mustContain(t, body, "Nicht verbaut.", "1.400 km")
	if strings.Contains(body, `<option value="`+bikeID+`"`) {
		t.Error("retired bike offered for installation")
	}
}
