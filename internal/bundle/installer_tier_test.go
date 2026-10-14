package bundle

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// A successful paid bundle install must explain a retained free tier even
// when its version is skipped or the install hook keeps the upgraded copy free.
func TestBundleInstallKeptFreeTierWarning(t *testing.T) {
	for _, tc := range []struct {
		name, version, tier, price string
		pair, warn                 bool
	}{
		{"lower free", "0.9.0", "free", "$10", true, true},
		{"same free", "1.0.0", "free", "$10", true, true},
		{"higher free", "2.0.0", "free", "$10", true, true},
		{"licensed", "1.0.0", "pro", "$10", true, false},
		{"free bundle", "1.0.0", "free", "FREE", true, false},
		{"free only", "1.0.0", "free", "$10", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := fmt.Sprintf(`{"name":"notify","version":"1.0.0","tier":"free","tier_pair":%t}`, tc.pair)
			if tc.pair {
				entries += `,{"name":"notify","version":"1.0.0","tier":"pro","tier_pair":true}`
			}
			setupOfflineRegistry(t, []byte(`{"plugins":[`+entries+`]}`))
			dir := t.TempDir()
			writeCopy := func(version string) error {
				if err := os.MkdirAll(filepath.Join(dir, "notify"), 0700); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "notify", "plugin.json"), []byte(fmt.Sprintf(`{"name":"notify","version":%q,"tier":%q,"description":"Notify","category":"utility","license":"MIT"}`, version, tc.tier)), 0600)
			}
			if err := writeCopy(tc.version); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			m := memberRun{
				b: Bundle{Slug: "claw", Price: tc.price}, out: &out,
				result: &InstallResult{}, planned: []string{"notify"},
				pins: VersionPins{"notify": "1.0.0"}, pluginDir: dir,
				installFn: func(context.Context, *config.Config, string, string) error { return writeCopy("1.0.0") },
			}
			if err := m.run(context.Background()); err != nil {
				t.Fatal(err)
			}
			wantInstalled := 0
			if tc.version == "0.9.0" {
				wantInstalled = 1
			}
			if len(m.result.Installed) != wantInstalled {
				t.Errorf("installed count = %d, want %d", len(m.result.Installed), wantInstalled)
			}
			warning := "kept notify free; run nself plugin update notify --tier licensed"
			if got := strings.Contains(out.String(), warning); got != tc.warn {
				t.Errorf("warning present = %v, want %v; output:\n%s", got, tc.warn, out.String())
			}
		})
	}
}
