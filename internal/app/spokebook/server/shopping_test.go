package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestShoppingList(t *testing.T) {
	c := newSeededClient(t)
	_, _, body := c.do("GET", "/shopping", nil)
	mustContain(t, body, "<h1>Einkaufsliste</h1>", "Nichts zu kaufen.", `href="/shopping" aria-current="page"`)

	status, _, body := c.do("POST", "/shopping", url.Values{"text": {"  "}})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("empty text: %d", status)
	}
	mustContain(t, body, "Bitte angeben, was gekauft werden soll.")

	_, path, _ := c.do("POST", "/shopping", url.Values{"text": {" Kettenöl "}})
	if path != "/shopping" {
		t.Errorf("redirect to %s", path)
	}
	c.do("POST", "/shopping", url.Values{"text": {"Dichtmilch"}})
	_, _, body = c.do("GET", "/shopping", nil)
	mustContain(t, body, "<td>Kettenöl</td>", "<td>Dichtmilch</td>")
	mustNotContain(t, body, "Gekaufte entfernen")
	// Kettenöl is first.
	oilID := between(body, `action="/shopping/`, `/bought"`, `/bought"`)

	// Ticked off, it moves to the end and can be removed with the others.
	_, _, body = c.do("POST", "/shopping/"+oilID+"/bought", url.Values{})
	mustContain(t, body, `<tr class="bought">`, "<s>Kettenöl</s>", `action="/shopping/`+oilID+`/open"`, "Gekaufte entfernen")
	if strings.Index(body, "Dichtmilch") > strings.Index(body, "Kettenöl") {
		t.Error("bought item not at the end")
	}
	_, _, body = c.do("POST", "/shopping/"+oilID+"/open", url.Values{})
	mustNotContain(t, body, "<s>Kettenöl</s>")
	c.do("POST", "/shopping/"+oilID+"/bought", url.Values{})
	_, _, body = c.do("GET", "/shopping/bought/delete", nil)
	mustContain(t, body, "Alle gekauften Einträge von der Einkaufsliste entfernen?")
	_, _, body = c.do("POST", "/shopping/bought/delete", url.Values{})
	mustContain(t, body, "<td>Dichtmilch</td>")
	mustNotContain(t, body, "Kettenöl")

	// Edit and delete.
	sealantID := between(body, `href="/shopping/`, `/edit"`, `/edit"`)
	_, _, body = c.do("GET", "/shopping/"+sealantID+"/edit", nil)
	mustContain(t, body, `value="Dichtmilch"`)
	_, _, body = c.do("POST", "/shopping/"+sealantID, url.Values{"text": {"Dichtmilch 1 l"}})
	mustContain(t, body, "<td>Dichtmilch 1 l</td>")
	_, _, body = c.do("GET", "/shopping/"+sealantID+"/delete", nil)
	mustContain(t, body, "„Dichtmilch 1 l“ von der Einkaufsliste löschen?")
	_, _, body = c.do("POST", "/shopping/"+sealantID+"/delete", url.Values{})
	mustContain(t, body, "Nichts zu kaufen.")

	if status, _, _ := c.do("POST", "/shopping/"+sealantID+"/bought", url.Values{}); status != http.StatusNotFound {
		t.Errorf("deleted item: %d", status)
	}
}

func TestShoppingFromOrigin(t *testing.T) {
	c := newSeededClient(t)
	bikePath := c.createBike("Gravel")
	_, mPath, body := c.do("POST", bikePath+"/maintenances", url.Values{"level": {"monthly"}, "date": {"2026-05-01"}, "km": {"100"}})
	mustContain(t, body, `href="/shopping/new?maintenance=`+strings.TrimPrefix(mPath, "/maintenances/")+`"`)
	mID := strings.TrimPrefix(mPath, "/maintenances/")

	// From a maintenance, the form returns there.
	_, _, body = c.do("GET", "/shopping/new?maintenance="+mID, nil)
	mustContain(t, body, `<a href="`+bikePath+`">Gravel</a>`, `<a href="`+mPath+`">Monatscheck</a>`,
		`<input type="hidden" name="maintenance" value="`+mID+`">`)
	_, path, _ := c.do("POST", "/shopping", url.Values{"maintenance": {mID}, "text": {"Bremsbeläge"}})
	if path != mPath {
		t.Errorf("redirect to %s, want %s", path, mPath)
	}

	// From a component, the text is prefilled.
	_, cPath, body := c.do("POST", "/components", url.Values{"type": {"chain"}, "manufacturer": {"KMC"}, "model": {"X11"}, "part_number": {"CN-X11"}})
	cID := strings.TrimPrefix(cPath, "/components/")
	mustContain(t, body, `href="/shopping/new?component=`+cID+`" role="button" class="outline">Nachkaufen`)
	_, _, body = c.do("GET", "/shopping/new?component="+cID, nil)
	mustContain(t, body, `value="KMC X11 (CN-X11)"`, `<a href="`+cPath+`">KMC X11</a>`)
	_, path, _ = c.do("POST", "/shopping", url.Values{"component": {cID}, "text": {"KMC X11 (CN-X11)"}})
	if path != cPath {
		t.Errorf("redirect to %s, want %s", path, cPath)
	}

	_, _, body = c.do("GET", "/shopping", nil)
	mustContain(t, body, "<td>Bremsbeläge</td>", "<td>KMC X11 (CN-X11)</td>")

	// Unknown origins are not found and add nothing.
	for _, q := range []string{"maintenance", "component"} {
		if status, _, _ := c.do("GET", "/shopping/new?"+q+"=nope", nil); status != http.StatusNotFound {
			t.Errorf("GET %s: %d", q, status)
		}
		if status, _, _ := c.do("POST", "/shopping", url.Values{q: {"nope"}, "text": {"x"}}); status != http.StatusNotFound {
			t.Errorf("POST %s: %d", q, status)
		}
	}
}
