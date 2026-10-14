package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

// infoFixture serves notify as a free/licensed tier pair, points the plugin
// directory at a temp dir and, when installedTier is not empty, installs
// notify there as that tier. It returns nothing; the env carries the setup.
func infoFixture(t *testing.T, installedTier string) {
	t.Helper()
	infoFixtureRegistry(t, installedTier, `{"plugins":[`+
		`{"name":"notify","tier":"free","tier_pair":true,"version":"1.0.0","description":"free notify"},`+
		`{"name":"notify","tier":"pro","tier_pair":true,"version":"1.1.0","description":"licensed notify","bundles":["claw"]}]}`)
}

// infoFixtureRegistry is infoFixture with the registry body chosen by the test.
func infoFixtureRegistry(t *testing.T, installedTier, registry string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(registry))
	}))
	t.Cleanup(srv.Close)
	pluginDir := t.TempDir()
	if installedTier != "" {
		dir := filepath.Join(pluginDir, "notify")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := `{"name":"notify","version":"1.0.0","description":"d","tier":"` + installedTier + `"}`
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("NSELF_PLUGIN_CACHE", t.TempDir())
	t.Setenv("NSELF_PLUGIN_DIR", pluginDir)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	t.Setenv("HOME", t.TempDir())
}

func runInfo(t *testing.T, asJSON bool) string {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("open", false, "")
	cmd.Flags().Bool("json", asJSON, "")
	return captureVocabularyOutput(t, func() {
		if err := runPluginInfo(cmd, []string{"notify"}); err != nil {
			t.Error(err)
		}
	})
}

// TestPluginInfoJSONTier: `plugin info --json` reports tier and tier_reason;
// an installed plugin reports the tier it is installed as.
func TestPluginInfoJSONTier(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		infoFixture(t, "free")
		if !compat.V15() {
			if out := runInfo(t, true); json.Valid([]byte(out)) {
				t.Fatalf("v1.4 ignores --json (CR S2), got JSON: %s", out)
			}
			return
		}
		var got struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			Tier       string `json:"tier"`
			TierReason string `json:"tier_reason"`
		}
		if err := json.Unmarshal([]byte(runInfo(t, true)), &got); err != nil {
			t.Fatalf("not JSON: %v", err)
		}
		if got.Name != "notify" || got.Tier != "free" || got.TierReason != "installed" {
			t.Fatalf("installed free: %+v", got)
		}
		if compat.V15() && got.Version != "1.0.0" {
			t.Fatalf("v1.5 describes the installed tier's entry, got version %s", got.Version)
		}

		infoFixture(t, "pro")
		if err := json.Unmarshal([]byte(runInfo(t, true)), &got); err != nil {
			t.Fatal(err)
		}
		if got.Tier != "licensed" || got.TierReason != "installed" {
			t.Fatalf("installed pro: %+v", got)
		}
	})
}

// TestPluginInfoNotInstalled: with nothing installed, v1.5 reports what an
// install would pick (free here: no licence) and why; v1.4 ignores --json.
func TestPluginInfoNotInstalled(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		infoFixture(t, "")
		if !compat.V15() {
			if out := runInfo(t, true); json.Valid([]byte(out)) {
				t.Fatalf("v1.4 ignores --json (CR S2), got JSON: %s", out)
			}
			return
		}
		var got struct {
			Tier       string `json:"tier"`
			TierReason string `json:"tier_reason"`
		}
		if err := json.Unmarshal([]byte(runInfo(t, true)), &got); err != nil {
			t.Fatal(err)
		}
		if got.Tier != "free" || got.TierReason != "default-free" {
			t.Fatalf("not installed: %+v, want free/default-free", got)
		}
	})
}

