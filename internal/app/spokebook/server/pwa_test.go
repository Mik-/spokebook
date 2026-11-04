package server_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestPWA(t *testing.T) {
	c := newAnonymousClient(t)

	// The service worker, the offline page and the manifest are public.
	res, err := c.srv.Client().Get(c.srv.URL + "/sw.js")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/javascript") {
		t.Fatalf("sw.js: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	status, path, body := c.do("GET", "/offline", nil)
	if status != http.StatusOK || path != "/offline" {
		t.Fatalf("offline: %d %s", status, path)
	}
	mustContain(t, body, "Keine Verbindung", `rel="manifest" href="/static/manifest.json"`, `src="/static/pwa.js"`)
	status, _, body = c.do("GET", "/static/manifest.json", nil)
	if status != http.StatusOK {
		t.Fatalf("manifest: %d", status)
	}
	mustContain(t, body, `"display": "standalone"`, `"purpose": "maskable"`)
}
