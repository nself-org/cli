package database

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Regression tests for the migrate-ledger primary-key collision (Unity PCI):
// the ledger id used to be the date-derived prefix before the first "_", so
// two migrations sharing a day collided on the PK and the second was recorded
// (or skipped) without its DDL ever running. The nested Hasura layout had the
// same family of bug through name = filepath.Base(f) = "up.sql" for every
// migration.

// --- migrationKey ---

func TestMigrationKey_FlatAndNested(t *testing.T) {
	cases := map[string]string{
		"hasura/migrations/20260701_add_users.sql":            "20260701_add_users.sql",
		"migrations/001_init.sql":                             "001_init.sql",
		"hasura/migrations/default/20260701_add_users/up.sql": "20260701_add_users",
		"hasura/migrations/default/1000_init/up.sql":          "1000_init",
	}
	for path, want := range cases {
		if got := migrationKey(filepath.FromSlash(path)); got != want {
			t.Errorf("migrationKey(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestMigrationKey_NestedSameDayDistinct is the core regression: two nested
// migrations must never collapse to the same ledger key ("up.sql" under the
// old code), which silently skipped the second while reporting it applied.
func TestMigrationKey_NestedSameDayDistinct(t *testing.T) {
	a := migrationKey(filepath.FromSlash("m/20260701_add_users/up.sql"))
	b := migrationKey(filepath.FromSlash("m/20260701_add_orders/up.sql"))
	if a == b {
		t.Fatalf("nested same-day migrations share ledger key %q — second would be silently skipped", a)
	}
}

// --- extractMigrationID ---

// TestExtractMigrationID_SameDayDistinct verifies that two migrations sharing
// the same date-derived version prefix get DISTINCT ledger ids. The old code
// returned "20260701" for both, and ON CONFLICT (id) DO NOTHING dropped the
// second row from nself_ops.migrations.
func TestExtractMigrationID_SameDayDistinct(t *testing.T) {
	a := extractMigrationID("hasura/migrations/20260701_add_users.sql")
	b := extractMigrationID("hasura/migrations/20260701_add_orders.sql")
	if a == b {
		t.Fatalf("same-day migrations share ledger id %q — second ledger row would be a silent no-op", a)
	}
	if a != "20260701_add_users" || b != "20260701_add_orders" {
		t.Errorf("ids = %q, %q; want full-name ids", a, b)
	}

	// Nested layout: id is the parent dir name, distinct per migration.
	na := extractMigrationID(filepath.FromSlash("m/20260701_add_users/up.sql"))
	nb := extractMigrationID(filepath.FromSlash("m/20260701_add_orders/up.sql"))
	if na == nb {
		t.Fatalf("nested same-day migrations share ledger id %q", na)
	}

	// Down files strip the .down suffix.
	if got := extractMigrationID("m/20260701_add_users.down.sql"); got != "20260701_add_users" {
		t.Errorf("down file id = %q, want 20260701_add_users", got)
	}
}

// --- pendingMigrationFiles (apply gating) ---

// TestPendingMigrationFiles_SameDayPairBothApply: with an empty ledger both
// same-day migrations are pending; after the first applies, the second is
// STILL pending (regression: must not be treated as applied).
func TestPendingMigrationFiles_SameDayPairBothApply(t *testing.T) {
	files := []string{
		"m/20260701_add_users.sql",
		"m/20260701_add_orders.sql",
	}

	pending := pendingMigrationFiles(files, map[string]time.Time{})
	if len(pending) != 2 {
		t.Fatalf("empty ledger: want 2 pending, got %d (%v)", len(pending), pending)
	}

	applied := map[string]time.Time{migrationKey(files[0]): time.Now()}
	pending = pendingMigrationFiles(files, applied)
	if len(pending) != 1 || pending[0] != files[1] {
		t.Fatalf("after first applied: want [%s] pending, got %v", files[1], pending)
	}
}

// TestPendingMigrationFiles_NestedNotCollapsed: nested layout, second same-day
// migration must stay pending after the first is applied.
func TestPendingMigrationFiles_NestedNotCollapsed(t *testing.T) {
	files := []string{
		filepath.FromSlash("m/20260701_add_orders/up.sql"),
		filepath.FromSlash("m/20260701_add_users/up.sql"),
	}
	applied := map[string]time.Time{"20260701_add_orders": time.Now()}

	pending := pendingMigrationFiles(files, applied)
	if len(pending) != 1 || migrationKey(pending[0]) != "20260701_add_users" {
		t.Fatalf("want only 20260701_add_users pending, got %v", pending)
	}
}

// TestPendingMigrationFiles_ReRunIdempotent: when every migration is recorded,
// nothing is pending (re-run of `nself db migrate up` is a no-op).
func TestPendingMigrationFiles_ReRunIdempotent(t *testing.T) {
	files := []string{
		"m/20260701_add_users.sql",
		filepath.FromSlash("m/20260702_add_orders/up.sql"),
	}
	applied := map[string]time.Time{
		"20260701_add_users.sql": time.Now(),
		"20260702_add_orders":    time.Now(),
	}
	if pending := pendingMigrationFiles(files, applied); len(pending) != 0 {
		t.Fatalf("fully applied ledger: want 0 pending, got %v", pending)
	}
}

// --- migrationRecordSQL (on-conflict semantics) ---

// TestMigrationRecordSQL_NeverSilentNoop: the ops-ledger record must APPLY on
// conflict (DO UPDATE — ids are unique per migration now, so a conflict can
// only be the same migration re-recorded) and the legacy record must ERROR on
// conflict (plain INSERT). Neither may silently DO NOTHING.
func TestMigrationRecordSQL_NeverSilentNoop(t *testing.T) {
	legacy, ops := migrationRecordSQL("20260701_add_users", "20260701_add_users.sql", "abc")

	if strings.Contains(ops, "DO NOTHING") || strings.Contains(legacy, "DO NOTHING") {
		t.Fatal("ledger record SQL must never use ON CONFLICT DO NOTHING (silent no-op)")
	}
	if !strings.Contains(ops, "ON CONFLICT (id) DO UPDATE") {
		t.Errorf("ops record must apply on conflict, got: %s", ops)
	}
	if strings.Contains(legacy, "ON CONFLICT") {
		t.Errorf("legacy record must be a plain INSERT that errors on conflict, got: %s", legacy)
	}
	if !strings.Contains(ops, "'20260701_add_users'") || !strings.Contains(legacy, "'20260701_add_users.sql'") {
		t.Errorf("record SQL missing expected key/name:\n%s\n%s", legacy, ops)
	}
}

func TestMigrationRecordSQL_EscapesQuotes(t *testing.T) {
	legacy, ops := migrationRecordSQL("o'id", "o'name.sql", "abc")
	if !strings.Contains(legacy, "o''name.sql") || !strings.Contains(ops, "o''id") {
		t.Errorf("single quotes must be doubled:\n%s\n%s", legacy, ops)
	}
}

// --- ledger upgrade path ---

// TestResolveLegacyNestedKey covers mapping the legacy 'up.sql' ledger row to
// the migration that actually ran on an already-deployed box.
func TestResolveLegacyNestedKey(t *testing.T) {
	files := []string{
		filepath.FromSlash("m/1000_init/up.sql"),
		filepath.FromSlash("m/2000_users/up.sql"),
		"m/9999_flat.sql", // flat file must be ignored
	}

	// Ops row id (old prefix style) uniquely identifies the dir that ran.
	if got := resolveLegacyNestedKey(files, []string{"2000"}); got != "2000_users" {
		t.Errorf("ops-id match: want 2000_users, got %q", got)
	}

	// No ops rows: fall back to the first sorted nested migration — the only
	// one the old collided code could ever have applied.
	if got := resolveLegacyNestedKey(files, nil); got != "1000_init" {
		t.Errorf("fallback: want 1000_init, got %q", got)
	}

	// Ambiguous prefix (two dirs same day): fall back to first sorted.
	amb := []string{
		filepath.FromSlash("m/20260701_a/up.sql"),
		filepath.FromSlash("m/20260701_b/up.sql"),
	}
	if got := resolveLegacyNestedKey(amb, []string{"20260701"}); got != "20260701_a" {
		t.Errorf("ambiguous prefix: want 20260701_a, got %q", got)
	}

	// No nested migrations on disk: nothing to map.
	if got := resolveLegacyNestedKey([]string{"m/only_flat.sql"}, []string{"1000"}); got != "" {
		t.Errorf("flat-only: want empty key, got %q", got)
	}
}

// TestLegacyOpsIDBackfillSQL_Guards: the in-place id rewrite must be
// idempotent (skip rows already on the new id), never touch the legacy nested
// 'up.sql' rows, and never create a PK conflict.
func TestLegacyOpsIDBackfillSQL_Guards(t *testing.T) {
	sql := legacyOpsIDBackfillSQL
	for _, want := range []string{
		"m.id <> left(m.name, length(m.name) - 4)", // idempotence guard
		"m.name <> 'up.sql'",                       // nested rows handled separately
		"NOT EXISTS",                               // PK-conflict guard
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("backfill SQL missing guard %q:\n%s", want, sql)
		}
	}
}

// TestLegacyNestedRenameSQL_IdempotentShape: the rename targets only rows
// literally named 'up.sql' (so a second run is a no-op), updates before it
// deletes, and escapes the key.
func TestLegacyNestedRenameSQL_IdempotentShape(t *testing.T) {
	sql := legacyNestedRenameSQL("20260701_add_users")

	if !strings.Contains(sql, "WHERE name = 'up.sql'") {
		t.Error("rename must only touch legacy 'up.sql' rows")
	}
	if !strings.Contains(sql, "NOT EXISTS") {
		t.Error("rename must guard against an existing new-style row")
	}
	updIdx := strings.Index(sql, "UPDATE np_common.schema_versions")
	delIdx := strings.Index(sql, "DELETE FROM np_common.schema_versions")
	if updIdx == -1 || delIdx == -1 || updIdx > delIdx {
		t.Error("rename must UPDATE the applied fact before DELETE cleanup")
	}

	esc := legacyNestedRenameSQL("o'brien")
	if strings.Contains(esc, "'o'brien'") || !strings.Contains(esc, "o''brien") {
		t.Error("rename key must have single quotes doubled")
	}
}

// TestIsDownMigrationFile_AllConventions is the table test for defect 1: web
// uses "*_down.sql", this CLI's own generator uses "*.down.sql", and both
// (plus their generalized non-".sql" forms) must be recognized as down
// files. Anything else, including a name that merely ends in "down" without
// the "." or "_" separator, must NOT be treated as a down file.
func TestIsDownMigrationFile_AllConventions(t *testing.T) {
	cases := map[string]bool{
		"017_nself_accounts_down.sql": true, // web's convention
		"017_nself_accounts.down.sql": true, // this CLI's `migrate generate` convention
		"foo.down.sql":                true,
		"foo_down.sql":                true,
		"foo.down.pgsql":              true, // generalized non-".sql" down variant
		"foo_down.pgsql":              true,
		"017_nself_accounts.sql":      false, // ordinary up file
		"teardown.sql":                false, // ends in "down" but not "_down"/".down"
		"downgrade.sql":               false,
		"down.sql":                    false, // bare "down" has neither the "." nor "_" separator
	}
	for name, want := range cases {
		if got := isDownMigrationFile(name); got != want {
			t.Errorf("isDownMigrationFile(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestScanMigrations_ExcludesBothDownConventions is the integration-shaped
// regression for the production incident: a flat directory containing both
// down-file conventions must only return the up files, in order, and never
// the down files that were previously slipping through as "*_down.sql".
func TestScanMigrations_ExcludesBothDownConventions(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "017_nself_accounts.sql", "-- up 017")
	writeFile(t, tmp, "017_nself_accounts_down.sql", "-- down 017, web convention")
	writeFile(t, tmp, "018_next.sql", "-- up 018")
	writeFile(t, tmp, "018_next.down.sql", "-- down 018, cli convention")

	files, err := scanMigrations(tmp)
	if err != nil {
		t.Fatalf("scanMigrations: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("scanMigrations = %v, want exactly the 2 up files (down files must be excluded)", files)
	}
	for _, f := range files {
		if strings.Contains(migrationKey(f), "down") {
			t.Errorf("scanMigrations returned a down file: %s", f)
		}
	}
	want := []string{"017_nself_accounts.sql", "018_next.sql"}
	for i, f := range files {
		if migrationKey(f) != want[i] {
			t.Errorf("files[%d] key = %q, want %q", i, migrationKey(f), want[i])
		}
	}
}

// TestScanMigrations_NestedOrderByKey: with the nested layout the sort must
// order by the version-bearing directory name, not the constant "up.sql"
// basename (which left the apply order undefined).
func TestScanMigrations_NestedOrderByKey(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "3000_orders/up.sql", "-- orders")
	writeFile(t, tmp, "1000_init/up.sql", "-- init")
	writeFile(t, tmp, "2000_users/up.sql", "-- users")

	files, err := scanMigrations(tmp)
	if err != nil {
		t.Fatalf("scanMigrations: %v", err)
	}
	want := []string{"1000_init", "2000_users", "3000_orders"}
	if len(files) != len(want) {
		t.Fatalf("want %d files, got %d", len(want), len(files))
	}
	for i, f := range files {
		if migrationKey(f) != want[i] {
			t.Errorf("files[%d] key = %q, want %q", i, migrationKey(f), want[i])
		}
	}
}
