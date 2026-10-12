package commands

// Tests for the candidate sources and the failure handling of `update --yes`
// (P7-LIVE-10 fix round 3): image variants, plugin tiers, the strict registry,
// the transactional pins and the bundle ledger refresh.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/plugin"
)

// A pgvector pin is offered the pgvector entry, never plain postgres; a pin of
// another repository is a row with a reason; the functions runtimes likewise.
func TestUpdatePlanImageVariant(t *testing.T) {
	pgv, ok := compose.LockedRef("pgvector")
	plain, ok2 := compose.LockedRef("postgres")
	if !ok || !ok2 {
		t.Fatal("image lock lacks postgres or pgvector")
	}
	if plain.String() == pgv.String() {
		t.Fatal("fixture assumption broken: the two variants are the same ref")
	}
	oldPgv := pgv.Repository + ":old@" + updOld
	cand, found := imageCandidate("postgres", oldPgv, "")
	if !found || cand.Value != pgv.String() {
		t.Fatalf("pgvector pin offered %+v, want %s (keying by service name offered %s)", cand, pgv.String(), plain.String())
	}
	if cand, _ := imageCandidate("postgres", plain.Repository+":old@"+updOld, ""); cand.Value != plain.String() {
		t.Fatalf("plain pin offered %+v", cand)
	}
	if cand, _ := imageCandidate("postgres", "docker.io/custom/pg:1@"+updOld, ""); cand.Value != "" || cand.Reason == "" {
		t.Fatalf("a foreign repository must be unresolvable with a reason: %+v", cand)
	}
	deno, _ := compose.LockedRef("deno")
	if cand, _ := imageCandidate("functions", deno.Repository+":old@"+updOld, ""); cand.Value != deno.String() {
		t.Fatalf("deno runtime pin offered %+v, want %s", cand, deno.String())
	}
	if _, found := imageCandidate("no-such-service", "x:1", ""); found {
		t.Fatal("an unknown key must have no candidate")
	}
}

// Free and licensed entries of one slug resolve the way plugin.Update does.
func TestUpdatePlanTierPair(t *testing.T) {
	compattest.Set(t, true) // v1.5: the installed tier sticks
	reg := []plugin.PluginManifest{
		{Name: "cron", Version: "1.0.0", Tier: "free", TierPair: true},
		{Name: "cron", Version: "1.1.2", Tier: "pro", TierPair: true, Bundles: []string{"claw"}},
	}
	for _, tc := range []struct{ tier, want string }{{lock.TierFree, "1.0.0"}, {lock.TierLicensed, "1.1.2"}} {
		updateProject(t)
		r := newUpdateRecorder(t)
		r.reg = reg
		l, err := lock.Load(".")
		if err != nil {
			t.Fatal(err)
		}
		l.Plugins = map[string]lock.PluginPin{"cron": {Version: "0.9.0", Tier: tc.tier}}
		p, err := gatherPlan(context.Background(), newUpdateDeps(), *l, "")
		if err != nil {
			t.Fatal(err)
		}
		var got string
		for _, pin := range p.Pins {
			if pin.Kind == "plugin" && pin.Name == "cron" {
				got = pin.To
			}
		}
		if got != tc.want {
			t.Errorf("lock tier %s: cron offered %q, want %q", tc.tier, got, tc.want)
		}
	}
}

// The registry used for planning has no cache and no fallback: a 429 is an
// error even when a fresh-looking cache file sits in the plugin cache.
func TestUpdatePlanStrictRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cache := plugin.DefaultCacheDir()
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := `{"version":"1","plugins":[{"name":"ai","version":"9.9.9"}]}`
	if err := os.WriteFile(filepath.Join(cache, "registry.json"), []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(filepath.Join(cache, "registry.json"), old, old)

	status := http.StatusTooManyRequests
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status != http.StatusOK {
			http.Error(w, "slow down", status)
			return
		}
		_, _ = w.Write([]byte(`{"version":"1","plugins":[{"name":"ai","version":"2.0.0"}]}`))
	}))
	defer srv.Close()
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)

	if reg, err := strictRegistry(context.Background()); err == nil || !strings.Contains(err.Error(), "status 429") {
		t.Fatalf("a 429 gave reg=%v err=%v, want an error naming status 429 (the stale cache has ai 9.9.9)", reg, err)
	}
	status = http.StatusOK
	reg, err := strictRegistry(context.Background())
	if err != nil || len(reg.Plugins) != 1 || reg.Plugins[0].Version != "2.0.0" {
		t.Fatalf("200 answer: reg=%+v err=%v", reg, err)
	}
}

// A failed build restores the old pins, so a retry sees the same diff and
// rebuilds. The same for a failed plugin update after the image pins moved.
func TestUpdateYesRestoresPinsOnFailure(t *testing.T) {
	for _, tc := range []string{"build", "plugin"} {
		t.Run(tc, func(t *testing.T) {
			dir := updateProject(t)
			r := newUpdateRecorder(t)
			orig, err := os.ReadFile(lock.Path(dir))
			if err != nil {
				t.Fatal(err)
			}
			if tc == "build" {
				r.buildErr = errors.New("compose render failed")
			} else {
				r.pluginErr = errors.New("download failed")
			}
			if _, _, err := runUpdate(t, "--yes"); err == nil || !strings.Contains(err.Error(), "were restored") {
				t.Fatalf("err = %v, want a failure that says the pins were restored", err)
			}
			after, _ := os.ReadFile(lock.Path(dir))
			if string(after) != string(orig) {
				t.Fatalf("lock changed after a failed apply:\n%s", after)
			}
			// Retry: the diff is still there and the build runs.
			r.buildErr, r.pluginErr, r.calls = nil, nil, nil
			if _, _, err := runUpdate(t, "--yes", "--cli=false"); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(r.calls, ","); got != "images,plugins:ai,build" {
				t.Fatalf("retry calls = %s", got)
			}
		})
	}
}

// After a plugin update the bundle ledger record takes the installed version,
// keeping its owners; without a ledger nothing is created.
func TestUpdateRefreshesBundleLedger(t *testing.T) {
	dir := t.TempDir()
	if err := refreshBundleLedger(dir, "bots"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bundle.LedgerStore(dir).Path()); !os.IsNotExist(err) {
		t.Fatalf("a ledger was created: %v", err)
	}
	writeLedgerFixturePlugin(t, dir, "bots", "1.0.0")
	store := bundle.LedgerStore(dir)
	old := strings.Repeat("a", 64)
	err := store.Update(func(l *ledger.Ledger) error {
		l.AddBundle("nclaw", []ledger.InstalledPlugin{{Name: "bots", Version: "1.0.0", Tier: ledger.TierFree, Checksum: old}}, time.Now())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	writeLedgerFixturePlugin(t, dir, "bots", "2.0.0") // what plugin.Update leaves on disk
	l, _, _ := store.Load()
	before := l.Plugins["bots"]
	if before.Version != "1.0.0" || before.Checksum != old {
		t.Fatalf("precondition: the ledger should still say 1.0.0, got %+v", before)
	}
	if err := refreshBundleLedger(dir, "bots"); err != nil {
		t.Fatal(err)
	}
	l, _, _ = store.Load()
	rec := l.Plugins["bots"]
	if rec.Version != "2.0.0" || rec.Checksum == old || rec.Explicit != before.Explicit || strings.Join(rec.InstalledBy, ",") != "nclaw" {
		t.Fatalf("record after refresh = %+v", rec)
	}
}
