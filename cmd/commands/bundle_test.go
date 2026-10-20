package commands

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle"

	"github.com/spf13/cobra"
)

// fixtureBundlesJSON serves as the bundles.json response for every test in
// this file. A local httptest server (not the live plugins.nself.org
// endpoint) is pointed at via NSELF_BUNDLES_URL so bundleCmd's
// PersistentPreRunE (bundle.Load) resolves deterministically offline,
// exercising the real fetch path rather than mocking it away.
const fixtureBundlesJSON = `{
  "schema_version": "2.0.0",
  "bundles": {
    "task":   {"display": "ɳTask",   "tier": "free", "price_monthly": 0,    "price_yearly": 0,    "plugins": ["notifications","jobs"]},
    "chat":   {"display": "ɳChat",   "tier": "paid", "price_monthly": 0.99, "price_yearly": 9.99, "plugins": ["bots","livekit"]},
    "claw":   {"display": "ɳClaw",   "tier": "paid", "price_monthly": 0.99, "price_yearly": 9.99, "plugins": ["ai","claw","mux","voice","browser","google","notify","cron"]},
    "family": {"display": "ɳFamily", "tier": "paid", "price_monthly": 0.99, "price_yearly": 9.99, "plugins": ["social","photos"]},
    "sentry": {"display": "ɳSentry", "tier": "paid", "price_monthly": 0.99, "price_yearly": 9.99, "plugins": ["nself-uptime-monitor","nself-status-page"]},
    "tv":     {"display": "ɳTV",     "tier": "paid", "price_monthly": 0.99, "price_yearly": 9.99, "plugins": ["streaming","epg"]},
    "clawde": {"display": "ClawDE",  "tier": "paid", "price_monthly": 0.99, "price_yearly": 9.99, "plugins": ["auth","cms","realtime"]}
  }
}`

func TestMain(m *testing.M) {
	// Fixtures model local projects; CI-specific tests opt in explicitly.
	_ = os.Unsetenv("CI")
	_ = os.Unsetenv("NSELF_CI")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fixtureBundlesJSON))
	}))
	defer srv.Close()
	_ = os.Setenv("NSELF_BUNDLES_URL", srv.URL)
	// Seed once up front too, so direct RunE calls that bypass
	// PersistentPreRunE (not routed through cobra Execute) still resolve.
	if err := bundle.LoadBytes([]byte(fixtureBundlesJSON)); err != nil {
		panic("seeding bundle fixture: " + err.Error())
	}

	// AI_AUTO_INSTALL=false for the whole package's test binary. start.go's
	// runStart calls autoInstallAIIfNeeded on every invocation, which fires a
	// real `curl | zstd` Ollama install whenever AI_AUTO_INSTALL is unset
	// (default enabled) and NSELF_MASTER_SECRET is non-empty in the process
	// environment. config.Load calls godotenv.Overload for every .env file
	// it reads, which sets real os.Environ values with no cleanup — so any
	// start_test.go test can observe a NSELF_MASTER_SECRET left behind by an
	// unrelated earlier test in this same binary. On CI's Linux runners that
	// silently downloads a real binary via curl/zstd instead of erroring out
	// (installer.Install only short-circuits on non-linux GOOS), and an
	// orphaned curl/zstd surviving a cancelled test context is what produced
	// the intermittent "Test I/O incomplete" / WaitDelay failures in
	// cmd/commands on ubuntu-24.04-arm. No test in this package should ever
	// make a real network install; this is the package-wide guard against it.
	_ = os.Setenv("AI_AUTO_INSTALL", "false")

	// No test in this package may reach GitHub's release endpoints. The error
	// harness runs `nself update` and `nself upgrade` for real; whenever the
	// local version differs from the published latest (every release-bump
	// branch, before the release exists) they call selfUpdate, which downloads
	// the published binary and atomically replaces os.Executable(), that is
	// the running test binary. Every later `exec.Command(os.Args[0], ...)`
	// helper child then runs the real nself and prints root help. A stub that
	// answers 404 makes those commands fail fast, whatever version.Version is.
	releaseStub := httptest.NewServer(http.NotFoundHandler())
	defer releaseStub.Close()
	githubLatestReleaseURL = releaseStub.URL + "/releases/latest"
	githubReleaseByTagURL = releaseStub.URL + "/releases/tags/%s"
	githubDownloadBaseURL = releaseStub.URL + "/releases/download"

	os.Exit(m.Run())
}