// TestPluginInfoTableTier: the table shows the licence word and the reason in
// v1.5 and the registry word, unchanged, in v1.4.
func TestPluginInfoTableTier(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		infoFixture(t, "pro")
		out := runInfo(t, false)
		if compat.V15() {
			if !strings.Contains(out, "Tier reason") || !strings.Contains(out, "installed") || !strings.Contains(out, "licensed") {
				t.Fatalf("v1.5 table: %s", out)
			}
		} else if strings.Contains(out, "Tier reason") {
			t.Fatalf("v1.4 table must not change: %s", out)
		}
	})
}

// TestPluginUpdateTierFlag: `plugin update` takes --tier, rejects a bad value
// and a missing plugin name before touching anything.
func TestPluginUpdateTierFlag(t *testing.T) {
	if pluginUpdateCmd.Flags().Lookup("tier") == nil {
		t.Fatal("plugin update must have a --tier flag")
	}
	run := func(tier string, args ...string) error {
		cmd := &cobra.Command{}
		cmd.Flags().String("tier", tier, "")
		return runPluginUpdateTier(cmd, args)
	}
	if err := run("gold", "notify"); err == nil || !strings.Contains(err.Error(), "invalid --tier") {
		t.Fatalf("bad tier: %v", err)
	}
	if err := run("licensed"); err == nil || !strings.Contains(err.Error(), "needs a plugin name") {
		t.Fatalf("no name: %v", err)
	}
}

// TestPluginInfoRootChild is the helper process of runInfoRoot: it runs the
// command the way main does (commands.Execute: argv rewrite, tree
// preparation, invocation decorator) in a process of its own, because Execute
// relocates the shared command tree and would leave the other tests with it.
func TestPluginInfoRootChild(t *testing.T) {
	if os.Getenv("NSELF_TEST_INFO_CHILD") != "1" {
		t.Skip("helper process for runInfoRoot")
	}
	os.Args = append([]string{"nself", "plugin", "info", "notify"}, strings.Fields(os.Getenv("NSELF_TEST_INFO_ARGS"))...)
	if err := Execute(); err != nil {
		// What cmd/nself/report.go does in v1.5 (report_legacy.go has the same first check):
		// a silent error was already reported, only its exit status is returned; otherwise
		// the error envelope on stdout in JSON mode, the redacting human rendering on
		// stderr, the cause's exit status.
		var silencer errs.Silencer
		if errors.As(err, &silencer) && silencer.Silent() {
			os.Exit(errs.ExitCodeFor(err))
		}
		if command, jsonMode, known := output.Invocation(); known && jsonMode {
			_ = output.EmitError(output.Default(), command, err)
		}
		output.RenderError(os.Stderr, err)
		os.Exit(errs.ExitCodeFor(err))
	}
	os.Exit(0)
}

