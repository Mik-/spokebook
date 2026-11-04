package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/auth"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

func TestSetupAndLogin(t *testing.T) {
	c := newAnonymousClient(t)

	// Without an account everything leads to setup.
	status, path, body := c.do("GET", "/bikes/new", nil)
	if status != http.StatusOK || path != "/setup" {
		t.Fatalf("no account: %d %s", status, path)
	}
	mustContain(t, body, "Spokebook einrichten", `name="token"`)
	if strings.Contains(body, `href="/components"`) {
		t.Error("navigation shown without login")
	}

	m := regexp.MustCompile(`/setup\?token=([A-Z0-9]+)`).FindStringSubmatch(c.log.String())
	if m == nil {
		t.Fatalf("no setup link in log: %s", c.log)
	}
	token := m[1]
	_, _, body = c.do("GET", "/setup?token="+token, nil)
	mustContain(t, body, `type="hidden" name="token" value="`+token+`"`)

	form := url.Values{"token": {"wrong"}, "name": {"Rider"}, "password": {testPassword}, "password_confirm": {testPassword}}
	status, _, body = c.do("POST", "/setup", form)
	if status != http.StatusUnprocessableEntity {
		t.Errorf("wrong token: %d", status)
	}
	mustContain(t, body, "Das Setup-Token ist falsch.")
	form.Set("token", token)
	form.Set("password_confirm", "something else")
	_, _, body = c.do("POST", "/setup", form)
	mustContain(t, body, "Die Passwörter stimmen nicht überein.", `value="Rider"`)
	form.Set("password_confirm", testPassword)
	status, path, body = c.do("POST", "/setup", form)
	if status != http.StatusOK || path != "/account" {
		t.Fatalf("setup: %d %s", status, path)
	}
	mustContain(t, body, "<dd>Rider</dd>", "Noch keine Passkeys.", `action="/logout"`)

	// Setup is closed now, and its data belongs to the first user.
	if _, path, _ := c.do("GET", "/setup?token="+token, nil); path == "/setup" {
		t.Errorf("setup after account: %s", path)
	}
	c.createBike("Gravel")
	if bikes, _ := c.st.Bikes().ListBikes(context.Background(), auth.FirstUserID); len(bikes) != 1 {
		t.Errorf("bikes of first user: %d", len(bikes))
	}

	c.do("POST", "/logout", url.Values{})
	status, path, _ = c.do("GET", "/bikes/new?x=1", nil)
	if status != http.StatusOK || path != "/login" {
		t.Fatalf("after logout: %d %s", status, path)
	}
	status, path, _ = c.do("POST", "/bikes", url.Values{"name": {"Sneaky"}})
	if path != "/login" {
		t.Errorf("post without login: %d %s", status, path)
	}
	if bikes, _ := c.st.Bikes().ListBikes(context.Background(), auth.FirstUserID); len(bikes) != 1 {
		t.Errorf("bike created without login: %d", len(bikes))
	}

	status, _, body = c.do("POST", "/login", url.Values{"name": {"rider"}, "password": {"wrong password"}, "next": {"/bikes/new?x=1"}})
	if status != http.StatusUnauthorized {
		t.Errorf("wrong password: %d", status)
	}
	mustContain(t, body, "Name oder Passwort falsch.", `value="/bikes/new?x=1"`, `data-passkey-login`, `src="/static/passkey.js"`)

	status, path, _ = c.do("POST", "/login", url.Values{"name": {"rider"}, "password": {testPassword}, "next": {"/bikes/new?x=1"}})
	if status != http.StatusOK || path != "/bikes/new" {
		t.Fatalf("login: %d %s", status, path)
	}
	if _, path, _ := c.do("GET", "/login", nil); path != "/" {
		t.Errorf("login page while logged in: %s", path)
	}
}

func TestLoginRedirectsOnlyLocally(t *testing.T) {
	c := newSeededClient(t)
	c.do("POST", "/logout", url.Values{})
	for _, next := range []string{"//evil.example", "https://evil.example", "/\\evil.example", "/\t/evil.example"} {
		_, path, _ := c.do("POST", "/login", url.Values{"name": {"rider"}, "password": {testPassword}, "next": {next}})
		if path != "/" {
			t.Errorf("next %q: landed on %s", next, path)
		}
		c.do("POST", "/logout", url.Values{})
	}
}