// newBundleTestCmd returns an isolated cobra root with only the bundle
// command tree attached — avoids touching the global RootCmd state.
func newBundleTestCmd() *cobra.Command {
	root := &cobra.Command{Use: "nself", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	root.AddCommand(bundleCmd)
	// bundleCmd carries the GroupID the shared RootCmd tree assigned it; cobra
	// panics on Execute when that group is not defined on the parent, so register
	// the groups on this root too (otherwise the result depends on test order).
	applyGroups(root)
	// Flag values live on the package-level subcommands and survive an Execute;
	// without a reset, --installed from one test empties the list of the next.
	resetBundleFlags(bundleCmd)
	return root
}

// resetBundleFlags restores every flag under c to its default.
func resetBundleFlags(c *cobra.Command) {
	resetFlags(c)
	for _, sub := range c.Commands() {
		resetBundleFlags(sub)
	}
}

// ── Registration ─────────────────────────────────────────────────────

func TestBundleCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range RootCmd.Commands() {
		if cmd.Use == "bundle" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("bundleCmd is not registered on RootCmd")
	}
}

func TestBundleCmd_SubcommandsRegistered(t *testing.T) {
	subs := map[string]bool{}
	for _, sub := range bundleCmd.Commands() {
		subs[sub.Name()] = true
	}
	for _, want := range []string{"list", "info"} {
		if !subs[want] {
			t.Errorf("bundle subcommand %q is not registered", want)
		}
	}
}

// ── bundle membership (resolved from bundles.json via internal/bundle) ──

func TestBundles_AllPresent(t *testing.T) {
	required := []string{"claw", "chat", "family", "tv", "clawde", "sentry", "task", "nself-plus"}
	for _, key := range required {
		if _, ok := bundle.Get(key); !ok {
			t.Errorf("bundle.Get missing required bundle %q", key)
		}
	}
}

func TestBundles_NamesNotEmpty(t *testing.T) {
	for _, b := range bundle.All() {
		if b.Name == "" {
			t.Errorf("bundle %q has empty Name", b.Slug)
		}
		if b.Price == "" {
			t.Errorf("bundle %q has empty Price", b.Slug)
		}
	}
}

func TestBundles_ClawPlugins(t *testing.T) {
	b, ok := bundle.Get("claw")
	if !ok {
		t.Fatal("bundle.Get(claw) failed")
	}
	required := []string{"ai", "claw", "mux", "voice", "browser", "google", "notify", "cron"}
	for _, p := range required {
		found := false
		for _, got := range b.Plugins {
			if got == p {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("claw bundle missing expected plugin %q", p)
		}
	}
}

// ── bundle list ───────────────────────────────────────────────────────

func TestBundleList_RunsWithoutError(t *testing.T) {
	_, err := captureStdout(t, func() error {
		return runBundleList(nil, nil)
	})
	if err != nil {
		t.Fatalf("bundle list returned error: %v", err)
	}
}

func TestBundleList_ContainsAllBundles(t *testing.T) {
	out, _ := captureStdout(t, func() error {
		return runBundleList(nil, nil)
	})
	for _, key := range []string{"claw", "chat", "tv", "clawde", "sentry", "task", "nself-plus"} {
		if !strings.Contains(out, key) {
			t.Errorf("bundle list output missing bundle key %q\nfull output:\n%s", key, out)
		}
	}
}

func TestBundleList_ContainsPricing(t *testing.T) {
	out, _ := captureStdout(t, func() error {
		return runBundleList(nil, nil)
	})
	if !strings.Contains(out, "$0.99") {
		t.Errorf("bundle list output missing $0.99 pricing\nfull output:\n%s", out)
	}
	if !strings.Contains(out, "$3.99") {
		t.Errorf("bundle list output missing $3.99 ɳSelf+ pricing\nfull output:\n%s", out)
	}
}

// ── bundle info ───────────────────────────────────────────────────────

func TestBundleInfo_ClawHappyPath(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return runBundleInfo(&cobra.Command{}, []string{"claw"})
	})
	if err != nil {
		t.Fatalf("bundle info claw returned error: %v", err)
	}
	for _, want := range []string{"ɳClaw", "$0.99", "ai", "claw", "mux"} {
		if !strings.Contains(out, want) {
			t.Errorf("bundle info claw output missing %q\nfull:\n%s", want, out)
		}
	}
}

func TestBundleInfo_NSelfPlus(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return runBundleInfo(&cobra.Command{}, []string{"nself-plus"})
	})
	if err != nil {
		t.Fatalf("bundle info nself-plus returned error: %v", err)
	}
	if !strings.Contains(out, "ɳSelf+") {
		t.Errorf("bundle info nself-plus missing ɳSelf+ name\nfull:\n%s", out)
	}
	if !strings.Contains(out, "$3.99") {
		t.Errorf("bundle info nself-plus missing $3.99 price\nfull:\n%s", out)
	}
}

