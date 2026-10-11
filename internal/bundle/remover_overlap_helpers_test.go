package bundle

// remover_overlap_helpers_test.go — shared fixtures of the P7-PLUG-20 remove
// tests: the synthetic overlapping bundles (testdata/overlap/bundles.json), a
// recording remover seam and ledger helpers. No network, temp dirs only.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/config"
)

// useOverlapBundles swaps the bundle catalog for the synthetic one and
// restores the shared fixture when the test ends.
func useOverlapBundles(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "overlap", "bundles.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := LoadBytes(raw); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = LoadBytes([]byte(fixtureBundlesJSON)) })
}

// installPlugins puts a plugin.json plugin on disk for every name.
func installPlugins(t *testing.T, pluginDir string, names ...string) {
	t.Helper()
	for _, n := range names {
		writeMember(t, pluginDir, n, 1, nil)
	}
}

// recordBundle records the bundle (all its members) in the ledger of pluginDir.
func recordBundle(t *testing.T, pluginDir, slug string) {
	t.Helper()
	b, ok := Get(slug)
	if !ok {
		t.Fatalf("unknown bundle %s", slug)
	}
	var members []ledger.InstalledPlugin
	for _, p := range b.Plugins {
		members = append(members, ledger.InstalledPlugin{Name: p, Version: "1.0.0", Tier: ledger.TierFree})
	}
	err := newLedgerStore(pluginDir).Update(func(l *ledger.Ledger) error {
		l.AddBundle(slug, members, time.Now())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// loadLedger reads the ledger of pluginDir (Load validates it).
func loadLedger(t *testing.T, pluginDir string) ledger.Ledger {
	t.Helper()
	l, _, err := newLedgerStore(pluginDir).Load()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// ledgerBytes returns the ledger file, or nil when there is none.
func ledgerBytes(t *testing.T, pluginDir string) []byte {
	t.Helper()
	data, err := os.ReadFile(newLedgerStore(pluginDir).Path())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// removeCall is one call the recording remover saw.
type removeCall struct {
	Name     string
	KeepData bool
	Force    bool
}

// recorder is a remover seam that deletes the plugin directory (like
// plugin.Remove) and records its calls; failOn names a plugin it refuses.
type recorder struct {
	calls  []removeCall
	failOn string
}

func (r *recorder) fn(_ context.Context, _ *config.Config, name, pluginDir string, keepData, force bool) error {
	r.calls = append(r.calls, removeCall{name, keepData, force})
	if name == r.failOn {
		return os.ErrPermission
	}
	return os.RemoveAll(filepath.Join(pluginDir, name))
}

func (r *recorder) names() []string {
	var out []string
	for _, c := range r.calls {
		out = append(out, c.Name)
	}
	return out
}

// removeOpts is a baseline RemoveOpts over pluginDir with the recorder wired.
func removeOpts(pluginDir string, out *bytes.Buffer, r *recorder) RemoveOpts {
	return RemoveOpts{PluginDir: pluginDir, Out: out, Config: &config.Config{}, remover: r.fn}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
