package server_test

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/auth"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

const testGPX = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test" xmlns="http://www.topografix.com/GPX/1/1">
  <trk><name>Evening ride</name><trkseg>
    <trkpt lat="52.0" lon="13.0"><time>2026-06-30T16:30:00Z</time></trkpt>
    <trkpt lat="52.02" lon="13.0"><time>2026-06-30T16:40:00Z</time></trkpt>
  </trkseg></trk>
</gpx>`

var (
	rideField = regexp.MustCompile(`name="ride\.0" value="([^"]*)"`)
	rideEdit  = regexp.MustCompile(`/rides/[0-9a-f-]{36}/edit`)
)

func TestImportRides(t *testing.T) {
	c := newClient(t)
	bikePath := c.createBike("Gravel")
	bikeID := strings.TrimPrefix(bikePath, "/bikes/")

	status, _, body := c.uploadFiles("/rides/import", map[string]string{"bike": bikeID}, "files",
		map[string]string{"evening.gpx": testGPX, "notes.txt": "hello"})
	if status != http.StatusOK {
		t.Fatalf("preview: status %d", status)
	}
	mustContain(t, body, "Evening ride", "notes.txt: keine FIT-, GPX- oder TCX-Datei", `name="import.0" value="1" aria-label="Importieren" checked`)
	m := rideField.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("no ride field in preview")
	}
	form := url.Values{
		"ride.0": {html.UnescapeString(m[1])}, "import.0": {"1"}, "bike.0": {"auto"}, "default_bike": {bikeID},
	}
	status, path, body := c.do("POST", "/rides/import/confirm", form)
	if status != http.StatusOK || path != "/rides" {
		t.Fatalf("import: %d %s", status, path)
	}
	mustContain(t, body, "1 Fahrt importiert.", "Evening ride", "Gravel", "2,2 km")

	// The bike's odometer counts the ride.
	_, _, body = c.do("GET", bikePath, nil)
	mustContain(t, body, "2,2 km")

	// The same file again is recognized.
	_, _, body = c.uploadFiles("/rides/import", nil, "files", map[string]string{"evening.gpx": testGPX})
	mustContain(t, body, "Schon importiert", "Alle Fahrten waren schon importiert.")

	// Tampered rides are rejected.
	form.Set("ride.0", `{"Source":"other"}`)
	if status, _, _ := c.do("POST", "/rides/import/confirm", form); status != http.StatusBadRequest {
		t.Fatalf("tampered: status %d", status)
	}

	// Discard it: it no longer counts.
	_, _, body = c.do("GET", "/rides", nil)
	edit := rideEdit.FindString(body)
	_, _, body = c.do("GET", edit, nil)
	mustContain(t, body, "Verwerfen")
	if status, path, _ := c.do("POST", strings.TrimSuffix(edit, "/edit")+"/discard", url.Values{}); status != http.StatusOK || path != "/rides" {
		t.Fatalf("discard: %d %s", status, path)
	}
	_, _, body = c.do("GET", "/rides?bike="+bikeID, nil)
	mustContain(t, body, "Keine Fahrt passt zum Filter.")

	// Delete it: importing the file again adds it again.
	_, _, body = c.do("GET", strings.TrimSuffix(edit, "/edit")+"/delete", nil)
	mustContain(t, body, "Fahrt vom 30.06.2026 (2,2 km) löschen?")
	if status, path, _ := c.do("POST", strings.TrimSuffix(edit, "/edit")+"/delete", url.Values{}); status != http.StatusOK || path != "/rides" {
		t.Fatalf("delete: %d %s", status, path)
	}
	_, _, body = c.uploadFiles("/rides/import", nil, "files", map[string]string{"evening.gpx": testGPX})
	mustContain(t, body, `name="import.0" value="1" aria-label="Importieren" checked`)
}

// All rides a filter shows can be discarded or deleted at once, e.g. those
// of an old bike in Garmin Connect.
func TestBulkRides(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	now := time.Now().UTC()
	gear, err := c.st.Rides().CreateRideGear(ctx, store.RideGear{
		ID: "g1", OwnerID: auth.FirstUserID, Source: "fit", GearKey: "gear:old", Name: "old", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, key := range []string{"gear:old", "gear:old", ""} {
		start := time.Date(2026, 5, 1+i, 8, 0, 0, 0, time.UTC)
		_, err := c.st.Rides().CreateRide(ctx, store.Ride{
			ID: fmt.Sprintf("r%d", i), OwnerID: auth.FirstUserID, Source: "fit", ExternalID: fmt.Sprint(i), GearKey: key,
			StartedAt: start, EndedAt: start.Add(time.Hour), RiddenOn: start.Truncate(24 * time.Hour),
			DistanceM: 10_000, Name: fmt.Sprintf("Ride %d", i), CreatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	_, _, body := c.do("GET", "/rides?year=all", nil)
	if strings.Contains(body, "/rides/filtered/") {
		t.Fatal("bulk actions without filter")
	}
	_, _, body = c.do("GET", "/rides?gear="+gear.ID, nil)
	mustContain(t, body, "2 Fahrten", `href="/rides/filtered/delete?gear=g1&amp;year=2026"`, "Alle angezeigten verwerfen")
	_, _, body = c.do("GET", "/rides/filtered/delete?gear="+gear.ID+"&year=all", nil)
	mustContain(t, body, "2 Fahrten (20 km) löschen?", `action="/rides/filtered/delete?gear=g1&amp;year=all"`,
		`href="/rides?gear=g1&amp;year=all"`)

	status, path, _ := c.do("POST", "/rides/filtered/delete?gear="+gear.ID+"&year=all", url.Values{})
	if status != http.StatusOK || path != "/rides" {
		t.Fatalf("delete: %d %s", status, path)
	}
	if list, _ := c.st.Rides().ListRides(ctx, auth.FirstUserID); len(list) != 1 {
		t.Fatalf("%d rides left", len(list))
	}

	_, _, body = c.do("POST", "/rides/filtered/discard?bike=none", url.Values{})
	mustContain(t, body, "Ride 2", "Verworfen")
	if list, _ := c.st.Rides().ListRides(ctx, auth.FirstUserID); len(list) != 1 || !list[0].Discarded {
		t.Fatalf("rides = %+v", list)
	}

	if status, _, _ := c.do("POST", "/rides/filtered/delete", url.Values{}); status != http.StatusNotFound {
		t.Fatalf("without filter: status %d", status)
	}
}

// Without year, the list shows the newest year with rides.
func TestRidesDefaultYear(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	for i, year := range []int{2024, 2025} {
		start := time.Date(year, 5, 1, 8, 0, 0, 0, time.UTC)
		_, err := c.st.Rides().CreateRide(ctx, store.Ride{
			ID: fmt.Sprintf("r%d", i), OwnerID: auth.FirstUserID, Source: "fit", ExternalID: fmt.Sprint(i),
			StartedAt: start, EndedAt: start.Add(time.Hour), RiddenOn: start.Truncate(24 * time.Hour),
			DistanceM: 10_000, Name: fmt.Sprintf("Ride %d", year), CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	_, _, body := c.do("GET", "/rides", nil)
	mustContain(t, body, "Ride 2025", `<option value="2025" selected>`, `href="/rides?year=all"`)
	if strings.Contains(body, "Ride 2024") {
		t.Fatal("older year shown by default")
	}
	_, _, body = c.do("GET", "/rides?year=all", nil)
	mustContain(t, body, "Ride 2024", "Ride 2025", `<option value="all" selected>`)
}