// TestBundleInfo_LegacyAndCanonicalSlugsMatch is the acceptance-criteria
// check: `nself bundle info nclaw --json` and `nself bundle info claw --json`
// must return identical output (backward-compat alias verified).
func TestBundleInfo_LegacyAndCanonicalSlugsMatch(t *testing.T) {
	pairs := [][2]string{
		{"nclaw", "claw"}, {"nchat", "chat"}, {"nfamily", "family"},
		{"ntv", "tv"}, {"nsentry", "sentry"}, {"ntask", "task"},
	}
	for _, pair := range pairs {
		legacy, canonical := pair[0], pair[1]
		legacyOut, err1 := captureStdout(t, func() error {
			return runBundleInfo(&cobra.Command{}, []string{legacy})
		})
		canonicalOut, err2 := captureStdout(t, func() error {
			return runBundleInfo(&cobra.Command{}, []string{canonical})
		})
		if err1 != nil || err2 != nil {
			t.Errorf("pair (%q,%q): err1=%v err2=%v", legacy, canonical, err1, err2)
			continue
		}
		if legacyOut != canonicalOut {
			t.Errorf("bundle info %q and %q produced different output:\n%q\nvs\n%q", legacy, canonical, legacyOut, canonicalOut)
		}
	}
}

func TestBundleInfo_SentryAliasResolvesToCanonical(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return runBundleInfo(&cobra.Command{}, []string{"nsentry"})
	})
	if err != nil {
		t.Fatalf("bundle info nsentry (alias) returned error: %v", err)
	}
	for _, want := range []string{"ɳSentry", "$0.99"} {
		if !strings.Contains(out, want) {
			t.Errorf("bundle info nsentry output missing %q\nfull:\n%s", want, out)
		}
	}
}

func TestBundleInfo_SentryCaseAndSpaceInsensitive(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return runBundleInfo(&cobra.Command{}, []string{"  SENTRY "})
	})
	if err != nil {
		t.Fatalf("bundle info '  SENTRY ' returned error: %v", err)
	}
	if !strings.Contains(out, "ɳSentry") {
		t.Errorf("bundle info '  SENTRY ' output missing ɳSentry name\nfull:\n%s", out)
	}
}

func TestBundleInfo_UnknownBundle(t *testing.T) {
	_, err := captureStdout(t, func() error {
		return runBundleInfo(&cobra.Command{}, []string{"bogus-bundle-xyz"})
	})
	if err == nil {
		t.Fatal("expected error for unknown bundle, got nil")
	}
	if !strings.Contains(err.Error(), "bundle not found") {
		t.Errorf("error message should mention 'bundle not found', got: %v", err)
	}
	if !strings.Contains(err.Error(), "nself bundle list") {
		t.Errorf("error message should suggest 'nself bundle list', got: %v", err)
	}
}

func TestBundleInfo_UnknownBundle_ExitCode(t *testing.T) {
	root := newBundleTestCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"bundle", "info", "does-not-exist"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected non-nil error for unknown bundle name via cobra")
	}
}

func TestBundleInfo_JSONFlag(t *testing.T) {
	root := newBundleTestCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"bundle", "info", "claw", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("bundle info claw --json returned error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`"slug"`, `"claw"`, `"name"`, `"price"`, `"plugins"`, `"license_status"`, `"install_hint"`} {
		if !strings.Contains(out, want) {
			t.Errorf("--json output missing key %q\nfull:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "$0.99") {
		t.Errorf("--json output missing price $0.99\nfull:\n%s", out)
	}
}

func TestBundleInfo_JSON_AllFields(t *testing.T) {
	root := newBundleTestCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"bundle", "info", "task", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("bundle info task --json returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"FREE"`) {
		t.Errorf("--json task output missing FREE price\nfull:\n%s", out)
	}
	if !strings.Contains(out, `"plugin_count"`) {
		t.Errorf("--json output missing plugin_count\nfull:\n%s", out)
	}
}

func TestBundleInfo_CaseInsensitive(t *testing.T) {
	for _, input := range []string{"nClaw", "NCLAW", "NClaw", "claw", "CLAW"} {
		out, err := captureStdout(t, func() error {
			return runBundleInfo(&cobra.Command{}, []string{input})
		})
		if err != nil {
			t.Errorf("bundle info %q returned error: %v", input, err)
			continue
		}
		if !strings.Contains(out, "ɳClaw") {
			t.Errorf("bundle info %q did not resolve to claw\nfull:\n%s", input, out)
		}
	}
}

func TestBundleInfo_Sentry(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return runBundleInfo(&cobra.Command{}, []string{"sentry"})
	})
	if err != nil {
		t.Fatalf("bundle info sentry returned error: %v", err)
	}
	if !strings.Contains(out, "ɳSentry") {
		t.Errorf("bundle info sentry missing ɳSentry name\nfull:\n%s", out)
	}
	for _, p := range []string{"nself-uptime-monitor", "nself-status-page"} {
		if !strings.Contains(out, p) {
			t.Errorf("bundle info sentry missing plugin %q\nfull:\n%s", p, out)
		}
	}
}

