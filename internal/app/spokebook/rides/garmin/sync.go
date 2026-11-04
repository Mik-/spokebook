package garmin

import (
	"archive/zip"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/Mik-/spokebook/internal/app/spokebook/rides"
	"github.com/Mik-/spokebook/internal/app/spokebook/rides/parse"
	"github.com/Mik-/spokebook/internal/app/spokebook/secret"
	"github.com/Mik-/spokebook/internal/app/spokebook/store"
)

// Source is Garmin Connect as ride source. Its rides carry the source of
// the file (FIT), so files uploaded before are recognized.
const Source = "garmin"

const (
	// maxDownloads limits the activity files of one run; a first import
	// continues in the next runs.
	maxDownloads = 100
	pageSize     = 50
	// lookback is how far a run looks back before the newest activity
	// processed, for activities synced to Garmin late.
	lookback = 7 * 24 * time.Hour
	// every is the time between two runs; again while a run left
	// activities to fetch.
	every = 24 * time.Hour
	again = time.Hour
	// minManual is the time a run must be ago before another one can be
	// started by hand.
	minManual = 5 * time.Minute
	// checkEvery is how often the connected sources are checked for a
	// due run.
	checkEvery = 10 * time.Minute
	maxKeyLen  = 1000
)

// Why a run stopped, as stored in LastError.
const (
	// ErrorUnauthorized means the API key expired or was deleted.
	ErrorUnauthorized = "unauthorized"
	ErrorRateLimited  = "rate_limited"
	// ErrorSecret means the credentials can't be opened, e.g. because
	// APP_SECRET_KEY changed.
	ErrorSecret = "secret"
	ErrorFailed = "failed"
)

var (
	// ErrDisabled means APP_SECRET_KEY is not set, so no credentials can
	// be stored.
	ErrDisabled = errors.New("garmin: no secret key configured")
	// ErrNoKey means a source is connected without API key.
	ErrNoKey = errors.New("garmin: API key missing")
	// ErrTooSoon means the last run was less than minManual ago.
	ErrTooSoon = errors.New("garmin: last run too recent")
	// ErrUnreachable means Garmin could not be asked, e.g. no network.
	ErrUnreachable = errors.New("garmin: unreachable")
)

// credentials are sealed as JSON.
type credentials struct {
	APIKey  string    `json:"api_key"`
	Token   string    `json:"token,omitempty"`
	Expires time.Time `json:"expires,omitzero"`
}

// Syncer fetches the rides of the owners who connected Garmin Connect.
type Syncer struct {
	st     store.Store
	box    *secret.Box
	client *Client
	log    *slog.Logger

	// mu serializes runs and changes of the credentials.
	mu      sync.Mutex
	queue   chan string
	pmu     sync.Mutex
	pending map[string]bool
	running map[string]*Progress
	// stops cancel the owners' running runs.
	stops map[string]context.CancelFunc
}

// Progress tells how far a running run is.
type Progress struct {
	// Started is when the run started; rides created since come from it.
	Started time.Time
	// Checked counts the activities processed so far.
	Checked int
	// Imported and Unassigned count the rides imported so far.
	Imported   int
	Unassigned int
}

// NewSyncer returns a syncer; without box (no APP_SECRET_KEY) it is
// disabled.
func NewSyncer(st store.Store, box *secret.Box, client *Client, log *slog.Logger) *Syncer {
	return &Syncer{
		st: st, box: box, client: client, log: log, queue: make(chan string, 16), pending: map[string]bool{},
		running: map[string]*Progress{}, stops: map[string]context.CancelFunc{},
	}
}

// Enabled reports whether credentials can be stored.
func (s *Syncer) Enabled() bool { return s != nil && s.box != nil }

func purpose(ownerID string) string { return ownerID + "/" + Source }

