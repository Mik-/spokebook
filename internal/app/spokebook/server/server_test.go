package server_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Mik-/spokebook/internal/app/spokebook/auth"
	"github.com/Mik-/spokebook/internal/app/spokebook/server"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite"
)

type client struct {
	t    *testing.T
	srv  *httptest.Server
	st   store.Store
	lang string
	log  *bytes.Buffer
	htmx bool // send requests as htmx does
}

// newClient starts a server with an empty step catalog: the built-in steps
// are hidden. Requests are in German.
func newClient(t *testing.T) *client {
	t.Helper()
	c := newSeededClient(t)
	steps, err := c.st.Steps().ListSteps(context.Background(), auth.FirstUserID)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range steps {
		if err := c.st.Steps().HideStep(context.Background(), auth.FirstUserID, s.ID); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

// newSeededClient starts a server with the built-in step catalog and
// logs in as the first user.
func newSeededClient(t *testing.T) *client {
	t.Helper()
	return newSeededClientWith(t, nil)
}

// newSeededClientWith is newSeededClient with a configuration made for
// the store (nil: the default).
func newSeededClientWith(t *testing.T, config func(store.Store) server.Config) *client {
	t.Helper()
	c := newAnonymousClientWith(t, config)
	ctx := context.Background()
	if _, err := auth.CreateUser(ctx, c.st, "rider", testPassword); err != nil {
		t.Fatal(err)
	}
	token, _, err := auth.StartSession(ctx, c.st, auth.FirstUserID)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(c.srv.URL)
	c.srv.Client().Jar.SetCookies(u, []*http.Cookie{{Name: "spokebook_session", Value: token}})
	return c
}

const testPassword = "correct horse battery"

// newAnonymousClient starts a server without any account. Its client
// keeps cookies.
func newAnonymousClient(t *testing.T) *client {
	t.Helper()
	return newAnonymousClientWith(t, nil)
}

func newAnonymousClientWith(t *testing.T, config func(store.Store) server.Config) *client {
	t.Helper()
	st, err := sqlite.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var log bytes.Buffer
	cfg := server.Config{BaseURL: "http://localhost"}
	if config != nil {
		cfg = config(st)
	}
	app, err := server.New(context.Background(), st, slog.New(slog.NewTextHandler(&log, nil)), cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app.Handler())
	t.Cleanup(srv.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.Client().Jar = jar
	return &client{t: t, srv: srv, st: st, lang: "de", log: &log}
}

// do sends a request in c.lang and returns status, final path and body.
// Redirects are followed.
func (c *client) do(method, path string, form url.Values) (int, string, string) {
	c.t.Helper()
	req, err := http.NewRequest(method, c.srv.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Accept-Language", c.lang)
	if c.htmx {
		req.Header.Set("HX-Request", "true")
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", c.srv.URL)
	}
	res, err := c.srv.Client().Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	return res.StatusCode, res.Request.URL.Path, string(body)
}

// createBike creates a bike and returns the path of its page.
func (c *client) createBike(name string) string {
	c.t.Helper()
	status, path, _ := c.do("POST", "/bikes", url.Values{"name": {name}})
	if status != http.StatusOK || !strings.HasPrefix(path, "/bikes/") {
		c.t.Fatalf("create bike: %d %s", status, path)
	}
	return path
}

func mustContain(t *testing.T, body string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("body does not contain %q", w)
		}
	}
}

func mustNotContain(t *testing.T, body string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(body, u) {
			t.Errorf("body contains %q", u)
		}
	}
}

func TestBikeWithOdometer(t *testing.T) {
	c := newClient(t)

	status, _, body := c.do("POST", "/bikes", url.Values{"name": {""}})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("empty name: status %d", status)
	}
	mustContain(t, body, "Bitte einen Namen angeben.")

	status, _, _ = c.do("POST", "/bikes", url.Values{"name": {"Gravel"}, "type": {"Gravel"}})
	if status != http.StatusOK {
		t.Fatalf("create: status %d", status)
	}
	_, _, body = c.do("GET", "/", nil)
	mustContain(t, body, "Fahrräder", "Gravel")

	// Follow the link from the list to the bike page.
	bikePath := firstLink(t, body, "/bikes/")

	status, _, body = c.do("GET", bikePath, nil)
	if status != http.StatusOK {
		t.Fatalf("show: status %d", status)
	}
	mustContain(t, body, "Noch keine Kilometerstände.")

	for _, km := range []string{"-1", "abc", ""} {
		status, _, body = c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-10-25"}, "km": {km}})
		if status != http.StatusUnprocessableEntity {
			t.Errorf("km %q: status %d", km, status)
		}
		mustContain(t, body, "Bitte einen Kilometerstand von 0 oder mehr angeben.")
	}
	status, _, _ = c.do("POST", bikePath+"/odometer", url.Values{"date": {"25.10.2026"}, "km": {"1"}})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("invalid date: status %d", status)
	}

	c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-05-01"}, "km": {"1000"}})
	status, path, body := c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-10-25"}, "km": {"1234,5"}})
	if status != http.StatusOK || path != bikePath {
		t.Fatalf("add reading: status %d, path %s", status, path)
	}
	mustContain(t, body, `<small>Kilometerstand</small><strong>1.234,5 km</strong>`, "25.10.2026", "01.05.2026", "1.000 km", "manuell")

	status, _, body = c.do("POST", bikePath+"/odometer", url.Values{"date": {"2026-10-26"}, "km": {"1200"}})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("decreasing odometer: status %d", status)
	}
	mustContain(t, body, "Der Kilometerstand kann nicht unter 1.234,5 km liegen (Stand vom 25.10.2026).")
}

func TestNotFound(t *testing.T) {
	c := newClient(t)
	if status, _, body := c.do("GET", "/bikes/does-not-exist", nil); status != http.StatusNotFound {
		t.Errorf("show: status %d", status)
	} else {
		mustContain(t, body, "Da ist etwas schiefgegangen", "Nicht gefunden.", `href="/components"`)
	}
	status, _, _ := c.do("POST", "/bikes/does-not-exist/odometer", url.Values{"date": {"2026-10-25"}, "km": {"1"}})
	if status != http.StatusNotFound {
		t.Errorf("add reading: status %d", status)
	}
}

func TestCrossOriginRejected(t *testing.T) {
	c := newClient(t)
	req, _ := http.NewRequest("POST", c.srv.URL+"/bikes", strings.NewReader("name=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://evil.example")
	res, err := c.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status %d, want 403", res.StatusCode)
	}
}
