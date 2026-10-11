package bundle

// remover_catalog_test.go — P7-PLUG-20 fix round 2: the guard and the ledger
// bootstrap load the bundle catalog themselves (M1), a plugin the catalog lists
// under another recorded bundle is never removed by a bundle remove (S1), and
// ForgetPlugin treats only a missing file as "no ledger" (N1).

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

// catalogServer serves the overlap catalog and points the loader at it, with an
// empty cache and the catalog unloaded; everything is restored when t ends.
func catalogServer(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "overlap", "bundles.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(raw) }))
	t.Cleanup(srv.Close)
	isolateCatalog(t, srv.URL)
}

// isolateCatalog unloads the catalog, aims the loader at url and gives it an empty cache.
func isolateCatalog(t *testing.T, url string) {
	t.Helper()
	t.Setenv("NSELF_BUNDLES_URL", url)
	t.Setenv("NSELF_BUNDLES_CACHE_PATH", filepath.Join(t.TempDir(), "bundles-cache.json"))
	t.Cleanup(UnloadCatalogForTest())
}

func TestGuardLoadsCatalogWithoutLedger(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		t.Run("catalog reachable", func(t *testing.T) {
			catalogServer(t)
			dir := t.TempDir()
			installPlugins(t, dir, "px", "py") // bundle chat fully on disk, no ledger file
			var w bytes.Buffer
			err := GuardPluginRemove(&w, dir, "py", false)
			if compat.V15() {
				if d := errs.Describe(err); err == nil || d == nil || d.Code != "E133" || !strings.Contains(err.Error(), "chat") {
					t.Fatalf("v1.5: err = %v, want E133 naming chat", err)
				}
			} else if err != nil || !strings.Contains(w.String(), "chat") {
				t.Fatalf("v1.4: err=%v warning=%q, want nil plus a warning naming chat", err, w.String())
			}
			if _, statErr := os.Stat(newLedgerStore(dir).Path()); !os.IsNotExist(statErr) {
				t.Errorf("the guard wrote a ledger: %v", statErr)
			}
		})
		t.Run("catalog unreachable", func(t *testing.T) {
			dead := httptest.NewServer(http.NotFoundHandler())
			url := dead.URL
			dead.Close()
			isolateCatalog(t, url)
			dir := t.TempDir()
			installPlugins(t, dir, "px", "py")
			var w bytes.Buffer
			err := GuardPluginRemove(&w, dir, "py", false)
			if compat.V15() {
				if err == nil || strings.Contains(err.Error(), "E133") || !strings.Contains(err.Error(), "--force") {
					t.Fatalf("v1.5: err = %v, want a refusal that cannot read the catalog and names --force", err)
				}
				if err := GuardPluginRemove(&w, dir, "py", true); err != nil {
					t.Errorf("v1.5 --force: %v", err)
				}
			} else if err != nil || !strings.Contains(w.String(), "could not read") {
				t.Fatalf("v1.4: err=%v warning=%q, want nil plus a warning", err, w.String())
			}
		})
	})
}

// TestRemoveKeepsCatalogMemberOfRecordedBundle is the Opus sequence: chat={px,py}
// and claw={py,pz} are recorded, a forced plugin remove forgets py, a reinstall
// of chat records py under chat only; removing chat must still keep py.
func TestRemoveKeepsCatalogMemberOfRecordedBundle(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		useOverlapBundles(t)
		dir := t.TempDir()
		installPlugins(t, dir, "px", "py", "pz")
		recordBundle(t, dir, "chat")
		recordBundle(t, dir, "claw")
		if err := ForgetPlugin(dir, "py"); err != nil {
			t.Fatal(err)
		}
		recordBundle(t, dir, "chat") // py comes back with installed_by [chat] only
		if by := loadLedger(t, dir).Plugins["py"].InstalledBy; !sameStrings(by, []string{"chat"}) {
			t.Fatalf("setup: py installed_by = %v, want [chat]", by)
		}
		var out bytes.Buffer
		rec := &recorder{}
		opts := removeOpts(dir, &out, rec)
		opts.KeepData = true
		res, err := Remove(context.Background(), "chat", opts)
		if err != nil {
			t.Fatalf("remove chat: %v\n%s", err, out.String())
		}
		if !sameStrings(rec.names(), []string{"px"}) {
			t.Errorf("removed %v, want only px (py is a claw member)", rec.names())
		}
		if len(res.Kept) != 1 || res.Kept[0].Name != "py" || res.Kept[0].Reason != "member of claw" {
			t.Errorf("kept = %+v, want py kept as a member of claw", res.Kept)
		}
		if !isAlreadyInstalled(dir, "py") {
			t.Error("py was removed")
		}
		l := loadLedger(t, dir) // validates
		if by := l.Plugins["py"].InstalledBy; !sameStrings(by, []string{"claw"}) || l.Plugins["py"].Explicit {
			t.Errorf("py record = %+v, want installed_by [claw], not explicit", l.Plugins["py"])
		}
		if _, ok := l.Bundles["chat"]; ok {
			t.Error("chat is still recorded")
		}
	})
}

// TestForgetPluginStatError: only a missing ledger file means "no ledger".
func TestForgetPluginStatError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a stat through a regular file maps to ErrNotExist on Windows, so the trick yields no non-ENOENT error")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".nself-state"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ForgetPlugin(dir, "py"); err == nil {
		t.Error("a ledger path that cannot be stat'ed (not ENOENT) was treated as no ledger")
	}
}

// TestRemovePlanDataLine: the plan names --keep-data as the reason data is kept only
// where the default would drop it (v1.4, paid bundle); otherwise it prints the default line.
func TestRemovePlanDataLine(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		useOverlapBundles(t)
		dir := t.TempDir()
		installPlugins(t, dir, "px", "py", "pg")
		recordBundle(t, dir, "chat")
		recordBundle(t, dir, "task")
		plan := func(bundleSlug string, mut func(*RemoveOpts)) string {
			var out bytes.Buffer
			opts := removeOpts(dir, &out, &recorder{})
			opts.DryRun = true
			mut(&opts)
			if _, err := Remove(context.Background(), bundleSlug, opts); err != nil {
				t.Fatal(err)
			}
			for _, line := range strings.Split(out.String(), "\n") {
				if strings.Contains(line, "data:") {
					return strings.TrimSpace(line)
				}
			}
			t.Fatalf("no data line in:\n%s", out.String())
			return ""
		}
		const keepDefault = "data: keep (default; --delete-data deletes it)"
		withKeep := plan("chat", func(o *RemoveOpts) { o.KeepData = true })
		if compat.V15() && withKeep != keepDefault {
			t.Errorf("v1.5 paid with --keep-data (a no-op): %q, want %q", withKeep, keepDefault)
		}
		if !compat.V15() && withKeep != "data: preserve (--keep-data)" {
			t.Errorf("v1.4 paid with --keep-data: %q, want the --keep-data line", withKeep)
		}
		if got := plan("task", func(o *RemoveOpts) { o.KeepData = true }); got != keepDefault {
			t.Errorf("free bundle with --keep-data: %q, want %q", got, keepDefault)
		}
		if got := plan("task", func(*RemoveOpts) {}); got != keepDefault {
			t.Errorf("free bundle, no flags: %q, want %q", got, keepDefault)
		}
		if got := plan("chat", func(o *RemoveOpts) { o.DeleteData = true }); got != "data: drop plugin schema/tables" {
			t.Errorf("--delete-data: %q", got)
		}
	})
}
