// Package sqlite implements store.Store on SQLite. SQLite types and errors
// never leave this package.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/pressly/goose/v3"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/spokebook/spokebook/internal/app/spokebook/store"
	"github.com/spokebook/spokebook/internal/app/spokebook/store/sqlite/db"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Times are stored as UTC text with a fixed width, so that text comparison
// and ORDER BY match chronological order (RFC3339Nano trims trailing zeros).
const timeLayout = "2006-01-02T15:04:05.000000000Z"

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

// parseTime also accepts RFC 3339 without fractional seconds or with fewer digits.
func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }

type Store struct {
	sqlDB *sql.DB
	stores
}

type stores struct {
	q *db.Queries
}

func (s stores) Users() store.UserStore        { return userStore(s) }
func (s stores) Bikes() store.BikeStore        { return bikeStore(s) }
func (s stores) Odometer() store.OdometerStore { return odometerStore(s) }
func (s stores) Rides() store.RideStore        { return rideStore(s) }

func (s stores) Components() store.ComponentStore { return componentStore(s) }
func (s stores) Steps() store.StepStore           { return stepStore(s) }

func (s stores) Maintenances() store.MaintenanceStore { return maintenanceStore(s) }
func (s stores) Attachments() store.AttachmentStore   { return attachmentStore(s) }
func (s stores) Shopping() store.ShoppingStore        { return shoppingStore(s) }

// Open opens the database at path (":memory:" for tests) and runs the
// migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path)
	if path == ":memory:" {
		dsn = "file::memory:?_pragma=foreign_keys(1)"
	}
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	// A single connection serializes writes and keeps :memory: consistent.
	sqlDB.SetMaxOpenConns(1)

	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{sqlDB: sqlDB, stores: stores{q: db.New(sqlDB)}}, nil
}

func (s *Store) Close() error { return s.sqlDB.Close() }

func (s *Store) WithTx(ctx context.Context, fn func(store.Stores) error) error {
	tx, err := s.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(stores{q: s.q.WithTx(tx)}); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// mapErr translates SQLite errors into store errors. A foreign key violation
// means the referenced parent (for this owner) does not exist.
func mapErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return store.ErrNotFound
	}
	var se *sqlite.Error
	if errors.As(err, &se) {
		switch se.Code() {
		case sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY:
			return store.ErrNotFound
		case sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
			return store.ErrConflict
		}
	}
	return err
}

func nullTime(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(*t), Valid: true}
}

func timePtr(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil
	}
	t, err := parseTime(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// nullString maps "" to NULL.
func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func nullInt(i *int64) sql.NullInt64 {
	if i == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *i, Valid: true}
}

func intPtr(i sql.NullInt64) *int64 {
	if !i.Valid {
		return nil
	}
	return &i.Int64
}
