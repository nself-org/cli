package commands

// Tests for P7-LIVE-10 fix round 4: the plugin step installs the versions the
// plan showed and proves it, the cache service follows the engine the project
// asks for (or the mode default), and the strict registry never falls back.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/upgrade/plan"
)

// A stale shared cache (ai 1.0.0, still fresh by age) must not decide what the
// plugin step installs: --yes primes the cache with the plan's registry, so
// Install resolves 2.0.0. --plan alone writes no cache file.
func TestUpdateYesPrimesRegistryCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cacheFile := filepath.Join(plugin.DefaultCacheDir(), "registry.json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, []byte(`{"version":"1","plugins":[{"name":"ai","version":"1.0.0"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	updateProject(t)
	newUpdateRecorder(t)
	var seen string
	old := newUpdateDeps
	newUpdateDeps = func() updateDeps {
		d := old()
		d.primeRegistry = func(reg *plugin.Registry) error {
			return plugin.WriteRegistryCache(plugin.DefaultCacheDir(), reg) // the real default
		}
		d.updatePlugin = func(ctx context.Context, _, slug string) error {
			// What plugin.Install does: read the shared cache (fresh, no network).
			reg, err := plugin.FetchRegistry(ctx, "", plugin.DefaultCacheDir())
			if err != nil {
				return err
			}
			for _, m := range reg.Plugins {
				if m.Name == slug {
					seen = m.Version
				}
			}
			return nil
		}
		return d
	}
	t.Cleanup(func() { newUpdateDeps = old })

	before, _ := os.ReadFile(cacheFile)
	if _, _, err := runUpdate(t, "--plan"); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(cacheFile); !bytes.Equal(before, after) {
		t.Fatalf("--plan changed the shared registry cache:\n%s", after)
	}
	if _, _, err := runUpdate(t, "--yes", "--cli=false"); err != nil {
		t.Fatal(err)
	}
	if seen != "2.0.0" {
		t.Fatalf("the plugin step resolved ai %q, want the planned 2.0.0 (stale cache held 1.0.0)", seen)
	}
	// Windows has no POSIX permission bits (it reads back 0666).
	if runtime.GOOS != "windows" {
		if st, err := os.Stat(cacheFile); err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("cache file mode = %v err = %v, want 0600", st.Mode().Perm(), err)
		}
	}
}

// A plugin that ends at another version than the plan showed fails the apply
// with planned vs installed named, and the lock pins are restored.
func TestUpdateYesFailsOnInstalledVersionMismatch(t *testing.T) {
	dir := updateProject(t)
	r := newUpdateRecorder(t)
	r.installed = map[string]string{"ai": "1.0.0"}
	orig, _ := os.ReadFile(lock.Path(dir))
	_, _, err := runUpdate(t, "--yes", "--cli=false")
	if err == nil {
		t.Fatal("a plugin left at 1.0.0 while the plan said 2.0.0 was reported as success")
	}
	for _, want := range []string{`"ai"`, "plan said 2.0.0", "1.0.0 is installed", "were restored"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err.Error(), want)
		}
	}
	if after, _ := os.ReadFile(lock.Path(dir)); string(after) != string(orig) {
		t.Fatalf("lock not restored:\n%s", after)
	}
	for _, c := range r.calls {
		if c == "build" {
			t.Fatal("the build ran after a version mismatch")
		}
	}
}

// The redis service moves to the engine REDIS_ENGINE names, else the mode
// default; the lock pin does not decide it. A move carries a note.
func TestUpdatePlanCacheEngine(t *testing.T) {
	rd, ok1 := compose.LockedRef("redis")
	vk, ok2 := compose.LockedRef("valkey")
	if !ok1 || !ok2 || rd.String() == vk.String() {
		t.Fatal("image lock needs distinct redis and valkey entries")
	}
	redisPin := rd.Repository + ":old@" + updOld
	valkeyPin := vk.Repository + ":old@" + updOld
	for _, tc := range []struct {
		name     string
		v15      bool
		pin, eng string
		want     compose.Ref
		note     string // substring, "" = no note
	}{
		{"v1.5 default moves redis to valkey", true, redisPin, "", vk, "valkey_data"},
		{"v1.4 default keeps redis", false, redisPin, "", rd, ""},
		{"v1.5 explicit redis stays", true, redisPin, "redis", rd, ""},
		{"v1.4 explicit valkey moves", false, redisPin, "valkey", vk, "valkey_data"},
		{"v1.5 valkey pin stays valkey", true, valkeyPin, "", vk, ""},
		{"v1.4 default moves a valkey pin back", false, valkeyPin, "", rd, "redis_data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, tc.v15)
			cand, found := imageCandidate("redis", tc.pin, tc.eng)
			if !found || cand.Value != tc.want.String() {
				t.Fatalf("candidate = %+v found=%v, want %s", cand, found, tc.want.String())
			}
			if tc.note == "" && cand.Note != "" {
				t.Fatalf("unexpected note %q", cand.Note)
			}
			for _, w := range []string{tc.note, "old volume", "kept", "not migrated"} {
				if tc.note != "" && !strings.Contains(cand.Note, w) {
					t.Fatalf("note %q lacks %q", cand.Note, w)
				}
			}
		})
	}
}

