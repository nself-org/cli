package commands

// p7-wire-20: `nself start` exits non-zero when the health gate fails or a core
// service is unhealthy. The tests drive the real RootCmd by argv against a stub
// docker whose `compose ps` reports postgres unhealthy.

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

const wire20PS = `[{"Name":"w20_postgres","Service":"postgres","State":"running","Health":"unhealthy"},` +
	`{"Name":"w20_hasura","Service":"hasura","State":"running","Health":"healthy"},` +
	`{"Name":"w20_auth","Service":"auth","State":"running","Health":"healthy"},` +
	`{"Name":"w20_nginx","Service":"nginx","State":"running","Health":"healthy"}]`

// wire20Project writes a project dir and a docker stub: every call succeeds and
// `compose ps` prints wire20PS.
func wire20Project(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("sh stub")
	}
	t.Setenv("NSELF_ALLOW_SOURCE_DIR", "1")
	dir := t.TempDir()
	files := map[string]string{
		".env":               "PROJECT_NAME=w20\nBASE_DOMAIN=test.example.com\n",
		"docker-compose.yml": "services:\n  postgres:\n    image: postgres:16\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bin := t.TempDir()
	script := "#!/bin/sh\ncase \"$*\" in *\" ps \"*) printf '%s' '" + wire20PS + "';; *--services*) printf 'postgres\nhasura\nauth\nnginx\n';; *\" config \"*) printf '%s' '{\"services\":{}}';; esac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil { //nolint:gosec // test stub
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AI_AUTO_INSTALL", "false")
	t.Chdir(dir)
}

func wire20Start(t *testing.T, extra ...string) error {
	t.Helper()
	args := append([]string{"start", "--no-monorepo", "--skip-build", "--skip-port-check", "--skip-plugins", "--timeout", "30"}, extra...)
	t.Cleanup(func() {
		if f := startCmd.Flags().Lookup("allow-degraded"); f != nil {
			_ = f.Value.Set("false")
			f.Changed = false
		}
	})
	RootCmd.SetArgs(args)
	return RootCmd.Execute()
}

func TestWire20UnhealthyCoreExitsNonZero(t *testing.T) {
	wire20Project(t)
	t.Setenv("NSELF_V15", "1")
	err := wire20Start(t)
	if err == nil {
		t.Fatal("start exited 0 with postgres unhealthy")
	}
	if !errors.Is(err, errs.ErrServiceUnhealthy) || !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("want a core-service error naming postgres, got: %v", err)
	}
}

func TestWire20AllowDegradedKeepsZero(t *testing.T) {
	wire20Project(t)
	t.Setenv("NSELF_V15", "1")
	if err := wire20Start(t, "--allow-degraded"); err != nil {
		t.Fatalf("--allow-degraded must exit 0, got: %v", err)
	}
}

// Before the flip (NSELF_V15 unset) the old behaviour stays the default.
func TestWire20DormantBeforeFlip(t *testing.T) {
	wire20Project(t)
	t.Setenv("NSELF_V15", "")
	if err := wire20Start(t); err != nil {
		t.Fatalf("v1.4 behaviour must exit 0, got: %v", err)
	}
}
