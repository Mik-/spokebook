package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestValueKindsMigration checks that existing values get a key and a kind.
func TestValueKindsMigration(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, sqlDB, "migrations", 12); err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.ExecContext(ctx, `INSERT INTO component_values (id, owner_id, component_id, position, label, value_from, value_to, unit)
		VALUES ('1', 'o1', 'c1', 0, 'Center-Lock', 5, 6, 'Nm'), ('2', 'o1', 'c1', 1, 'Pads', 1.2, NULL, 'mm'),
		       ('3', 'o1', 'c1', 2, 'Pressure', 2, 3.5, 'bar')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, sqlDB, "migrations", 13); err != nil {
		t.Fatal(err)
	}
	rows, err := sqlDB.QueryContext(ctx, `SELECT key, kind FROM component_values ORDER BY position`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got [][2]string
	for rows.Next() {
		var k [2]string
		if err := rows.Scan(&k[0], &k[1]); err != nil {
			t.Fatal(err)
		}
		got = append(got, k)
	}
	want := [][2]string{{"center_lock", "torque"}, {"pads", "limit"}, {"pressure", "target"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

// TestFollowUpsMigration checks that existing catalogs get the follow-ups
// of the built-in steps, each within its owner.
func TestFollowUpsMigration(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, sqlDB, "migrations", 22); err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.ExecContext(ctx, `INSERT INTO steps (id, owner_id, builtin_id, position, title, phase, created_at)
		VALUES ('a1', 'o1', 'chain_wear', 0, '', 'inspection', ''), ('a2', 'o1', 'replace_chain', 1, '', 'replacement', ''),
		       ('a3', 'o1', 'adjust_derailleur', 2, '', 'adjustment', ''),
		       ('b1', 'o2', 'chain_wear', 0, '', 'inspection', ''), ('b2', 'o2', 'wash_bike', 1, '', 'cleaning', '')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, sqlDB, "migrations", 23); err != nil {
		t.Fatal(err)
	}
	rows, err := sqlDB.QueryContext(ctx, `SELECT step_id || '>' || follow_up_id || ':' || kind FROM step_follow_ups ORDER BY step_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		got = append(got, s)
	}
	if want := []string{"a1>a2:finding", "a2>a3:always"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestValueInfoMigration checks that the rebuilt value tables keep their
// rows and accept the kind info.
func TestValueInfoMigration(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, sqlDB, "migrations", 24); err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.ExecContext(ctx, `INSERT INTO component_values (id, owner_id, component_id, position, label, value_from, value_to, unit, key, kind, hint)
		VALUES ('1', 'o1', 'c1', 0, 'Clamp', 5, 6, 'Nm', 'clamp', 'torque', 'paste')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, sqlDB, "migrations", 25); err != nil {
		t.Fatal(err)
	}
	var key, kind, hint string
	var from, to float64
	err = sqlDB.QueryRowContext(ctx, `SELECT key, kind, hint, value_from, value_to FROM component_values WHERE id = '1'`).Scan(&key, &kind, &hint, &from, &to)
	if err != nil || key != "clamp" || kind != "torque" || hint != "paste" || from != 5 || to != 6 {
		t.Errorf("got %s %s %s %v %v, %v", key, kind, hint, from, to, err)
	}
	for _, table := range []string{"component_values", "component_type_values", "step_values", "maintenance_values"} {
		var n int
		err := sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type = 'index' AND sql IS NOT NULL AND tbl_name = ?`, table).Scan(&n)
		if err != nil || n != 1 {
			t.Errorf("%s: %d indexes, %v", table, n, err)
		}
	}
	_, err = sqlDB.ExecContext(ctx, `INSERT INTO component_values (id, owner_id, component_id, position, label, value_from, unit, key, kind)
		VALUES ('2', 'o1', 'c1', 1, 'Tire width', 32, 'mm', 'tire_width', 'info')`)
	if err != nil {
		t.Errorf("info rejected: %v", err)
	}
}
