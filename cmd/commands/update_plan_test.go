package commands

// Tests for `nself update --plan` and `--yes` (P7-LIVE-10): the pin table, a
// pin the source no longer offers, a rate-limited source, the apply order, the
// envelope, and the flag rules. Sources are fixtures: an httptest release
// endpoint and stub plugin/build/self-update functions that record their order.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/spf13/cobra"
)

const (
	updOld = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	updSvc = "redis"
)

// updateProject writes a project with a lock pinning updSvc at an old digest and
// plugin "ai" at 1.0.0, and makes it the working directory.
func updateProject(t *testing.T) string {
	t.Helper()
	ref, ok := compose.LockedRef(updSvc)
	if !ok {
		t.Fatalf("image lock has no %q entry", updSvc)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l := lock.New()
	l.CLI = "1.4"
	l.Images[updSvc] = ref.Repository + ":old@" + updOld
	l.Plugins["ai"] = lock.PluginPin{Version: "1.0.0", Tier: lock.TierFree}
	if err := writeLockFile(dir, l); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

// hashTree digests every file path and content under dir.
func hashTree(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, rerr := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		fmt.Fprintf(h, "%s\x00%x\x00", rel, b)
		return rerr
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// updateEnvelope validates stdout against the envelope and update schemas. The
// schemas are found relative to this package, so it leaves the fixture project.
func updateEnvelope(t *testing.T, stdout, command string) map[string]any {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	t.Chdir(filepath.Dir(file))
	return validateEnvelope(t, stdout, command)
}

type updRecorder struct {
	t     *testing.T
	calls []string
	reg   []plugin.PluginManifest
	cli   func() (string, string, error)
}

// install replaces newUpdateDeps with recording stubs.
func (r *updRecorder) install() {
	old := newUpdateDeps
	r.t.Cleanup(func() { newUpdateDeps = old })
	newUpdateDeps = func() updateDeps {
		return updateDeps{
			runningVersion: func() string { return "v1.4.0" },
			latestRelease:  func(string) (string, string, error) { return r.cli() },
			registry: func(context.Context) (*plugin.Registry, error) {
				return &plugin.Registry{Plugins: r.reg}, nil
			},
			writeLock: func(dir string, l lock.Lock) error {
				r.calls = append(r.calls, "images")
				return writeLockFile(dir, l)
			},
			updatePlugin: func(_ context.Context, _ string, slug string) error {
				r.calls = append(r.calls, "plugins:"+slug)
				return nil
			},
			build: func(_ context.Context, dir string) error {
				r.calls = append(r.calls, "build")
				// A real build re-renders the lock from the new state: images keep
				// the moved pins, plugins take the installed versions.
				l, err := lock.Load(dir)
				if err != nil {
					return err
				}
				ref, _ := compose.LockedRef(updSvc)
				if l.Images[updSvc] != ref.String() {
					r.t.Errorf("build ran before the image pin moved: %s", l.Images[updSvc])
				}
				l.Plugins["ai"] = lock.PluginPin{Version: "2.0.0", Tier: lock.TierFree}
				return writeLockFile(dir, *l)
			},
			selfUpdate: func(tag string) error { r.calls = append(r.calls, "cli:"+tag); return nil },
		}
	}
}

func newUpdateRecorder(t *testing.T) *updRecorder {
	r := &updRecorder{t: t, reg: []plugin.PluginManifest{{Name: "ai", Version: "2.0.0"}},
		cli: func() (string, string, error) { return "v1.4.9", "", nil }}
	r.install()
	return r
}

// runUpdate drives the update flags through a throwaway command.
func runUpdate(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	c := &cobra.Command{Use: "update"}
	addUpdatePlanFlags(c)
	c.Flags().Bool("cli", false, "")
	c.Flags().Bool("admin", false, "")
	c.Flags().String("version", "", "")
	c.Flags().Bool("json", false, "")
	var out, errOut bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&errOut)
	if err := c.ParseFlags(args); err != nil {
		t.Fatal(err)
	}
	handled, err := runUpdatePlanMode(c)
	if !handled && err == nil {
		t.Fatal("plan mode did not handle the invocation")
	}
	return out.String(), errOut.String(), err
}

func TestUpdatePlanTable(t *testing.T) {
	dir := updateProject(t)
	r := newUpdateRecorder(t)
	before := hashTree(t, dir)
	out, _, err := runUpdate(t, "--plan")
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := compose.LockedRef(updSvc)
	for _, want := range []string{"KIND", "cli", "v1.4.0", "v1.4.9", "image", updSvc, ":old@" + updOld, ref.String(), "plugin", "ai", "1.0.0", "2.0.0",
		"1. backup", "2. images", "3. plugins", "4. build", "5. start", "6. migrate", "7. readiness", "8. health", "9. cli"} {
		if !strings.Contains(out, want) {
			t.Errorf("plan output lacks %q:\n%s", want, out)
		}
	}
	if after := hashTree(t, dir); after != before {
		t.Fatal("--plan changed the project tree")
	}
	if len(r.calls) != 0 {
		t.Fatalf("--plan applied something: %v", r.calls)
	}
}

// Negative control: the tree hash really moves when something is applied, so
// the "unchanged" assertion above can fail.
func TestUpdatePlanTreeHashControl(t *testing.T) {
	dir := updateProject(t)
	newUpdateRecorder(t)
	before := hashTree(t, dir)
	if _, _, err := runUpdate(t, "--yes"); err != nil {
		t.Fatal(err)
	}
	if hashTree(t, dir) == before {
		t.Fatal("--yes left the tree untouched: the unchanged-tree check cannot detect writes")
	}
}

func TestUpdatePlanUnresolvable(t *testing.T) {
	updateProject(t)
	r := newUpdateRecorder(t)
	r.reg = nil // plugin ai was deleted upstream
	stub := httptest.NewServer(http.NotFoundHandler())
	defer stub.Close()
	oldURL := githubLatestReleaseURL
	githubLatestReleaseURL = stub.URL + "/releases/latest"
	defer func() { githubLatestReleaseURL = oldURL }()
	r.cli = fetchLatestRelease // the real lookup, answered 404 by the fixture

	out, _, err := runUpdate(t, "--plan")
	if err != nil {
		t.Fatalf("a 404 pin must be a row, not an error: %v", err)
	}
	for _, want := range []string{"unresolvable", "no such release (HTTP 404)", "not in the plugin registry"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	jout, _ := pilotBuffers(t)
	if _, _, err := runUpdate(t, "--plan", "--json"); err != nil {
		t.Fatal(err)
	}
	data := updateEnvelope(t, jout.String(), "update")
	pins, _ := data["plan"].(map[string]any)["pins"].([]any)
	bad := 0
	for _, p := range pins {
		m := p.(map[string]any)
		if m["resolvable"] == false && m["reason"] != "" {
			bad++
		}
	}
	if bad != 2 { // the CLI and plugin ai
		t.Fatalf("unresolvable rows with a reason = %d, want 2: %v", bad, pins)
	}
}

func TestUpdatePlanRateLimited(t *testing.T) {
	updateProject(t)
	r := newUpdateRecorder(t)
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	defer stub.Close()
	oldURL := githubLatestReleaseURL
	githubLatestReleaseURL = stub.URL + "/releases/latest"
	defer func() { githubLatestReleaseURL = oldURL }()
	r.cli = fetchLatestRelease

	out, _, err := runUpdate(t, "--plan")
	if err == nil {
		t.Fatalf("a 429 produced a plan instead of an error:\n%s", out)
	}
	if d := errs.Describe(err); d == nil || d.Code != "E400" || errs.ExitCodeFor(err) == 0 {
		t.Fatalf("error is not coded and non-zero: %v (%+v)", err, d)
	}
	if !strings.Contains(err.Error(), "429") && !strings.Contains(fmt.Sprint(errs.Describe(err)), "429") {
		t.Errorf("error does not say 429: %v", err)
	}
	if out != "" {
		t.Errorf("a plan was printed for a failed source: %q", out)
	}
	// The plugin registry failing is an error too.
	r.cli = func() (string, string, error) { return "v1.4.9", "", nil }
	old := newUpdateDeps
	newUpdateDeps = func() updateDeps {
		d := old()
		d.registry = func(context.Context) (*plugin.Registry, error) {
			return nil, errors.New("HTTP GET registry: status 429")
		}
		return d
	}
	t.Cleanup(func() { newUpdateDeps = old })
	if _, _, err := runUpdate(t, "--plan"); err == nil || errs.Describe(err).Code != "E400" {
		t.Fatalf("registry failure = %v, want E400", err)
	}
}

func TestUpdatePlanJSONEnvelope(t *testing.T) {
	updateProject(t)
	newUpdateRecorder(t)
	out, _ := pilotBuffers(t)
	if _, _, err := runUpdate(t, "--plan", "--json"); err != nil {
		t.Fatal(err)
	}
	data := updateEnvelope(t, out.String(), "update")
	if data["applied"] != false {
		t.Fatalf("applied = %v, want false", data["applied"])
	}
	steps, _ := data["steps"].([]any)
	if len(steps) != 9 {
		t.Fatalf("steps = %d, want 9", len(steps))
	}
}

func TestUpdateYesOrder(t *testing.T) {
	dir := updateProject(t)
	r := newUpdateRecorder(t)
	if _, _, err := runUpdate(t, "--yes"); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(r.calls, ","), "images,plugins:ai,build,cli:v1.4.9"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
	l, err := lock.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := compose.LockedRef(updSvc)
	if l.Images[updSvc] != ref.String() || l.Plugins["ai"].Version != "2.0.0" {
		t.Fatalf("lock does not reflect the new pins: %v %v", l.Images, l.Plugins)
	}

	// --cli=false skips the CLI; --cli alone applies only the CLI.
	for _, tc := range []struct{ name, flag, want string }{{"no cli", "--cli=false", "images,plugins:ai,build"}, {"only cli", "--cli", "cli:v1.4.9"}} {
		t.Run(tc.name, func(t *testing.T) {
			updateProject(t)
			rr := newUpdateRecorder(t)
			if _, _, err := runUpdate(t, "--yes", tc.flag); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(rr.calls, ","); got != tc.want {
				t.Fatalf("calls = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestUpdateYesJSONEnvelope(t *testing.T) {
	updateProject(t)
	newUpdateRecorder(t)
	out, _ := pilotBuffers(t)
	if _, _, err := runUpdate(t, "--yes", "--json"); err != nil {
		t.Fatal(err)
	}
	data := updateEnvelope(t, out.String(), "update")
	if data["applied"] != true {
		t.Fatalf("applied = %v, want true", data["applied"])
	}
	status := map[string]string{}
	for _, s := range data["steps"].([]any) {
		m := s.(map[string]any)
		status[m["name"].(string)] = m["status"].(string)
	}
	for _, n := range []string{"images", "plugins", "build", "cli"} {
		if status[n] != "done" {
			t.Errorf("step %s = %q, want done", n, status[n])
		}
	}
	if status["start"] != "skipped" {
		t.Errorf("start = %q, want skipped (update stack runs it)", status["start"])
	}
}

func TestUpdatePlanFlagRules(t *testing.T) {
	updateProject(t)
	newUpdateRecorder(t)
	for _, tc := range []struct {
		name string
		args []string
		code string
	}{
		{"plan and yes", []string{"--plan", "--yes"}, "E401"},
		{"json alone", []string{"--json"}, "E402"},
		{"admin", []string{"--plan", "--admin"}, "E401"},
	} {
		if _, _, err := runUpdate(t, tc.args...); err == nil || errs.Describe(err).Code != tc.code {
			t.Errorf("%s: err = %v, want %s", tc.name, err, tc.code)
		}
	}
	if err := os.Remove("nself.lock.yaml"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runUpdate(t, "--plan"); err == nil || errs.Describe(err).Code != "E455" {
		t.Errorf("no lock: err = %v, want E455", err)
	}
	c := &cobra.Command{Use: "update"}
	addUpdatePlanFlags(c)
	c.Flags().Bool("json", false, "")
	if handled, err := runUpdatePlanMode(c); handled || err != nil {
		t.Errorf("plain update must fall through: handled=%v err=%v", handled, err)
	}
}