func TestLoginLockout(t *testing.T) {
	c := newSeededClient(t)
	c.do("POST", "/logout", url.Values{})
	var status int
	var body string
	for range auth.MaxFailedLogins {
		status, _, body = c.do("POST", "/login", url.Values{"name": {"rider"}, "password": {"wrong password"}})
	}
	if status != http.StatusTooManyRequests {
		t.Errorf("status %d, want 429", status)
	}
	mustContain(t, body, "Zu viele Fehlversuche.")

	// The client limit stops guessing across names, too.
	for range 10 {
		status, _, body = c.do("POST", "/login", url.Values{"name": {"other"}, "password": {"wrong password"}})
	}
	if status != http.StatusTooManyRequests {
		t.Errorf("rate limit: status %d", status)
	}
	mustContain(t, body, "Zu viele Versuche.")
}

func TestChangePasswordPage(t *testing.T) {
	c := newSeededClient(t)
	_, _, body := c.do("GET", "/account", nil)
	mustContain(t, body, `href="/account/password"`, `href="/account/passkeys/new"`)

	status, _, body := c.do("POST", "/account/password", url.Values{
		"current_password": {"wrong password"}, "password": {"new password 123"}, "password_confirm": {"new password 123"},
	})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("wrong current: %d", status)
	}
	mustContain(t, body, "Das aktuelle Passwort ist falsch.")
	_, _, body = c.do("POST", "/account/password", url.Values{
		"current_password": {testPassword}, "password": {"short"}, "password_confirm": {"short"},
	})
	mustContain(t, body, "Das Passwort braucht mindestens 10 Zeichen.")

	status, path, _ := c.do("POST", "/account/password", url.Values{
		"current_password": {testPassword}, "password": {"new password 123"}, "password_confirm": {"new password 123"},
	})
	if status != http.StatusOK || path != "/account" {
		t.Fatalf("change: %d %s", status, path)
	}
	c.do("POST", "/logout", url.Values{})
	if _, path, _ := c.do("POST", "/login", url.Values{"name": {"rider"}, "password": {"new password 123"}}); path != "/" {
		t.Errorf("login with new password: %s", path)
	}
}

func TestPasskeyPages(t *testing.T) {
	c := newSeededClient(t)
	_, _, body := c.do("GET", "/account/passkeys/new", nil)
	mustContain(t, body, `src="/static/passkey.js"`, "data-passkey-register", "data-passkey-unsupported")

	status, _, body := c.do("POST", "/account/passkeys/begin", url.Values{"name": {"Phone"}})
	if status != http.StatusOK {
		t.Fatalf("begin: %d %s", status, body)
	}
	var opts struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
			RP        struct{ ID string }
			User      struct{ Name string }
		} `json:"publicKey"`
	}
	if err := json.Unmarshal([]byte(body), &opts); err != nil {
		t.Fatal(err)
	}
	if opts.PublicKey.Challenge == "" || opts.PublicKey.RP.ID != "localhost" || opts.PublicKey.User.Name != "rider" {
		t.Errorf("options: %+v", opts)
	}
	status, _, body = c.do("POST", "/account/passkeys/begin", url.Values{"name": {""}})
	if status != http.StatusBadRequest || !strings.Contains(body, `"error":"Bitte einen Namen angeben."`) {
		t.Errorf("begin without name: %d %s", status, body)
	}

	// A broken answer fails with a message for the page.
	req, _ := http.NewRequest("POST", c.srv.URL+"/account/passkeys/finish", strings.NewReader("{}"))
	req.Header.Set("Accept-Language", "de")
	req.Header.Set("Origin", c.srv.URL)
	res, err := c.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("finish: %d", res.StatusCode)
	}

	// Listing and deleting.
	ctx := context.Background()
	k := store.Passkey{ID: "k1", UserID: auth.FirstUserID, CredentialID: []byte{1}, Name: "YubiKey", Credential: []byte("{}"), CreatedAt: time.Now().UTC()}
	if err := c.st.Users().CreatePasskey(ctx, k); err != nil {
		t.Fatal(err)
	}
	_, _, body = c.do("GET", "/account", nil)
	mustContain(t, body, "<td>YubiKey</td>", `href="/account/passkeys/k1/delete"`)
	_, _, body = c.do("GET", "/account/passkeys/k1/delete", nil)
	mustContain(t, body, "Den Passkey „YubiKey“ löschen?")
	status, path, body := c.do("POST", "/account/passkeys/k1/delete", url.Values{})
	if status != http.StatusOK || path != "/account" {
		t.Fatalf("delete: %d %s", status, path)
	}
	mustContain(t, body, "Noch keine Passkeys.")

	status, _, body = c.do("POST", "/login/passkey/begin", url.Values{})
	if status != http.StatusOK || !strings.Contains(body, `"challenge"`) {
		t.Errorf("login begin: %d %s", status, body)
	}
}
