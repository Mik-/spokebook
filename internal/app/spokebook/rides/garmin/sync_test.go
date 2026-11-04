package garmin_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"

	"github.com/Mik-/spokebook/internal/app/spokebook/odometer"
	"github.com/Mik-/spokebook/internal/app/spokebook/rides"
	"github.com/Mik-/spokebook/internal/app/spokebook/rides/garmin"
	"github.com/Mik-/spokebook/internal/app/spokebook/secret"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
	"github.com/Mik-/spokebook/internal/app/spokebook/store/sqlite"
)

const (
	owner  = "o1"
	apiKey = "GCDC_test-key"
	gearID = "07411423-ebc0-4657-a4a0-d68864fed343"
)

// fakeGarmin serves the Personal API for the activities, one ride of
// 10 km per day from 2026-03-01 on.
type fakeGarmin struct {
	t *testing.T

	mu        sync.Mutex
	acts      []activity
	tokens    int
	lists     []string // startDate of each list request
	downloads int
	// failAt makes the download with this number (1-based) fail with
	// status.
	failAt, status int
	// holdAt makes the download with this number wait until hold is
	// closed; reached tells it got there.
	holdAt        int
	hold, reached chan struct{}
}

type activity struct {
	ID             int64  `json:"activityId"`
	BeginTimestamp int64  `json:"beginTimestamp"`
	Name           string `json:"activityName"`
	Manual         bool   `json:"manualActivity"`
}

func (f *fakeGarmin) add(n int) {
	for range n {
		i := len(f.acts)
		start := time.Date(2026, 3, 1+i, 8, 0, 0, 0, time.UTC)
		f.acts = append(f.acts, activity{ID: int64(1000 + i), BeginTimestamp: start.UnixMilli(), Name: fmt.Sprintf("Ride %d", i)})
	}
}

