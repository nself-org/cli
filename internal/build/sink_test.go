package build

// sink_test.go — plan-mode no-write test with writable directories and a
// loose-mode env file (P7-LIVE-21 review S3).
// Inputs: the dev-minimal fixture with .env made 0644 and every directory left
// writable, so a stray Chmod, Remove or MkdirAll that reached disk would change
// the snapshot instead of failing silently.
// Outputs: pass/fail. Constraints: Unix permission bits only (skipped on Windows).

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPlanModeLooseEnvModeUntouchedWithWritableDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits only")
	}
	for _, name := range []string{"dev-minimal", "prod-plugins", "fronted"} {
		t.Run(name, func(t *testing.T) {
			f := newPlanFixture(t, name)
			envPath := filepath.Join(f.workdir, ".env")
			if err := os.Chmod(envPath, 0o644); err != nil {
				t.Fatal(err)
			}
			marker := stubHostTools(t, f.root)
			before := planSnapshot(t, f.root) // sha + mode of every path
			res, err := Build(f.workdir, BuildOptions{Mode: ModePlan})
			if err != nil {
				t.Fatalf("plan Build: %v", err)
			}
			after := planSnapshot(t, f.root)

			if d := lineDiff(strings.Join(before, "\n"), strings.Join(after, "\n")); d != "" {
				t.Errorf("plan mode changed a writable tree:\n%s", d)
			}
			if info, err := os.Stat(envPath); err != nil || info.Mode().Perm() != 0o644 {
				t.Errorf(".env mode after plan = %v (err %v), want 0644 untouched", info.Mode().Perm(), err)
			}
			if _, err := os.Stat(marker); err == nil {
				t.Error("plan mode ran a host tool")
			}
			if res.Planned == nil {
				t.Fatal("no Planned set")
			}
			var got os.FileMode
			var found bool
			for key, mode := range res.Planned.Modes {
				if key == ".env" || filepath.Clean(key) == envPath {
					got, found = mode, true
				}
			}
			if !found || got != 0o600 {
				t.Errorf("plan must record .env -> 0600, got %v (found %v) in %v", got, found, res.Planned.Modes)
			}
		})
	}
}
