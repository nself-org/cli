package commands

// start_lock_test.go — `nself start --frozen` (P7-LIVE-09, contract:cli.lockfile v1).

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/lock"
)

const frozenDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"

// frozenProject writes a docker-compose.yml and the lock that matches it, in a
// hermetic project dir with an empty plugin dir.
func frozenProject(t *testing.T) (dir string, compose string) {
	t.Helper()
	dir = t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", filepath.Join(dir, "plugins"))
	compose = "services:\n  hasura:\n    image: hasura/graphql-engine:v2.44.0@" + frozenDigest + "\n  custom:\n    image: mine/app:1\n"
	writeLockProject(t, dir, compose)
	return dir, compose
}

func writeLockProject(t *testing.T, dir, compose string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	l := lock.New()
	l.CLI = ""
	l.Images["hasura"] = "hasura/graphql-engine:v2.44.0@" + frozenDigest
	raw, err := l.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock.Path(dir), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestStartFrozenMismatch: --frozen exits 1 with E455 when a compose image
// differs from the lock (and when there is no lock); a plain start warns once
// and continues; a matching project passes either way.
func TestStartFrozenMismatch(t *testing.T) {
	dir, compose := frozenProject(t)

	var out bytes.Buffer
	if err := checkStartLock(dir, true, &out); err != nil || out.Len() != 0 {
		t.Fatalf("matching project: err %v, output %q", err, out.String())
	}

	// The compose image moves to another digest.
	moved := strings.Replace(compose, frozenDigest, "sha256:"+strings.Repeat("3", 64), 1)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(moved), 0o600); err != nil {
		t.Fatal(err)
	}
	err := checkStartLock(dir, true, &out)
	var ce *errs.CLIError
	if err == nil || !errors.As(err, &ce) || ce.Code != "E455" {
		t.Fatalf("--frozen on a mismatch: want E455, got %v", err)
	}
	if !strings.Contains(err.Error(), "image hasura") {
		t.Fatalf("E455 must name the service: %v", err)
	}
	if e := errs.Registry["E455"]; e.Exit != 1 {
		t.Fatalf("E455 exits %d, want 1", e.Exit)
	}

	// Plain start: one warning, no error.
	out.Reset()
	if err := checkStartLock(dir, false, &out); err != nil {
		t.Fatalf("plain start must continue: %v", err)
	}
	if n := strings.Count(out.String(), "warning:"); n != 1 || !strings.Contains(out.String(), "image hasura") {
		t.Fatalf("want exactly one warning naming hasura, got %q", out.String())
	}

	// A tag-only compose image is a mismatch too (the lock pins a digest).
	tagOnly := strings.Replace(compose, "@"+frozenDigest, "", 1)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(tagOnly), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkStartLock(dir, true, &out); err == nil {
		t.Fatal("--frozen accepted a compose image without the pinned digest")
	}

	// No lock: --frozen fails, plain start is silent.
	if err := os.Remove(lock.Path(dir)); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := checkStartLock(dir, true, &out); err == nil || !errors.As(err, &ce) || ce.Code != "E455" {
		t.Fatalf("--frozen without a lock must fail with E455, got %v", err)
	}
	if err := checkStartLock(dir, false, &out); err != nil || out.Len() != 0 {
		t.Fatalf("plain start without a lock must be silent: %v %q", err, out.String())
	}
}

// TestStartFrozenFlag: the flag exists on start and the pre-run hook is wired.
func TestStartFrozenFlag(t *testing.T) {
	if f := startCmd.Flags().Lookup("frozen"); f == nil || f.DefValue != "false" {
		t.Fatal("start has no --frozen flag")
	}
	if startCmd.PreRunE == nil {
		t.Fatal("start has no PreRunE lock check")
	}
}
