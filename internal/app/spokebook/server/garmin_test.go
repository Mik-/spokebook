package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/spokebook/spokebook/internal/app/spokebook/auth"
	"github.com/spokebook/spokebook/internal/app/spokebook/rides/garmin"
	"github.com/spokebook/spokebook/internal/app/spokebook/secret"
	"github.com/spokebook/spokebook/internal/app/spokebook/server"
	"github.com/spokebook/spokebook/internal/app/spokebook/store"
)

func TestGarminDisabled(t *testing.T) {
	c := newClient(t)
	_, _, body := c.do("GET", "/rides/garmin", nil)
	mustContain(t, body, "APP_SECRET_KEY")
	if status, _, _ := c.do("POST", "/rides/garmin/connect", url.Values{"key": {"k"}, "since": {"2026-01-01"}}); status != http.StatusUnprocessableEntity {
		t.Fatalf("connect: status %d", status)
	}
}

func TestGarmin(t *testing.T) {
	// Garmin knows one key and has no activities.
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/personalapitoken-service/personal-api/personal-api-key/token":
			if r.Header.Get("Authorization") != "ApiKey good" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, "token")
		default:
			_, _ = io.WriteString(w, "[]")
		}
	}))
	t.Cleanup(fake.Close)
	box, err := secret.New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	var syncer *garmin.Syncer
	c := newSeededClientWith(t, func(st store.Store) server.Config {
		client := garmin.NewClient(fake.URL)
		client.Wait = 0
		syncer = garmin.NewSyncer(st, box, client, slog.New(slog.DiscardHandler))
		return server.Config{BaseURL: "http://localhost", Garmin: syncer}
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go syncer.Run(ctx)

	c.createBike("Gravel")
	_, _, body := c.do("GET", "/rides", nil)
	mustContain(t, body, `href="/rides/garmin"`)
	_, _, body = c.do("GET", "/rides/garmin", nil)
	mustContain(t, body, "Holt einmal am Tag", `href="/rides/garmin/connect"`)
	_, _, body = c.do("GET", "/rides/garmin/connect", nil)
	mustContain(t, body, `name="key"`, `value="`+time.Now().Format(time.DateOnly)+`"`)

	form := url.Values{"key": {"wrong"}, "since": {"2026-01-01"}}
	status, _, body := c.do("POST", "/rides/garmin/connect", form)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("wrong key: status %d", status)
	}
	mustContain(t, body, "Garmin hat den API-Key abgelehnt.")
	form.Set("since", "2999-01-01")
	if status, _, _ := c.do("POST", "/rides/garmin/connect", form); status != http.StatusUnprocessableEntity {
		t.Fatalf("future: status %d", status)
	}

	form = url.Values{"key": {"good"}, "since": {"2026-01-01"}}
	if status, path, _ := c.do("POST", "/rides/garmin/connect", form); status != http.StatusOK || path != "/rides/garmin" {
		t.Fatalf("connect: %d %s", status, path)
	}
	for i := 0; syncer.Pending(auth.FirstUserID); i++ {
		if i == 100 {
			t.Fatal("run doesn't end")
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, _, body = c.do("GET", "/rides/garmin", nil)
	mustContain(t, body, "Letzter Abruf", "0 Fahrten importiert.", "Abgerufen werden Fahrten ab 01.01.2026.")
	if strings.Contains(body, "Abgerufen bis") {
		t.Fatal("fetched up to without activities")
	}
	cursor, at := time.Date(2026, 5, 14, 15, 0, 0, 0, time.UTC), time.Now().UTC()
	err = c.st.Rides().SetRideSourceRun(context.Background(), auth.FirstUserID, garmin.Source, store.RideSourceRun{
		Cursor: &cursor, LastRunAt: &at, LastImported: 100, More: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, body = c.do("GET", "/rides/garmin", nil)
	mustContain(t, body, "Abgerufen bis zur Fahrt vom 14.05.2026. Es sind noch Fahrten übrig")

	// Another run right after is refused.
	_, _, body = c.do("POST", "/rides/garmin/sync", url.Values{})
	mustContain(t, body, "erst wenige Minuten her")

	// Settings keep the key when left empty.
	_, _, body = c.do("GET", "/rides/garmin/connect", nil)
	mustContain(t, body, "Leer lassen", `value="2026-01-01"`, "Abgerufen bis zur Fahrt vom 14.05.2026. Ein früherer Tag")
	if status, path, _ := c.do("POST", "/rides/garmin/connect", url.Values{"key": {""}, "since": {"2026-02-01"}}); status != http.StatusOK || path != "/rides/garmin" {
		t.Fatalf("settings: %d %s", status, path)
	}

	_, _, body = c.do("GET", "/rides/garmin/disconnect", nil)
	mustContain(t, body, "Garmin Connect trennen?")
	if status, _, body := c.do("POST", "/rides/garmin/disconnect", url.Values{}); status != http.StatusOK {
		t.Fatalf("disconnect: status %d", status)
	} else {
		mustContain(t, body, "Holt einmal am Tag")
	}

	// Connecting again starts at the newest ride.
	start := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	_, err = c.st.Rides().CreateRide(context.Background(), store.Ride{
		ID: "r1", OwnerID: auth.FirstUserID, Source: "fit", ExternalID: "1", StartedAt: start, EndedAt: start.Add(time.Hour),
		RiddenOn: start.Truncate(24 * time.Hour), DistanceM: 1000, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, body = c.do("GET", "/rides/garmin/connect", nil)
	mustContain(t, body, `value="2026-05-01"`, "Vorbelegt mit dem Tag deiner neuesten Fahrt")
}

// While a run goes, the page shows the rides fetched so far.
func TestGarminProgress(t *testing.T) {
	reached, hold := make(chan struct{}), make(chan struct{})
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/personalapitoken-service/personal-api/personal-api-key/token" {
			_, _ = io.WriteString(w, "token")
			return
		}
		close(reached)
		<-hold
		_, _ = io.WriteString(w, "[]")
	}))
	t.Cleanup(fake.Close)
	t.Cleanup(func() { close(hold) })
	box, err := secret.New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	var syncer *garmin.Syncer
	c := newSeededClientWith(t, func(st store.Store) server.Config {
		client := garmin.NewClient(fake.URL)
		client.Wait = 0
		syncer = garmin.NewSyncer(st, box, client, slog.New(slog.DiscardHandler))
		return server.Config{BaseURL: "http://localhost", Garmin: syncer}
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go syncer.Run(ctx)
	if status, _, _ := c.do("POST", "/rides/garmin/connect", url.Values{"key": {"good"}, "since": {"2026-01-01"}}); status != http.StatusOK {
		t.Fatalf("connect: status %d", status)
	}
	<-reached

	// A ride the run created.
	start := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	_, err = c.st.Rides().CreateRide(context.Background(), store.Ride{
		ID: "r1", OwnerID: auth.FirstUserID, Source: "fit", ExternalID: "1", StartedAt: start, EndedAt: start.Add(time.Hour),
		RiddenOn: start.Truncate(24 * time.Hour), DistanceM: 12_300, Name: "Morning ride", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, body := c.do("GET", "/rides/garmin", nil)
	mustContain(t, body, `hx-trigger="every 3s"`, "0 Aktivitäten geprüft", "Morning ride", "12,3 km", "Nicht zugeordnet")
}