// End to end through gatherPlan: REDIS_ENGINE from the environment decides, and
// the plan row is resolvable with the note in the rendered table.
func TestUpdatePlanCacheEngineFromConfig(t *testing.T) {
	vk, _ := compose.LockedRef("valkey")
	rd, _ := compose.LockedRef("redis")
	for _, tc := range []struct {
		eng  string
		want compose.Ref
	}{{"", vk}, {"redis", rd}, {"valkey", vk}} {
		t.Run("REDIS_ENGINE="+tc.eng, func(t *testing.T) {
			updateProject(t)
			compattest.Set(t, true) // after updateProject, which pins v1.4
			t.Setenv("REDIS_ENGINE", tc.eng)
			newUpdateRecorder(t)
			l, err := lock.Load(".")
			if err != nil {
				t.Fatal(err)
			}
			p, err := gatherPlan(context.Background(), newUpdateDeps(), *l, "")
			if err != nil {
				t.Fatal(err)
			}
			var row *plan.Pin
			for i := range p.Pins {
				if p.Pins[i].Kind == plan.KindImage && p.Pins[i].Name == "redis" {
					row = &p.Pins[i]
				}
			}
			if row == nil || !row.Resolvable || row.To != tc.want.String() {
				t.Fatalf("redis row = %+v, want To %s", row, tc.want.String())
			}
			var out bytes.Buffer
			if err := plan.Render(&out, p); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(out.String(), "queued jobs are not migrated"); got != (tc.want.String() == vk.String()) {
				t.Fatalf("note shown = %v for engine %q:\n%s", got, tc.eng, out.String())
			}
		})
	}
}

// A 200 whose body does not parse is an error from the one request: the strict
// registry never retries on the network or the fallback host.
func TestUpdateStrictRegistryBadBodyNoFallback(t *testing.T) {
	compattest.Set(t, false)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`{"plugins": [oops`))
	}))
	defer srv.Close()
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("HOME", t.TempDir())
	start := time.Now()
	reg, err := strictRegistry(context.Background())
	if err == nil {
		t.Fatalf("a malformed 200 body gave a registry: %+v", reg)
	}
	if !strings.Contains(err.Error(), "does not parse") {
		t.Errorf("error = %v", err)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Fatalf("registry server hit %d times, want exactly 1", n)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("strictRegistry took long enough to suggest a fallback fetch")
	}
}

// Round 5: --check only reports, so combined with --plan or --yes it is a usage
// error raised before any dependency runs (it used to apply changes).
func TestUpdateCheckWithYesOrPlanIsUsageError(t *testing.T) {
	updateProject(t)
	r := newUpdateRecorder(t)
	for _, args := range [][]string{{"--check", "--yes"}, {"--check", "--plan"}, {"--check", "--yes", "--json"}} {
		_, _, err := runUpdate(t, args...)
		if err == nil || errs.Describe(err).Code != "E401" {
			t.Fatalf("%v: err = %v, want E401", args, err)
		}
		if len(r.calls) != 0 || r.primed != nil {
			t.Fatalf("%v ran deps: calls=%v primed=%v", args, r.calls, r.primed)
		}
	}
}

// The update schema accepts an engine-switch envelope: a redis pin moving to
// valkey carries the optional note, in data.pins and in the images step.
func TestUpdateEngineSwitchEnvelopeMatchesSchema(t *testing.T) {
	updateProject(t)
	compattest.Set(t, true) // after updateProject, which pins v1.4
	t.Setenv("REDIS_ENGINE", "")
	newUpdateRecorder(t)
	out, _ := pilotBuffers(t)
	if _, _, err := runUpdate(t, "--plan", "--json"); err != nil {
		t.Fatal(err)
	}
	data := updateEnvelope(t, out.String(), "update")
	if !strings.Contains(out.String(), `"note"`) || !strings.Contains(out.String(), "valkey_data") {
		t.Fatalf("envelope carries no engine-switch note:\n%s", out.String())
	}
	if data["applied"] != false {
		t.Fatalf("applied = %v, want false", data["applied"])
	}
}
