//go:build integration

// Live verification that ensureUUIDv7 leaves public.uuidv7() usable against a
// REAL Postgres that does not ship the pg_uuidv7 extension.
//
// postgres:16-alpine (what startTestPostgres boots) has no pg_uuidv7, which is
// exactly the case this exists to cover: the stock nSelf image
// (pgvector/pgvector:pg16) does not have it either. So the CREATE EXTENSION
// attempt inside ensureUUIDv7 fails here and the SQL fallback has to carry it.
//
// Asserting "the function exists" would be far too weak. A wrong implementation
// still defines a function and still returns a uuid. These tests pin the
// properties consumers actually depend on: RFC 9562 version 7, the variant bits,
// a timestamp that decodes to now, and strict ordering across milliseconds —
// the ordering is the whole reason callers chose uuidv7 over uuid_generate_v4
// for keyset-pagination cursor indexes.
package database

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// queryScalar runs a single-value query inside the test container.
func queryScalar(t *testing.T, cfg *configForQuery, sql string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("docker", "exec", cfg.container,
		"psql", "-U", "postgres", "-d", "nself", "-tAc", sql)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("query %q failed: %v\n%s", sql, err, stderr.String())
	}
	return strings.TrimSpace(stdout.String())
}

type configForQuery struct{ container string }

func TestEnsureUUIDv7_Integration_FallbackDefinesAConformingFunction(t *testing.T) {
	skipUnlessIntegration(t)
	cfg := startTestPostgres(t)
	q := &configForQuery{container: cfg.ProjectName + "_postgres"}

	// Precondition: the extension really is absent here, so we are proving the
	// fallback path and not accidentally testing a C implementation.
	if got := queryScalar(t, q,
		"SELECT count(*) FROM pg_available_extensions WHERE name = 'pg_uuidv7'"); got != "0" {
		t.Fatalf("precondition: expected pg_uuidv7 to be unavailable in postgres:16-alpine, got count=%s", got)
	}

	if err := ensureUUIDv7(context.Background(), cfg); err != nil {
		t.Fatalf("ensureUUIDv7: %v", err)
	}

	if got := queryScalar(t, q, `SELECT count(*) FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE p.proname = 'uuidv7' AND n.nspname = 'public'`); got != "1" {
		t.Fatalf("expected exactly one public.uuidv7(), got %s", got)
	}

	// Version nibble must be 7 (RFC 9562 §5.7). Character 15 of the canonical
	// text form, 1-indexed, is the version.
	if got := queryScalar(t, q, "SELECT substring(public.uuidv7()::text FROM 15 FOR 1)"); got != "7" {
		t.Errorf("version nibble = %q, want \"7\"", got)
	}

	// Variant bits must be 10xx, i.e. the first character of the 4th group is
	// one of 8, 9, a, b. gen_random_uuid() sets these and the overlay must not
	// have disturbed them.
	if got := queryScalar(t, q, "SELECT substring(public.uuidv7()::text FROM 20 FOR 1)"); !strings.Contains("89ab", got) {
		t.Errorf("variant nibble = %q, want one of 8/9/a/b", got)
	}

	// The leading 48 bits must decode to a timestamp at about now, which is what
	// makes these sortable. Allow a generous window; we are catching a wrong
	// field order or unit, not clock skew.
	skewRaw := queryScalar(t, q, `
		SELECT abs(extract(epoch FROM now()) -
		  (('x' || substring(public.uuidv7()::text FROM 1 FOR 8))::bit(32)::bigint * 65536
		   + ('x' || substring(public.uuidv7()::text FROM 10 FOR 4))::bit(16)::bigint) / 1000.0)::int`)
	if skewRaw == "" {
		t.Fatal("timestamp decode returned nothing")
	}
	skewSec, err := strconv.Atoi(skewRaw)
	if err != nil {
		t.Fatalf("timestamp decode returned %q, not a number", skewRaw)
	}
	if skewSec > 60 {
		t.Errorf("decoded timestamp is %ds away from now — field order or unit is wrong", skewSec)
	}

	// Strict ordering across milliseconds. Sleeping between calls guarantees a
	// different millisecond, so a correct implementation is strictly increasing.
	first := queryScalar(t, q, "SELECT public.uuidv7()")
	time.Sleep(5 * time.Millisecond)
	second := queryScalar(t, q, "SELECT public.uuidv7()")
	if !(first < second) {
		t.Errorf("not time-ordered: %q >= %q", first, second)
	}

	// Distinctness within the same millisecond — the random tail must still vary.
	if got := queryScalar(t, q,
		"SELECT count(DISTINCT public.uuidv7()) FROM generate_series(1, 200)"); got != "200" {
		t.Errorf("expected 200 distinct values in one batch, got %s", got)
	}
}

func TestEnsureUUIDv7_Integration_IsIdempotent(t *testing.T) {
	skipUnlessIntegration(t)
	cfg := startTestPostgres(t)
	q := &configForQuery{container: cfg.ProjectName + "_postgres"}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := ensureUUIDv7(ctx, cfg); err != nil {
			t.Fatalf("ensureUUIDv7 call %d: %v", i+1, err)
		}
	}

	// Still exactly one definition, and still working. Re-running nself start
	// must not accumulate overloads or error on the second boot.
	if got := queryScalar(t, q, `SELECT count(*) FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE p.proname = 'uuidv7' AND n.nspname = 'public'`); got != "1" {
		t.Fatalf("after 3 calls expected exactly one public.uuidv7(), got %s", got)
	}
	if got := queryScalar(t, q, "SELECT substring(public.uuidv7()::text FROM 15 FOR 1)"); got != "7" {
		t.Errorf("version nibble after repeat calls = %q, want \"7\"", got)
	}
}

func TestEnsureUUIDv7_Integration_DoesNotReplaceAnExistingDefinition(t *testing.T) {
	skipUnlessIntegration(t)
	cfg := startTestPostgres(t)
	q := &configForQuery{container: cfg.ProjectName + "_postgres"}

	// Stand in for the C extension: a pre-existing public.uuidv7() that returns a
	// recognisable constant. The guard must leave it alone, otherwise an image
	// that really does ship pg_uuidv7 would silently lose its implementation.
	sentinel := "11111111-1111-7111-8111-111111111111"
	if out, err := exec.Command("docker", "exec", q.container, "psql", "-U", "postgres", "-d", "nself",
		"-c", "CREATE FUNCTION public.uuidv7() RETURNS uuid LANGUAGE sql VOLATILE AS $$ SELECT '"+sentinel+"'::uuid $$",
	).CombinedOutput(); err != nil {
		t.Fatalf("seed sentinel uuidv7: %v\n%s", err, out)
	}

	if err := ensureUUIDv7(context.Background(), cfg); err != nil {
		t.Fatalf("ensureUUIDv7: %v", err)
	}

	if got := queryScalar(t, q, "SELECT public.uuidv7()::text"); got != sentinel {
		t.Errorf("existing definition was replaced: got %q, want the sentinel %q", got, sentinel)
	}
}