// runInfoRoot runs `nself plugin info notify <extra...>` through the root
// command in a child process (see TestPluginInfoRootChild), so the pre-run
// gates, the E402 --json check against the registry row among them, run as a
// user hits them. It returns the child's stdout and, on a non-zero exit, an
// error carrying its stderr.
func runInfoRoot(t *testing.T, extra ...string) (string, error) {
	t.Helper()
	child := exec.Command(os.Args[0], "-test.run=^TestPluginInfoRootChild$")
	child.Dir = t.TempDir()
	child.Env = append(os.Environ(), "NSELF_TEST_INFO_CHILD=1", "NSELF_TEST_INFO_ARGS="+strings.Join(extra, " "), "NSELF_ALLOW_SOURCE_DIR=1")
	var stdout, stderr bytes.Buffer
	child.Stdout, child.Stderr = &stdout, &stderr
	if err := child.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// infoData unwraps the v1.5 envelope of `plugin info --json` (command
// `config plugins info`) and returns its data member (P7-SURF-13: the document
// that was bare JSON is now the data of the v1 envelope; its members are
// unchanged).
func infoData(t *testing.T, out string) []byte {
	t.Helper()
	var env struct {
		Command string          `json:"command"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not an envelope: %v\n%s", err, out)
	}
	if env.Command != "config plugins info" || len(env.Data) == 0 {
		t.Fatalf("envelope command %q data %q, want config plugins info with data\n%s", env.Command, env.Data, out)
	}
	return env.Data
}

// infoFailure decodes the v1.5 failure of `plugin info --json`: one error
// envelope (no data member, so MCP and HTTP callers see a failure), whose message
// carries the reason and the cause as "plugin <n>: tier unresolved (tier_reason
// <reason>): <cause>". It returns the reason and the cause.
func infoFailure(t *testing.T, out string) (reason, cause string) {
	t.Helper()
	var env map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not one JSON document: %v\n%s", err, out)
	}
	if _, has := env["data"]; has {
		t.Fatalf("a failed lookup must not carry a success data member: %s", out)
	}
	var e struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(env["error"], &e); err != nil || e.Message == "" {
		t.Fatalf("no error member: %v\n%s", err, out)
	}
	const mid = "): "
	head, tail, ok := strings.Cut(e.Message, "(tier_reason ")
	if !ok || !strings.Contains(head, "tier unresolved") {
		t.Fatalf("message %q lacks the tier_reason marker", e.Message)
	}
	reason, cause, ok = strings.Cut(tail, mid)
	if !ok || cause == "" {
		t.Fatalf("message %q lacks the cause", e.Message)
	}
	return reason, cause
}

// TestPluginInfoJSONThroughRoot (CR M1, S2): `nself plugin info <n> --json`
// works through the root command. v1.5 prints the JSON (the registry row is
// json legacy, so the E402 gate lets it through); v1.4 still ignores the flag
// and prints the table.
func TestPluginInfoJSONThroughRoot(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		infoFixture(t, "free")
		out, err := runInfoRoot(t, "--json")
		if err != nil {
			t.Fatalf("plugin info --json through the root command: %v", err)
		}
		if !compat.V15() {
			if json.Valid([]byte(out)) || !strings.Contains(out, "Field") {
				t.Fatalf("v1.4 must ignore --json and print the table, got %q", out)
			}
			return
		}
		var got struct {
			Name       string `json:"name"`
			Tier       string `json:"tier"`
			TierReason string `json:"tier_reason"`
		}
		if jerr := json.Unmarshal(infoData(t, out), &got); jerr != nil {
			t.Fatalf("v1.5 output is not JSON: %v\n%s", jerr, out)
		}
		if got.Name != "notify" || got.Tier != "free" || got.TierReason != "installed" {
			t.Fatalf("v1.5 JSON: %+v", got)
		}
	})
}

// TestPluginInfoUnresolvedTier (CR S3): a slug that resolves to no single tier
// (two entries, not a declared tier pair) shows tier_reason "unresolved" and
// the error in v1.5, never the first registry entry.
func TestPluginInfoUnresolvedTier(t *testing.T) {
	compattest.Set(t, true)
	infoFixtureRegistry(t, "", `{"plugins":[`+
		`{"name":"notify","tier":"free","version":"1.0.0","description":"first entry"},`+
		`{"name":"notify","tier":"pro","version":"2.0.0","description":"second entry"}]}`)
	out, err := runInfoRoot(t, "--json")
	if err == nil {
		t.Fatalf("an unresolved tier must fail, got output %s", out)
	}
	if reason, cause := infoFailure(t, out); reason != "unresolved" || cause == "" || strings.Contains(out, "first entry") {
		t.Fatalf("unresolved lookup must carry reason+cause and no registry entry: %q %q\n%s", reason, cause, out)
	}
	table, terr := runInfoRoot(t)
	if terr == nil || !strings.Contains(terr.Error(), "tier unresolved") || strings.Contains(table, "first entry") {
		t.Fatalf("table mode: err=%v out=%q", terr, table)
	}
}

// TestPluginInfoLegacyEnvFailureIsOneBareDocument: with NSELF_JSON_LEGACY=1 the
// failed lookup keeps the pre-contract shape, one bare document with tier_reason
// and tier_error, and no second (error envelope) document; the exit stays
// non-zero and the stderr line is redacted.
func TestPluginInfoLegacyEnvFailureIsOneBareDocument(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv(output.LegacyEnvVar, "1")
	infoFixtureRegistry(t, "", `{"plugins":[`+
		`{"name":"notify","tier":"free","version":"1.0.0","description":"first entry"},`+
		`{"name":"notify","tier":"pro","version":"2.0.0","description":"second entry"}]}`)
	out, err := runInfoRoot(t, "--json")
	if err == nil {
		t.Fatalf("an unresolved tier must still fail, got output %s", out)
	}
	if !strings.Contains(err.Error(), "exit status") || !strings.Contains(err.Error(), "tier unresolved") {
		t.Fatalf("want a non-zero exit and the reported failure on stderr, got %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(out))
	var doc map[string]json.RawMessage
	if jerr := dec.Decode(&doc); jerr != nil {
		t.Fatalf("not a JSON document: %v\n%s", jerr, out)
	}
	var more json.RawMessage
	if dec.Decode(&more) != io.EOF {
		t.Fatalf("a second document follows the bare one:\n%s", out)
	}
	for _, key := range []string{"name", "tier", "tier_reason", "tier_error"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("bare document lacks %q: %s", key, out)
		}
	}
	for _, key := range []string{"schema_version", "command", "data", "error"} {
		if _, ok := doc[key]; ok {
			t.Errorf("bare document carries the envelope member %q: %s", key, out)
		}
	}
	var reason, cause string
	_ = json.Unmarshal(doc["tier_reason"], &reason)
	_ = json.Unmarshal(doc["tier_error"], &cause)
	if reason != "unresolved" || cause == "" {
		t.Fatalf("tier_reason %q tier_error %q", reason, cause)
	}
}

// The tier_error text goes through the public redaction seam.
func TestRedactedMessageRemovesCredentials(t *testing.T) {
	err := errors.New("license lookup failed: https://user" + ":" + "synthetic@license.example.test/check?token=abc123")
	got := redactedMessage(err)
	if strings.Contains(got, "synthetic") || strings.Contains(got, "abc123") || !strings.Contains(got, "license lookup failed") {
		t.Fatalf("redactedMessage = %q", got)
	}
	if plain := redactedMessage(errors.New("registry serves no licensed entry")); plain != "registry serves no licensed entry" {
		t.Fatalf("a plain message changed: %q", plain)
	}
}

// TestPluginInfoInstalledTierNotServed (CR S3): a plugin installed as licensed
// whose registry now serves only the free entry is unresolved in v1.5, not
// described by that other entry.
func TestPluginInfoInstalledTierNotServed(t *testing.T) {
	compattest.Set(t, true)
	infoFixtureRegistry(t, "pro", `{"plugins":[{"name":"notify","tier":"free","version":"1.0.0","description":"free only"}]}`)
	out, err := runInfoRoot(t, "--json")
	if err == nil {
		t.Fatalf("want an error, got output %s", out)
	}
	if reason, cause := infoFailure(t, out); reason != "unresolved" || !strings.Contains(cause, "registry serves no") || strings.Contains(out, "free only") {
		t.Fatalf("installed tier not served must be unresolved with the cause: %q %q\n%s", reason, cause, out)
	}
}

// TestPluginInfoMaxTier (Opus M1): a "max" entry is a licensed entry: installed
// as max it resolves (reason installed), not installed it reports the licence
// word "licensed", never the raw "max". Reason for the latter is only-tier (S3):
// no entitlement is looked up.
func TestPluginInfoMaxTier(t *testing.T) {
	compattest.Set(t, true)
	const reg = `{"plugins":[{"name":"notify","tier":"max","version":"1.0.0","description":"max notify","bundles":["claw"]}]}`
	var got struct {
		Tier       string `json:"tier"`
		TierReason string `json:"tier_reason"`
		TierError  string `json:"tier_error"`
	}
	for _, tc := range []struct{ installed, reason string }{{"max", "installed"}, {"", "only-tier"}} {
		infoFixtureRegistry(t, tc.installed, reg)
		out, err := runInfoRoot(t, "--json")
		if err != nil {
			t.Fatalf("installed=%q: %v\n%s", tc.installed, err, out)
		}
		if jerr := json.Unmarshal(infoData(t, out), &got); jerr != nil {
			t.Fatal(jerr)
		}
		if got.Tier != "licensed" || got.TierReason != tc.reason {
			t.Fatalf("installed=%q: %+v, want licensed/%s", tc.installed, got, tc.reason)
		}
	}
}

// TestPluginInfoEntitlementLookupError (Opus N2): when the licence lookup
// fails, info reports the error in tier_error with reason unresolved, not a
// decided default-free (install still fails closed to free).
func TestPluginInfoEntitlementLookupError(t *testing.T) {
	compattest.Set(t, true)
	infoFixture(t, "")
	ping := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	t.Cleanup(ping.Close)
	t.Setenv("LICENSE_PING_URL", ping.URL)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "nself_pro_test_key")
	out, err := runInfoRoot(t, "--json")
	if err == nil {
		t.Fatalf("a failed licence lookup must be an error, got %s", out)
	}
	if reason, cause := infoFailure(t, out); reason != "unresolved" || cause == "" || strings.Contains(out, "default-free") {
		t.Fatalf("lookup error must be the cause, not default-free: %q %q\n%s", reason, cause, out)
	}
}

// TestUpdateWithTierEnv (CR r2 S1): the --tier override travels in
// NSELF_PLUGIN_INSTALL_TIER set by the cmd layer for the update only. With it
// a lapsed-licence update of an installed licensed plugin gets past tier
// resolution (without it: E131); the caller's value is put back afterwards.
func TestUpdateWithTierEnv(t *testing.T) {
	setup := func(t *testing.T) string {
		infoFixtureRegistry(t, "pro", `{"plugins":[`+
			`{"name":"notify","tier":"free","tier_pair":true,"version":"1.0.0","description":"free"},`+
			`{"name":"notify","tier":"pro","tier_pair":true,"version":"1.1.0","description":"licensed","bundles":["claw"]}]}`)
		t.Setenv("NSELF_PLUGIN_INSTALL_TIER", "")
		compattest.Set(t, true)
		return os.Getenv("NSELF_PLUGIN_DIR")
	}
	isE131 := func(err error) bool { return err != nil && strings.Contains(err.Error(), "E131") }

	t.Run("without --tier the installed tier sticks (E131)", func(t *testing.T) {
		dir := setup(t)
		if err := updateWithTier(context.Background(), &config.Config{}, "notify", dir, ""); !isE131(err) {
			t.Fatalf("want E131, got %v", err)
		}
	})
	t.Run("with --tier free the switch goes through", func(t *testing.T) {
		dir := setup(t)
		if err := updateWithTier(context.Background(), &config.Config{}, "notify", dir, "free"); isE131(err) {
			t.Fatalf("--tier free must not stop with E131: %v", err)
		}
	})
	t.Run("the caller's env is restored", func(t *testing.T) {
		dir := setup(t)
		t.Setenv("NSELF_PLUGIN_INSTALL_TIER", "pro")
		_ = updateWithTier(context.Background(), &config.Config{}, "notify", dir, "free")
		if v := os.Getenv("NSELF_PLUGIN_INSTALL_TIER"); v != "pro" {
			t.Fatalf("env not restored: %q", v)
		}
		_ = os.Unsetenv("NSELF_PLUGIN_INSTALL_TIER")
		_ = updateWithTier(context.Background(), &config.Config{}, "notify", dir, "free")
		if _, set := os.LookupEnv("NSELF_PLUGIN_INSTALL_TIER"); set {
			t.Fatal("env must be unset again after the call")
		}
	})
}
