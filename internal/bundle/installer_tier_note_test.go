package bundle

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// TestInstall_KeptFreeCopyIsReported: a bundle install that leaves an
// installed free copy of a tier-pair member in place says so (D-0320); an
// upgrade needs no note because the old copy is moved aside before the install,
// so the install sees no installed tier and resolves by entitlement.
func TestInstall_KeptFreeCopyIsReported(t *testing.T) {
	const name = "nself-uptime-monitor"
	for _, tc := range []struct {
		label    string
		tier     string
		version  string
		pair     bool
		wantNote bool
	}{
		{"free, same version, pair", "free", "1.0.0", true, true},
		{"free, higher version, pair", "free", "2.0.0", true, true},
		{"free, same version, no licensed build", "free", "1.0.0", false, false},
		{"licensed copy kept", "pro", "1.0.0", true, false},
		{"free, upgrade", "free", "0.9.0", true, false},
	} {
		t.Run(tc.label, func(t *testing.T) {
			setupOfflineRegistry(t, mockNsentryRegistry())
			pluginDir := t.TempDir()
			dir := filepath.Join(pluginDir, name)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			manifest := `{"name":"` + name + `","version":"` + tc.version + `","tier":"` + tc.tier + `","description":"x","category":"monitoring","license":"MIT"}`
			if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			installed := map[string]bool{}
			_, err := Install(opCtx(t), "nsentry", InstallOpts{
				PluginDir:             pluginDir,
				Out:                   &out,
				bundleEntitledChecker: func(context.Context, string, string) (bool, error) { return true, nil },
				licenseChecker:        func(context.Context, []string) error { return nil },
				licensedPair:          func(_ context.Context, n string) bool { return tc.pair && n == name },
				installer: func(_ context.Context, _ *config.Config, n, pd string) error {
					if n == name {
						if _, serr := os.Stat(filepath.Join(pd, n)); serr == nil {
							t.Errorf("upgrade must move the old copy aside before installing it")
						}
					}
					installed[n] = true
					d := filepath.Join(pd, n)
					if err := os.MkdirAll(d, 0o700); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(d, "plugin.json"), []byte(`{"name":"`+n+`","version":"1.0.0","description":"x","category":"monitoring","license":"MIT"}`), 0o600)
				},
			})
			if err != nil {
				t.Fatalf("install: %v\n%s", err, out.String())
			}
			got := strings.Contains(out.String(), "kept the installed free copy")
			if got != tc.wantNote {
				t.Errorf("note printed = %v, want %v\n%s", got, tc.wantNote, out.String())
			}
			if tc.wantNote && !strings.Contains(out.String(), "nself plugin update "+name+" --tier licensed") {
				t.Errorf("note does not name the switch command:\n%s", out.String())
			}
		})
	}
}
