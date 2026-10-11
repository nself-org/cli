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
	"github.com/nself-org/cli/internal/version"

	"github.com/spf13/cobra"
)

const frozenDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"

// frozenProject writes a docker-compose.yml and the lock that matches it, in a
// hermetic project dir with an empty plugin dir.
func frozenProject(t *testing.T) (dir string, compose string) {
	t.Helper()
	dir = t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", filepath.Join(dir, "plugins"))
	oldVersion := version.Version
	version.Version = "1.4.0"
	t.Cleanup(func() { version.Version = oldVersion })
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
	l.CLI = "1.4"
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

// TestBuildWriteLockFlag: `build --plan --write-lock` reaches the pipeline as
// BuildOptions.WriteLock (no environment side channel): a v1.4 plan lists
// nself.lock.yaml with the flag and not without it.
func TestBuildWriteLockFlag(t *testing.T) {
	lockPlanned := func(args ...string) bool {
		p := newL03Project(t, "dev-minimal")
		r := p.run(t, false, append([]string{"build", "--plan", "--json"}, args...)...)
		if r.code != 0 {
			t.Fatalf("build %v exited %d\n%s\n%s", args, r.code, r.stdout, r.stderr)
		}
		arts, _ := validateEnvelope(t, r.stdout, "build")["artifacts"].([]any)
		for _, a := range arts {
			if m, _ := a.(map[string]any); m != nil && m["path"] == lock.FileName {
				return true
			}
		}
		return false
	}
	if lockPlanned() {
		t.Fatal("a v1.4 plan without --write-lock lists the lock")
	}
	if !lockPlanned("--write-lock") {
		t.Fatal("build --plan --write-lock does not list the lock")
	}
}

// TestStartFrozenDisablesAutoChange: after the lock check passes, --frozen must
// not let runStart rebuild (it would re-render the compose and rewrite the
// lock) or auto-install the AI plugins. A plain start keeps both.
func TestStartFrozenDisablesAutoChange(t *testing.T) {
	dir, _ := frozenProject(t)
	t.Setenv("AI_AUTO_INSTALL", "")
	t.Chdir(dir)

	newCmd := func(frozen bool) *cobra.Command {
		c := &cobra.Command{Use: "start"}
		c.Flags().Bool("skip-build", false, "")
		c.Flags().Bool("frozen", false, "")
		if frozen {
			_ = c.Flags().Set("frozen", "true")
		}
		return c
	}

	plain := newCmd(false)
	if err := startLockPreRun(plain, nil); err != nil {
		t.Fatal(err)
	}
	if v, _ := plain.Flags().GetBool("skip-build"); v || os.Getenv("AI_AUTO_INSTALL") != "" {
		t.Fatalf("a plain start was frozen: skip-build %v, AI_AUTO_INSTALL %q", v, os.Getenv("AI_AUTO_INSTALL"))
	}

	frozen := newCmd(true)
	if err := startLockPreRun(frozen, nil); err != nil {
		t.Fatalf("matching project failed --frozen: %v", err)
	}
	if v, _ := frozen.Flags().GetBool("skip-build"); !v {
		t.Fatal("--frozen left the automatic rebuild on")
	}
	if got := os.Getenv("AI_AUTO_INSTALL"); got != "false" {
		t.Fatalf("--frozen left the AI auto-install on (AI_AUTO_INSTALL=%q)", got)
	}
}

// TestStartFrozenOverrideImage: an override file that sets a pinned service's
// image cannot change the stack without E455 (lens must-fix 1); one that does
// not touch a CLI-owned image passes.
func TestStartFrozenOverrideImage(t *testing.T) {
	dir, _ := frozenProject(t)
	over := filepath.Join(dir, "docker-compose.override.yml")

	if err := os.WriteFile(over, []byte("services:\n  custom:\n    image: mine/app:2\n    environment:\n      A: b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := checkStartLock(dir, true, &out); err != nil {
		t.Fatalf("an override of a service the CLI does not own must pass: %v", err)
	}

	if err := os.WriteFile(over, []byte("services:\n  hasura:\n    image: hasura/graphql-engine:v2.51.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := checkStartLock(dir, true, &out)
	var ce *errs.CLIError
	if err == nil || !errors.As(err, &ce) || ce.Code != "E455" || !strings.Contains(err.Error(), "image hasura") {
		t.Fatalf("an override moving hasura must fail with E455 naming it, got %v", err)
	}
}

// TestStartFrozenStrictEmptyAndUnpinned: --frozen treats a blanked cli or
// plugin sha256 as a mismatch and warns once listing tag-only services
// (lens should-fix 5 and 6); a plain start keeps the lenient comparison.
func TestStartFrozenStrictEmptyAndUnpinned(t *testing.T) {
	dir, _ := frozenProject(t)
	l, err := lock.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	l.CLI = ""
	raw, err := l.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock.Path(dir), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = checkStartLock(dir, true, &out)
	if err == nil || !strings.Contains(err.Error(), "cli: lock has <empty>") {
		t.Fatalf("--frozen: the lock cli is blank and the stack has one, want a cli mismatch, got %v", err)
	}
	out.Reset()
	if err := checkStartLock(dir, false, &out); err != nil || out.Len() != 0 {
		t.Fatalf("plain start stays lenient on an unknown cli: %v %q", err, out.String())
	}

	// Tag-only services: one warning that names them, and no failure.
	dir2 := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", filepath.Join(dir2, "plugins"))
	if err := os.WriteFile(filepath.Join(dir2, "docker-compose.yml"), []byte("services:\n  hasura:\n    image: hasura/graphql-engine:v2.40.0\n  redis:\n    image: redis:6\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	u := lock.New()
	u.CLI = "1.4"
	u.Unpinned["hasura"] = "hasura/graphql-engine:v2.40.0"
	u.Unpinned["redis"] = "redis:6"
	raw, err = u.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock.Path(dir2), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := checkStartLock(dir2, true, &out); err != nil {
		t.Fatalf("tag-only services that match must pass: %v", err)
	}
	if n := strings.Count(out.String(), "warning:"); n != 1 || !strings.Contains(out.String(), "hasura, redis") {
		t.Fatalf("want one warning listing hasura, redis; got %q", out.String())
	}
	out.Reset()
	if err := checkStartLock(dir2, false, &out); err != nil || out.Len() != 0 {
		t.Fatalf("plain start must not warn about tag-only services: %v %q", err, out.String())
	}
}

// TestStartFrozenMirrorOverride: the registry-mirror file that start writes
// (.nself/state/image-overrides.yml) must not fail --frozen when its digest is
// the locked one, and any other override still does (CR r3 M1).
func TestStartFrozenMirrorOverride(t *testing.T) {
	dir, _ := frozenProject(t)
	path := filepath.Join(dir, ".nself", "state", "image-overrides.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(ref string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("# GENERATED BY nself\nservices:\n  hasura:\n    image: "+ref+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	write("mirror.example/hasura/graphql-engine@" + frozenDigest)
	if err := checkStartLock(dir, true, &out); err != nil {
		t.Fatalf("a same-digest mirror override must pass --frozen: %v", err)
	}
	write("mirror.example/hasura/graphql-engine@sha256:" + strings.Repeat("7", 64))
	err := checkStartLock(dir, true, &out)
	var ce *errs.CLIError
	if err == nil || !errors.As(err, &ce) || ce.Code != "E455" || !strings.Contains(err.Error(), "image hasura") {
		t.Fatalf("an override with another digest must fail with E455 naming hasura, got %v", err)
	}
}

// TestStartFrozenUnverifiableImages: under --frozen an extends, build, include
// or variable image on a CLI-owned service cannot be read as an image, so it is
// an E455 ("image source cannot be verified"); a plain start only carries on.
// A user-owned service may use all of them (adversarial R1, R2).
func TestStartFrozenUnverifiableImages(t *testing.T) {
	cases := []struct{ name, override string }{
		{"extends", "services:\n  hasura:\n    extends:\n      file: evil.yml\n      service: evilbase\n"},
		{"build", "services:\n  hasura:\n    build: ./evil\n    pull_policy: build\n"},
		{"include", "include:\n  - evil.yml\n"},
		{"variable", "services:\n  hasura:\n    image: ${HX:-hasura/graphql-engine:v2.44.0}\n"},
		{"bare variable", "services:\n  hasura:\n    image: $HX\n"},
	}
	for _, c := range cases {
		dir, _ := frozenProject(t)
		over := filepath.Join(dir, "docker-compose.override.yml")
		if err := os.WriteFile(over, []byte(c.override), 0o600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err := checkStartLock(dir, true, &out)
		var ce *errs.CLIError
		if err == nil || !errors.As(err, &ce) || ce.Code != "E455" || !strings.Contains(err.Error(), "cannot be verified") {
			t.Errorf("%s: --frozen must fail with E455 'cannot be verified', got %v", c.name, err)
		}
		out.Reset()
		if err := checkStartLock(dir, false, &out); err != nil {
			t.Errorf("%s: a plain start must not fail: %v", c.name, err)
		}
	}
	dir, _ := frozenProject(t)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.override.yml"), []byte("services:\n  custom:\n    build: .\n    image: $X\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := checkStartLock(dir, true, &out); err != nil {
		t.Errorf("a user-owned service may use build and variables: %v", err)
	}
}