// Connect checks the API key with Garmin and saves it with the first day
// whose rides are fetched, then queues a run. With an empty apiKey the
// stored key stays and only the day changes; moving it back fetches the
// earlier rides. A running run stops; the queued one goes on.
func (s *Syncer) Connect(ctx context.Context, ownerID, apiKey string, since time.Time) error {
	if !s.Enabled() {
		return ErrDisabled
	}
	s.stop(ownerID)
	s.mu.Lock()
	defer s.mu.Unlock()
	src, err := s.st.Rides().GetRideSource(ctx, ownerID, Source)
	found := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if !found && apiKey == "" {
		return ErrNoKey
	}
	if len(apiKey) > maxKeyLen {
		return ErrUnauthorized
	}
	if apiKey != "" {
		token, exp, err := s.client.Token(ctx, apiKey)
		if err != nil && !errors.Is(err, ErrUnauthorized) && !errors.Is(err, ErrRateLimited) && ctx.Err() == nil {
			return fmt.Errorf("%w: %w", ErrUnreachable, err)
		}
		if err != nil {
			return err
		}
		sealed, err := s.seal(ownerID, credentials{APIKey: apiKey, Token: token, Expires: exp})
		if err != nil {
			return err
		}
		if !found {
			err = s.st.Rides().CreateRideSource(ctx, store.RideSource{
				OwnerID: ownerID, Source: Source, Credentials: sealed, Since: since, CreatedAt: time.Now().UTC(),
			})
		} else {
			err = s.st.Rides().SetRideSourceCredentials(ctx, ownerID, Source, sealed)
		}
		if err != nil {
			return err
		}
	}
	if found && !since.Equal(src.Since) {
		cursor := src.Cursor
		if since.Before(src.Since) {
			cursor = nil
		}
		if err := s.st.Rides().SetRideSourceSince(ctx, ownerID, Source, since, cursor); err != nil {
			return err
		}
	}
	s.queueRun(ownerID)
	return nil
}

// Disconnect stops a running run and deletes the stored key; the rides
// stay.
func (s *Syncer) Disconnect(ctx context.Context, ownerID string) error {
	s.stop(ownerID)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st.Rides().DeleteRideSource(ctx, ownerID, Source)
}

// Trigger queues a run started by hand.
func (s *Syncer) Trigger(ctx context.Context, ownerID string) error {
	if !s.Enabled() {
		return ErrDisabled
	}
	src, err := s.st.Rides().GetRideSource(ctx, ownerID, Source)
	if err != nil {
		return err
	}
	if src.LastRunAt != nil && time.Since(*src.LastRunAt) < minManual {
		return ErrTooSoon
	}
	s.queueRun(ownerID)
	return nil
}

// Pending reports whether a run of the owner is queued or running.
func (s *Syncer) Pending(ownerID string) bool {
	s.pmu.Lock()
	defer s.pmu.Unlock()
	return s.pending[ownerID] || s.running[ownerID] != nil
}

// Progress returns how far the owner's running run is; false if none
// runs (it may still be queued).
func (s *Syncer) Progress(ownerID string) (Progress, bool) {
	s.pmu.Lock()
	defer s.pmu.Unlock()
	p := s.running[ownerID]
	if p == nil {
		return Progress{}, false
	}
	return *p, true
}

// track changes the progress of the owner's run.
func (s *Syncer) track(ownerID string, fn func(*Progress)) {
	s.pmu.Lock()
	defer s.pmu.Unlock()
	if p := s.running[ownerID]; p != nil {
		fn(p)
	}
}

// stop cancels the owner's run, so that changes of the credentials don't
// wait for it.
func (s *Syncer) stop(ownerID string) {
	s.pmu.Lock()
	defer s.pmu.Unlock()
	if cancel := s.stops[ownerID]; cancel != nil {
		cancel()
	}
}

func (s *Syncer) queueRun(ownerID string) {
	s.pmu.Lock()
	defer s.pmu.Unlock()
	if s.pending[ownerID] {
		return
	}
	select {
	case s.queue <- ownerID:
		s.pending[ownerID] = true
	default:
	}
}

// Run fetches the rides of every connected owner once a day (hourly while
// a run left activities to fetch) and when queued, until ctx is done.
func (s *Syncer) Run(ctx context.Context) {
	if !s.Enabled() {
		return
	}
	tick := time.NewTicker(checkEvery)
	defer tick.Stop()
	s.runDue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.runDue(ctx)
		case ownerID := <-s.queue:
			s.syncLogged(ctx, ownerID, true)
		}
	}
}

