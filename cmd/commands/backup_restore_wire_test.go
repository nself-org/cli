package commands

// Purpose: prove the deploy/promote rollback restore line works through the
// real cobra root (argv in, Execute) on a production config with no stdin:
// --confirm-project <name> restores, a wrong name and bare --yes refuse before
// any database command runs. A stub docker on PATH logs every call (the probe).
// Each run is a child process (this test binary re-executed) so Execute's
// tree preparation never leaks into the rest of the package.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/restoreconsent"
)

// TestWire18Child is the child entry: it runs the real root on the argv the
// parent passes (newline-separated) and exits with its code. A no-op otherwise.
func TestWire18Child(t *testing.T) {
	argv := os.Getenv("NSELF_TEST_WIRE18_ARGV")
	if argv == "" {
		t.Skip("child entry for the TestWire18 restore tests")
	}
	os.Args = append([]string{"nself"}, strings.Split(argv, "\n")...)
	err := Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, "WIRE18-ERR:", err)
	}
	os.Exit(errs.ExitCodeFor(err))
}

// wire18Run builds a prod project "demo" with one backup and a logging docker
// stub, runs `nself <argv>` in a child with no stdin, and returns the child's
// error (nil on exit 0), its stderr and the docker calls.
func wire18Run(t *testing.T, argv ...string) (error, string, string) {
	t.Helper()
	dir := t.TempDir()
	write := func(p, body string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, ".env"), "PROJECT_NAME=demo\nENV=prod\n", 0o600)
	write(filepath.Join(dir, "backups", "demo_full_1.dump"), "PGDMP", 0o600)
	log := filepath.Join(dir, "docker.log")
	bin := filepath.Join(dir, "bin")
	write(filepath.Join(bin, "docker"), "#!/bin/sh\necho \"$*\" >> '"+log+"'\ncat >/dev/null 2>&1 || true\nexit 0\n", 0o755)

	home := t.TempDir()
	child := exec.Command(os.Args[0], "-test.run=^TestWire18Child$")
	child.Dir = dir
	child.Stdin = nil // no stdin, as over ssh
	child.Env = append(os.Environ(),
		"NSELF_TEST_WIRE18_ARGV="+strings.Join(argv, "\n"),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+home, "USERPROFILE="+home, "PROJECT_NAME=demo", "ENV=prod")
	var stderr strings.Builder
	child.Stderr = &stderr
	err := child.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(log)
	return err, stderr.String(), string(calls)
}

func TestWire18ConfirmProjectRestoresOnProd(t *testing.T) {
	line, cerr := restoreconsent.Command("demo_full_1", "demo")
	if cerr != nil || !strings.Contains(line, "--confirm-project 'demo'") {
		t.Fatalf("rollback restore line %q lost its proof: %v", line, cerr)
	}
	err, stderr, calls := wire18Run(t, "backup", "restore", "demo_full_1", "--confirm-project", "demo", "--only", "pg")
	if err != nil {
		t.Fatalf("prod restore with --confirm-project demo and no stdin: %v\n%s", err, stderr)
	}
	if !strings.Contains(calls, "pg_restore") || !strings.Contains(calls, "demo_postgres") || !strings.Contains(calls, "RENAME TO") {
		t.Fatalf("restore did not run against demo_postgres; docker calls:\n%s", calls)
	}
}

func TestWire18WrongProjectRefused(t *testing.T) {
	err, stderr, calls := wire18Run(t, "backup", "restore", "demo_full_1", "--yes", "--confirm-project", "other", "--only", "pg")
	if err == nil || !strings.Contains(stderr, "confirmation") {
		t.Fatalf("wrong --confirm-project on prod must refuse, got %v\n%s", err, stderr)
	}
	if calls != "" {
		t.Fatalf("refused restore still ran docker:\n%s", calls)
	}
}

func TestWire18BareYesStillRefused(t *testing.T) {
	err, stderr, calls := wire18Run(t, "backup", "restore", "demo_full_1", "--yes", "--only", "pg")
	if err == nil || !strings.Contains(stderr, "confirmation") {
		t.Fatalf("bare --yes on prod must refuse, got %v\n%s", err, stderr)
	}
	if calls != "" {
		t.Fatalf("refused restore still ran docker:\n%s", calls)
	}
}