func (f *fakeGarmin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/personalapitoken-service/personal-api/personal-api-key/token":
		if r.Header.Get("Authorization") != "ApiKey "+apiKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.tokens++
		claims, _ := json.Marshal(map[string]any{"exp": time.Now().Add(24 * time.Hour).Unix()})
		_, _ = io.WriteString(w, "x."+base64.RawURLEncoding.EncodeToString(claims)+".y")
		return
	case r.Header.Get("Authorization") == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer x."):
		w.WriteHeader(http.StatusUnauthorized)
		return
	case r.URL.Path == "/activitylist-service/activities/search/activities":
		q := r.URL.Query()
		f.lists = append(f.lists, q.Get("startDate"))
		since, _ := time.Parse(time.DateOnly, q.Get("startDate"))
		start, _ := strconv.Atoi(q.Get("start"))
		limit, _ := strconv.Atoi(q.Get("limit"))
		var list []activity
		for _, a := range f.acts {
			if !time.UnixMilli(a.BeginTimestamp).Before(since) {
				list = append(list, a)
			}
		}
		list = list[min(start, len(list)):min(start+limit, len(list))]
		_ = json.NewEncoder(w).Encode(list)
	case strings.HasPrefix(r.URL.Path, "/download-service/files/activity/"):
		f.downloads++
		if f.downloads == f.holdAt {
			f.mu.Unlock()
			close(f.reached)
			<-f.hold
			f.mu.Lock()
		}
		if f.downloads == f.failAt {
			w.WriteHeader(f.status)
			return
		}
		id, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/download-service/files/activity/"), 10, 64)
		for _, a := range f.acts {
			if a.ID == id {
				_, _ = w.Write(zipped(f.t, id, time.UnixMilli(a.BeginTimestamp).UTC()))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// zipped returns a ZIP archive with a FIT file of a 10 km ride with the
// Garmin gear.
func zipped(t *testing.T, id int64, start time.Time) []byte {
	t.Helper()
	gear := proto.Message{Num: 526, Fields: []proto.Field{{
		FieldBase: &proto.FieldBase{Name: "unknown", Num: 0, BaseType: basetype.String, Scale: 1},
		Value:     proto.String(gearID),
	}}}
	msgs := []proto.Message{
		mesgdef.NewFileId(nil).SetType(typedef.FileActivity).SetManufacturer(typedef.ManufacturerGarmin).
			SetSerialNumber(3412345).SetTimeCreated(start).ToMesg(nil),
		gear,
		mesgdef.NewSession(nil).SetStartTime(start).SetSport(typedef.SportCycling).SetTotalDistance(1_000_000).
			SetTotalElapsedTime(3_600_000).SetTimestamp(start.Add(time.Hour)).ToMesg(nil),
		mesgdef.NewActivity(nil).SetTimestamp(start.Add(time.Hour)).SetLocalTimestamp(start.Add(time.Hour)).ToMesg(nil),
	}
	var fit bytes.Buffer
	if err := encoder.New(&fit).Encode(&proto.FIT{Messages: msgs}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(strconv.FormatInt(id, 10) + "_ACTIVITY.fit")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(fit.Bytes())
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func setup(t *testing.T) (context.Context, store.Store, *fakeGarmin, *garmin.Syncer) {
	t.Helper()
	ctx := context.Background()
	st, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = st.Bikes().CreateBike(ctx, store.Bike{
		ID: "gravel", OwnerID: owner, Name: "Gravel", InServiceAt: new(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		InServiceM: new(int64(0)), CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeGarmin{t: t}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	box, err := secret.New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	client := garmin.NewClient(srv.URL)
	client.Wait = 0
	return ctx, st, fake, garmin.NewSyncer(st, box, client, slog.New(slog.DiscardHandler))
}

func source(t *testing.T, ctx context.Context, st store.Store) store.RideSource {
	t.Helper()
	src, err := st.Rides().GetRideSource(ctx, owner, garmin.Source)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

var since = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

func TestSync(t *testing.T) {
	ctx, st, fake, syncer := setup(t)
	fake.add(3)
	fake.acts[1].Manual = true

	if err := syncer.Connect(ctx, owner, "wrong", since); err != garmin.ErrUnauthorized {
		t.Fatalf("wrong key: %v", err)
	}
	if err := syncer.Connect(ctx, owner, apiKey, since); err != nil {
		t.Fatal(err)
	}
	src := source(t, ctx, st)
	if bytes.Contains(src.Credentials, []byte(apiKey)) {
		t.Fatal("API key stored in plain text")
	}
	if err := syncer.Sync(ctx, owner); err != nil {
		t.Fatal(err)
	}
	src = source(t, ctx, st)
	// The manual activity has no file.
	if src.LastError != "" || src.LastImported != 2 || src.LastUnassigned != 2 || src.More || fake.downloads != 2 {
		t.Fatalf("run = %+v, downloads %d", src.RideSourceRun, fake.downloads)
	}
	list, err := st.Rides().ListRides(ctx, owner)
	if err != nil || len(list) != 2 || list[0].Name != "Ride 2" || list[0].Source != "fit" || list[0].GearKey != "gear:"+gearID {
		t.Fatalf("rides = %+v, err = %v", list, err)
	}

	// Mapping the gear assigns the rides; the next run only fetches the
	// new activity and reuses the token.
	gears, err := st.Rides().ListRideGears(ctx, owner)
	if err != nil || len(gears) != 1 {
		t.Fatalf("gears = %+v, err = %v", gears, err)
	}
	if err := syncer.Connect(ctx, owner, "", since); err != nil {
		t.Fatal(err)
	}
	if err := rides.SetGearBike(ctx, st, owner, gears[0].ID, "gravel", true); err != nil {
		t.Fatal(err)
	}
	fake.add(1)
	if err := syncer.Sync(ctx, owner); err != nil {
		t.Fatal(err)
	}
	src = source(t, ctx, st)
	if src.LastImported != 1 || src.LastUnassigned != 0 || fake.downloads != 3 || fake.tokens != 1 {
		t.Fatalf("run = %+v, downloads %d, tokens %d", src.RideSourceRun, fake.downloads, fake.tokens)
	}
	// The run looked back 7 days before the newest activity.
	if got := fake.lists[len(fake.lists)-1]; got != "2026-02-24" {
		t.Fatalf("startDate = %s", got)
	}
	latest, err := odometer.Latest(ctx, st, owner)
	if err != nil || latest["gravel"] != 30_000 {
		t.Fatalf("odometer = %v, err = %v", latest, err)
	}
}

// A first import over many activities is spread over runs; a rate limit
// stops a run but keeps what it fetched.
func TestSyncLimits(t *testing.T) {
	ctx, st, fake, syncer := setup(t)
	fake.add(120)
	if err := syncer.Connect(ctx, owner, apiKey, since); err != nil {
		t.Fatal(err)
	}
	if err := syncer.Sync(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if src := source(t, ctx, st); src.LastImported != 100 || !src.More || src.LastError != "" {
		t.Fatalf("run = %+v", src.RideSourceRun)
	}
	fake.failAt, fake.status = 110, http.StatusTooManyRequests
	if err := syncer.Sync(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if src := source(t, ctx, st); src.LastImported != 9 || src.LastError != garmin.ErrorRateLimited {
		t.Fatalf("run = %+v", src.RideSourceRun)
	}
	if err := syncer.Sync(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if src := source(t, ctx, st); src.LastImported != 11 || src.More || src.LastError != "" {
		t.Fatalf("run = %+v", src.RideSourceRun)
	}
	list, err := st.Rides().ListRides(ctx, owner)
	if err != nil || len(list) != 120 {
		t.Fatalf("%d rides, err = %v", len(list), err)
	}
}

// A refused token is dropped, so the next run gets a new one.
func TestSyncUnauthorized(t *testing.T) {
	ctx, st, fake, syncer := setup(t)
	fake.add(2)
	if err := syncer.Connect(ctx, owner, apiKey, since); err != nil {
		t.Fatal(err)
	}
	fake.failAt, fake.status = 1, http.StatusUnauthorized
	if err := syncer.Sync(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if src := source(t, ctx, st); src.LastError != garmin.ErrorUnauthorized {
		t.Fatalf("run = %+v", src.RideSourceRun)
	}
	if err := syncer.Sync(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if src := source(t, ctx, st); src.LastError != "" || src.LastImported != 2 || fake.tokens != 2 {
		t.Fatalf("run = %+v, tokens %d", src.RideSourceRun, fake.tokens)
	}

	// Another secret key can't open the credentials.
	other, err := secret.New("another key, at least 32 characters long")
	if err != nil {
		t.Fatal(err)
	}
	client := garmin.NewClient("http://127.0.0.1:1")
	if err := garmin.NewSyncer(st, other, client, slog.New(slog.DiscardHandler)).Sync(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if src := source(t, ctx, st); src.LastError != garmin.ErrorSecret {
		t.Fatalf("run = %+v", src.RideSourceRun)
	}

	if err := syncer.Disconnect(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if list, err := st.Rides().ListRides(ctx, owner); err != nil || len(list) != 2 {
		t.Fatalf("%d rides after disconnect, err = %v", len(list), err)
	}
}

// While a run goes, its progress tells what it fetched so far.
func TestSyncProgress(t *testing.T) {
	ctx, st, fake, syncer := setup(t)
	fake.add(3)
	fake.holdAt, fake.hold, fake.reached = 2, make(chan struct{}), make(chan struct{})
	if err := syncer.Connect(ctx, owner, apiKey, since); err != nil {
		t.Fatal(err)
	}
	if _, ok := syncer.Progress(owner); ok {
		t.Fatal("progress before the run")
	}
	done := make(chan error)
	go func() { done <- syncer.Sync(ctx, owner) }()
	<-fake.reached
	p, ok := syncer.Progress(owner)
	if !ok || p.Checked != 1 || p.Imported != 1 || p.Unassigned != 1 || p.Started.IsZero() || !syncer.Pending(owner) {
		t.Fatalf("progress = %+v, %v", p, ok)
	}
	close(fake.hold)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, ok := syncer.Progress(owner); ok {
		t.Fatal("progress after the run")
	}
	if src := source(t, ctx, st); src.LastImported != 3 {
		t.Fatalf("run = %+v", src.RideSourceRun)
	}
}

// holdRun starts a run that waits in the download with number n; the
// download is released when the test ends.
func holdRun(t *testing.T, fake *fakeGarmin, n int) {
	t.Helper()
	fake.mu.Lock()
	fake.holdAt, fake.hold, fake.reached = n, make(chan struct{}), make(chan struct{})
	fake.mu.Unlock()
	hold := fake.hold
	t.Cleanup(func() { close(hold) })
}

// within fails the test if fn takes longer than a few seconds.
func within(t *testing.T, what string, fn func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("%s waits for the run", what)
	}
}

func TestDisconnectStopsRun(t *testing.T) {
	ctx, st, fake, syncer := setup(t)
	fake.add(3)
	holdRun(t, fake, 2)
	if err := syncer.Connect(ctx, owner, apiKey, since); err != nil {
		t.Fatal(err)
	}
	done := make(chan error)
	go func() { done <- syncer.Sync(ctx, owner) }()
	<-fake.reached
	within(t, "disconnect", func() error { return syncer.Disconnect(ctx, owner) })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := st.Rides().GetRideSource(ctx, owner, garmin.Source); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("source after disconnect: %v", err)
	}
	if rides, _ := st.Rides().ListRides(ctx, owner); len(rides) != 1 {
		t.Fatalf("%d rides, want the one fetched before", len(rides))
	}
}

func TestConnectDuringRun(t *testing.T) {
	ctx, st, fake, syncer := setup(t)
	fake.add(1)
	if err := syncer.Connect(ctx, owner, apiKey, since); err != nil {
		t.Fatal(err)
	}
	if err := syncer.Sync(ctx, owner); err != nil {
		t.Fatal(err)
	}
	// The run Connect queued runs now, with two more rides, and waits in
	// the second download.
	fake.mu.Lock()
	fake.add(2)
	fake.mu.Unlock()
	holdRun(t, fake, 3)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go syncer.Run(runCtx)
	<-fake.reached

	// Moving the first day back stops the run and queues a new one, which
	// fetches the rest.
	within(t, "connect", func() error { return syncer.Connect(ctx, owner, "", since.AddDate(0, -1, 0)) })
	deadline := time.Now().Add(5 * time.Second)
	for syncer.Pending(owner) || source(t, ctx, st).LastRunAt == nil {
		if time.Now().After(deadline) {
			t.Fatal("no run after connect")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if src := source(t, ctx, st); !src.Since.Equal(since.AddDate(0, -1, 0)) || src.More || src.LastError != "" {
		t.Fatalf("source %+v", src)
	}
	if rides, _ := st.Rides().ListRides(ctx, owner); len(rides) != 3 {
		t.Fatalf("%d rides, want 3", len(rides))
	}
}