func (s *Syncer) runDue(ctx context.Context) {
	list, err := s.st.Rides().ListAllRideSources(ctx)
	if err != nil {
		s.log.Error("garmin: list sources", "err", err)
		return
	}
	now := time.Now()
	for _, src := range list {
		if src.Source == Source && due(src, now) {
			s.syncLogged(ctx, src.OwnerID, false)
		}
	}
}

// due reports whether the next run of the source is due.
func due(src store.RideSource, now time.Time) bool {
	if src.LastRunAt == nil {
		return true
	}
	wait := every
	if src.More && src.LastError == "" {
		wait = again
	}
	return now.Sub(*src.LastRunAt) >= wait
}

func (s *Syncer) syncLogged(ctx context.Context, ownerID string, queued bool) {
	if err := s.sync(ctx, ownerID, queued); err != nil && ctx.Err() == nil {
		s.log.Error("garmin: sync", "user", ownerID, "err", err)
	}
}

// Sync fetches the new rides of the owner and stores how the run went.
// Errors from Garmin end up in LastError; only store errors are
// returned. Connect and Disconnect stop it early (see stop); the next run
// goes on from there.
func (s *Syncer) Sync(ctx context.Context, ownerID string) error {
	return s.sync(ctx, ownerID, false)
}

// sync is Sync; a queued run stops being pending when it starts running,
// so that Connect can queue another one meanwhile.
func (s *Syncer) sync(ctx context.Context, ownerID string, queued bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	src, err := s.st.Rides().GetRideSource(ctx, ownerID, Source)
	if err != nil {
		s.pmu.Lock()
		if queued {
			delete(s.pending, ownerID)
		}
		s.pmu.Unlock()
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	now := time.Now().UTC()
	run := store.RideSourceRun{Cursor: src.Cursor, LastRunAt: &now}
	s.pmu.Lock()
	if queued {
		delete(s.pending, ownerID)
	}
	s.running[ownerID] = &Progress{Started: now}
	s.stops[ownerID] = cancel
	s.pmu.Unlock()
	defer func() {
		s.pmu.Lock()
		delete(s.running, ownerID)
		delete(s.stops, ownerID)
		s.pmu.Unlock()
	}()
	err = s.fetch(runCtx, src, &run)
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case runCtx.Err() != nil:
		run.More = true
	case errors.Is(err, ErrUnauthorized):
		run.LastError = ErrorUnauthorized
	case errors.Is(err, ErrRateLimited):
		run.LastError = ErrorRateLimited
	case errors.Is(err, secret.ErrOpen):
		run.LastError = ErrorSecret
	case err != nil:
		run.LastError = ErrorFailed
		s.log.Error("garmin: fetch", "user", ownerID, "err", err)
	}
	s.log.Info("garmin synced", "user", ownerID, "imported", run.LastImported, "unassigned", run.LastUnassigned,
		"more", run.More, "error", run.LastError)
	return s.st.Rides().SetRideSourceRun(ctx, ownerID, Source, run)
}

// fetch lists the activities from the cursor on and imports the new ones,
// oldest first, so the cursor only moves past processed activities.
func (s *Syncer) fetch(ctx context.Context, src store.RideSource, run *store.RideSourceRun) error {
	token, err := s.token(ctx, src)
	if err != nil {
		return err
	}
	from := src.Since
	if run.Cursor != nil && run.Cursor.Add(-lookback).After(from) {
		from = run.Cursor.Add(-lookback)
	}
	downloads := 0
	for start := 0; ; start += pageSize {
		list, err := s.client.Activities(ctx, token, from, start, pageSize)
		if err != nil {
			return s.dropToken(ctx, src, err)
		}
		for _, a := range list {
			id := strconv.FormatInt(a.ID, 10)
			seen, err := s.st.Rides().HasRideSourceItem(ctx, src.OwnerID, Source, id)
			if err != nil {
				return err
			}
			if seen {
				continue
			}
			if downloads == maxDownloads {
				run.More = true
				return nil
			}
			// A manual activity has no file.
			if !a.Manual {
				downloads++
				res, err := s.importActivity(ctx, src.OwnerID, token, a)
				if err != nil {
					return s.dropToken(ctx, src, err)
				}
				run.LastImported += res.Imported
				run.LastUnassigned += res.Unassigned
				s.track(src.OwnerID, func(p *Progress) {
					p.Imported += res.Imported
					p.Unassigned += res.Unassigned
				})
			}
			s.track(src.OwnerID, func(p *Progress) { p.Checked++ })
			if err := s.st.Rides().AddRideSourceItem(ctx, src.OwnerID, Source, id, time.Now().UTC()); err != nil {
				return err
			}
			if start := a.Start(); run.Cursor == nil || start.After(*run.Cursor) {
				run.Cursor = &start
			}
		}
		if len(list) < pageSize {
			return nil
		}
	}
}