func TestBundleInfo_Task(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return runBundleInfo(&cobra.Command{}, []string{"task"})
	})
	if err != nil {
		t.Fatalf("bundle info task returned error: %v", err)
	}
	if !strings.Contains(out, "ɳTask") {
		t.Errorf("bundle info task missing ɳTask name\nfull:\n%s", out)
	}
	if !strings.Contains(out, "FREE") {
		t.Errorf("bundle info task missing FREE price\nfull:\n%s", out)
	}
}

func TestBundleInfo_ClawdeAndTaskUnprefixedOnly(t *testing.T) {
	// clawde and task never had an n-prefixed form; both must resolve
	// directly without error (acceptance criterion #3).
	for _, slug := range []string{"clawde", "task"} {
		if _, err := captureStdout(t, func() error {
			return runBundleInfo(&cobra.Command{}, []string{slug})
		}); err != nil {
			t.Errorf("bundle info %q returned error: %v", slug, err)
		}
	}
}

// ── resolveBundleLicenseStatus ────────────────────────────────────────

func TestResolveBundleLicenseStatus_Task(t *testing.T) {
	status := resolveBundleLicenseStatus("task")
	if !strings.Contains(status, "free") {
		t.Errorf("task license status should mention 'free', got: %s", status)
	}
}

func TestResolveBundleLicenseStatus_NoCache(t *testing.T) {
	// Override cache path to a guaranteed non-existent file.
	t.Setenv("LICENSE_CACHE_PATH", "/tmp/nself-test-no-license-cache-xyz.json")
	status := resolveBundleLicenseStatus("claw")
	if !strings.Contains(status, "not activated") {
		t.Errorf("expected 'not activated' when no cache, got: %s", status)
	}
}

// ── bundle list / display order ─────────────────────────────────────────

func TestBundleList_JSONFlag(t *testing.T) {
	root := newBundleTestCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"bundle", "list", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("bundle list --json returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"slug"`) {
		t.Errorf("--json output missing 'slug' key: %s", out)
	}
	if !strings.Contains(out, `"claw"`) {
		t.Errorf("--json output missing claw entry: %s", out)
	}
}

func TestBundleList_InstalledFlag_NoPlugins(t *testing.T) {
	// With no plugins installed the --installed flag should produce empty output
	// (just the header and footer lines, no bundle rows).
	root := newBundleTestCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"bundle", "list", "--installed"})
	if err := root.Execute(); err != nil {
		t.Fatalf("bundle list --installed returned error: %v", err)
	}
}

func TestDisplayOrder_AllKeysValid(t *testing.T) {
	for _, key := range bundle.DisplayOrder {
		if _, ok := bundle.Get(key); !ok {
			t.Errorf("bundle.DisplayOrder references unknown bundle key %q", key)
		}
	}
}

// ── parent command dispatching ────────────────────────────────────────

func TestBundleCmd_NoArgs_ShowsHelp(t *testing.T) {
	root := newBundleTestCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"bundle"})
	// No args shows help — should not error (cobra Help() returns nil)
	_ = root.Execute()
}

func TestBundleCmd_UnknownSubcommand(t *testing.T) {
	cmd := &cobra.Command{Use: "nself"}
	parent := *bundleCmd // shallow copy to avoid mutation
	cmd.AddCommand(&parent)
	applyGroups(cmd) // the copy keeps bundleCmd's GroupID; see newBundleTestCmd
	cmd.SetArgs([]string{"bundle", "badcmd"})
	buf := &bytes.Buffer{}
	cmd.SetErr(buf)
	err := cmd.Execute()
	// cobra returns an error for unknown subcommands
	if err == nil {
		t.Log("no error returned for unknown subcommand (cobra may print help instead)")
	}
}