// token returns the stored access token while it is valid for another
// hour, else a new one.
func (s *Syncer) token(ctx context.Context, src store.RideSource) (string, error) {
	c, err := s.open(src)
	if err != nil {
		return "", err
	}
	if c.Token != "" && time.Now().Add(time.Hour).Before(c.Expires) {
		return c.Token, nil
	}
	if c.Token, c.Expires, err = s.client.Token(ctx, c.APIKey); err != nil {
		return "", err
	}
	sealed, err := s.seal(src.OwnerID, c)
	if err != nil {
		return "", err
	}
	return c.Token, s.st.Rides().SetRideSourceCredentials(ctx, src.OwnerID, Source, sealed)
}

// dropToken forgets the stored token if Garmin refused it, so the next
// run gets a new one; it returns err.
func (s *Syncer) dropToken(ctx context.Context, src store.RideSource, err error) error {
	if !errors.Is(err, ErrUnauthorized) {
		return err
	}
	c, oerr := s.open(src)
	if oerr != nil {
		return err
	}
	c.Token, c.Expires = "", time.Time{}
	if sealed, serr := s.seal(src.OwnerID, c); serr == nil {
		if serr := s.st.Rides().SetRideSourceCredentials(ctx, src.OwnerID, Source, sealed); serr != nil {
			return serr
		}
	}
	return err
}

func (s *Syncer) open(src store.RideSource) (credentials, error) {
	var c credentials
	plain, err := s.box.Open(src.Credentials, purpose(src.OwnerID))
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(plain, &c)
}

func (s *Syncer) seal(ownerID string, c credentials) ([]byte, error) {
	plain, err := json.Marshal(c) //nolint:gosec // G117: sealed right below, never stored in plain
	if err != nil {
		return nil, err
	}
	return s.box.Seal(plain, purpose(ownerID)), nil
}

// importActivity downloads the file of the activity and imports its
// rides. A file without ride is skipped.
func (s *Syncer) importActivity(ctx context.Context, ownerID, token string, a Activity) (rides.Result, error) {
	data, err := s.client.Download(ctx, token, a.ID)
	if err != nil {
		return rides.Result{}, err
	}
	read, err := readArchive(data)
	if err != nil {
		s.log.Info("garmin: activity skipped", "user", ownerID, "activity", a.ID, "err", err)
		return rides.Result{}, nil
	}
	for i := range read {
		if a.Name != "" {
			read[i].Name = a.Name
		}
	}
	return rides.ImportAuto(ctx, s.st, ownerID, read)
}

// readArchive reads the rides of the files in a ZIP archive, or of the
// data itself if it is no archive.
func readArchive(data []byte) ([]parse.Ride, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return parse.File("activity", bytes.NewReader(data), time.Local)
	}
	var out []parse.Ride
	var first error
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			first = cmp.Or(first, err)
			continue
		}
		got, err := parse.File(f.Name, rc, time.Local)
		_ = rc.Close()
		if err != nil {
			first = cmp.Or(first, err)
			continue
		}
		out = append(out, got...)
	}
	if len(out) == 0 {
		return nil, cmp.Or(first, parse.ErrNoRide)
	}
	return out, nil
}
