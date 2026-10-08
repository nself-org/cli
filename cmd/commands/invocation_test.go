package commands

// Tests for the invocation decorator (invocation.go, registry.go).
//
// Fixture tests model the command classes on a small tree with a fixture
// registry; real-tree tests prove the live registry builds and that the
// guard decides correctly for the real `restart`, `config validate`,
// `status` and `secrets decrypt-on-deploy` commands (parse only: no command
// body ever runs).

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// fixture is a tree shaped like the real classes of command.
type fixture struct {
	root *cobra.Command
	ran  map[string]int
	pre  int
}

func str(s string) *string { return &s }

func newFixture(t *testing.T) *fixture {
	t.Helper()
	output.ResetState()
	t.Cleanup(output.ResetState)
	f := &fixture{ran: map[string]int{}}
	body := func(name string) func(*cobra.Command, []string) error {
		return func(*cobra.Command, []string) error { f.ran[name]++; return nil }
	}
	root := &cobra.Command{Use: "nself", SilenceUsage: true, SilenceErrors: true, RunE: body("root")}
	root.PersistentFlags().Bool("json", false, "json")
	root.PersistentFlags().Bool("no-monorepo", false, "x")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	restart := &cobra.Command{Use: "restart", RunE: body("restart")}
	stop := &cobra.Command{Use: "stop", RunE: body("stop")}
	status := &cobra.Command{Use: "status", RunE: body("status")}
	status.Flags().BoolP("json", "j", false, "own json")
	cfg := &cobra.Command{Use: "cfg", RunE: body("cfg")}
	cfg.PersistentFlags().Bool("json", false, "own persistent json")
	validate := &cobra.Command{Use: "validate", RunE: body("validate")}
	cfg.AddCommand(validate)
	doc := &cobra.Command{Use: "doc", RunE: body("doc")}
	doc.Flags().Bool("install-check", false, "x")
	doc.Flags().Bool("ai", false, "x")
	ran := &cobra.Command{Use: "runonly", Run: func(*cobra.Command, []string) { f.ran["runonly"]++ }}
	bun := &cobra.Command{Use: "bun", RunE: body("bun"),
		PersistentPreRunE: func(*cobra.Command, []string) error { f.pre++; return nil }}
	bunSub := &cobra.Command{Use: "sub", RunE: body("bunsub")}
	bun.AddCommand(bunSub)
	arg := &cobra.Command{Use: "arg <x>", Args: cobra.ExactArgs(1), RunE: body("arg")}
	arg.Flags().Int("api-key", 0, "x")
	arg.Flags().Int("port", 0, "x")
	root.AddCommand(restart, stop, status, cfg, doc, ran, bun, arg)

	entries := map[string]*cmdregistry.Command{
		"nself restart":      {JSON: canon.JSONNone},
		"nself stop":         {JSON: canon.JSONNone},
		"nself status":       {JSON: canon.JSONLegacy},
		"nself cfg":          {JSON: canon.JSONNone},
		"nself cfg validate": {JSON: canon.JSONNone},
		"nself doc": {JSON: canon.JSONNone, Flags: []cmdregistry.Flag{
			{Name: "install-check", JSON: str(canon.JSONLegacy)},
			{Name: "ai", JSON: str(canon.JSONNone)},
		}},
		"nself runonly": {JSON: canon.JSONNone},
		"nself bun":     {JSON: canon.JSONNone},
		"nself bun sub": {JSON: canon.JSONNone},
		"nself arg":     {JSON: canon.JSONEnvelope},
	}
	old := jsonEntryFor
	jsonEntryFor = func(c *cobra.Command) (*cmdregistry.Command, error) {
		if e, ok := entries[c.CommandPath()]; ok {
			return e, nil
		}
		return nil, errors.New("no entry for " + c.CommandPath())
	}
	t.Cleanup(func() { jsonEntryFor = old })
	installInvocationDecorator(root)
	f.root = root
	return f
}

// newFixtureRun runs args on a fresh fixture and checks bun's hook still runs
// when --json is absent (the decorator wraps the hook, never replaces it).
func newFixtureRun(t *testing.T, args ...string) error {
	f := newFixture(t)
	err := f.run(args...)
	if err == nil && f.pre == 0 {
		t.Fatal("bun's own PersistentPreRunE must still run without --json")
	}
	return err
}

func (f *fixture) run(args ...string) error {
	f.root.SetArgs(args)
	return f.root.Execute()
}

func wantE402(t *testing.T, err error) {
	t.Helper()
	if d := errs.Describe(err); err == nil || d.Code != "E402" {
		t.Fatalf("want E402, got %v", err)
	}
}

func TestInvocationRootFlagNoneCommandIsE402InBothModes(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		f := newFixture(t)
		wantE402(t, f.run("restart", "--json"))
		if f.ran["restart"] != 0 {
			t.Fatal("restart body ran: the guard must return before the original RunE")
		}
		if cmd, on, known := output.Invocation(); !known || !on || cmd != "restart" {
			t.Fatalf("invocation = %q %v %v", cmd, on, known)
		}
	})
}

func TestInvocationJSONFalseIsNotRefused(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		f := newFixture(t)
		if err := f.run("stop", "--json=false"); err != nil {
			t.Fatal(err)
		}
		if f.ran["stop"] != 1 {
			t.Fatal("stop did not run")
		}
		if _, on, _ := output.Invocation(); on {
			t.Fatal("--json=false recorded as JSON mode")
		}
	})
}

func TestInvocationRootCommandJSONIsE402InBothModes(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		f := newFixture(t)
		wantE402(t, f.run("--json"))
		if f.ran["root"] != 0 {
			t.Fatal("root body ran")
		}
		// cobra keeps flag values across Execute calls: reset explicitly.
		if err := f.run("--json=false"); err != nil || f.ran["root"] != 1 {
			t.Fatalf("plain root must still run: %v", err)
		}
	})
}

func TestInvocationAncestorOwnJSONFlagIsGatedByMode(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		f := newFixture(t)
		err := f.run("cfg", "validate", "--json")
		if compat.V15() {
			wantE402(t, err)
			if f.ran["validate"] != 0 {
				t.Fatal("validate ran in v1.5")
			}
			return
		}
		if err != nil || f.ran["validate"] != 1 {
			t.Fatalf("v1.4 must accept and ignore: err=%v ran=%d", err, f.ran["validate"])
		}
	})
}

func TestInvocationLegacyCommandPassesThrough(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		f := newFixture(t)
		for _, a := range []string{"--json", "-j"} {
			if err := f.run("status", a); err != nil {
				t.Fatalf("status %s: %v", a, err)
			}
		}
		if f.ran["status"] != 2 {
			t.Fatalf("status ran %d times", f.ran["status"])
		}
	})
}

func TestInvocationFlagOverrides(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		// Fresh tree per case: cobra keeps flag values across Execute calls.
		if err := newFixture(t).run("doc", "--install-check", "--json"); err != nil {
			t.Fatalf("legacy override must pass: %v", err)
		}
		wantE402(t, newFixture(t).run("doc", "--ai", "--json"))
		wantE402(t, newFixture(t).run("doc", "--ai", "--install-check", "--json")) // none beats legacy
	})
}

func TestInvocationRunOnlyAndBundleSubcommandAreGuarded(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		f := newFixture(t)
		if err := f.run("runonly"); err != nil || f.ran["runonly"] != 1 {
			t.Fatalf("Run-only command must still run: %v", err)
		}
		wantE402(t, f.run("runonly", "--json"))
		wantE402(t, f.run("bun", "sub", "--json"))
		if f.pre != 0 {
			t.Fatal("bun's own PersistentPreRunE ran before the E402 refusal: a refused command must do no work")
		}
		if f.ran["bunsub"] != 0 {
			t.Fatal("bun sub body ran")
		}
		if err := newFixtureRun(t, "bun", "sub"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestInvocationEnvelopeCommandRunsInJSONMode(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		f := newFixture(t)
		if err := f.run("arg", "x", "--json"); err != nil || f.ran["arg"] != 1 {
			t.Fatalf("envelope command must run: %v", err)
		}
	})
}

func TestInvocationUsageErrorsCodedOnlyInV15(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		f := newFixture(t)
		for _, args := range [][]string{{"status", "--bogus"}, {"arg"}} {
			err := f.run(args...)
			if err == nil {
				t.Fatalf("%v: want an error", args)
			}
			code := errs.Describe(err).Code
			if compat.V15() {
				if code != "E401" {
					t.Fatalf("%v: code %s, want E401", args, code)
				}
				continue
			}
			var ce *errs.CLIError
			if errors.As(err, &ce) || code != "E400" {
				t.Fatalf("%v: v1.4 must return cobra's error untouched, got %T code %s", args, err, code)
			}
		}
		if err := f.run("status", "--bogus"); !compat.V15() && err.Error() != "unknown flag: --bogus" {
			t.Fatalf("v1.4 text changed: %q", err.Error())
		}
	})
}

func TestInvocationInstallDecoratorIsIdempotent(t *testing.T) {
	f := newFixture(t)
	installInvocationDecorator(f.root)
	installInvocationDecorator(f.root)
	if err := f.run("stop"); err != nil || f.ran["stop"] != 1 {
		t.Fatalf("stop ran %d times after repeated install (err %v)", f.ran["stop"], err)
	}
}

func TestInvocationDecoratorDoesNotLoadCanonWithoutJSON(t *testing.T) {
	f := newFixture(t)
	n := 0
	old := canonLoad
	canonLoad = func(b bool) (*canon.File, error) { n++; return old(b) }
	t.Cleanup(func() { canonLoad = old })
	for _, a := range [][]string{{"stop"}, {"status"}, {"stop", "--json=false"}} {
		if err := f.run(a...); err != nil {
			t.Fatal(err)
		}
	}
	if n != 0 {
		t.Fatalf("canon loaded %d times on runs without --json", n)
	}
}

// --- real tree ---------------------------------------------------------

// reattachRealTree points every top-level command back at RootCmd. The error
// harness test (error_harness_test.go) re-adds all commands to a throwaway
// root, which moves their parent pointer and leaves them detached from
// RootCmd's persistent flags for the rest of the run.
func reattachRealTree() {
	for _, c := range append([]*cobra.Command(nil), RootCmd.Commands()...) {
		if c.Parent() != RootCmd {
			RootCmd.RemoveCommand(c) // also clears the parent pointer
			RootCmd.AddCommand(c)
		}
	}
}

// parseReal resolves a real command and parses --json (no body runs); the
// flag state is restored afterwards.
func parseReal(t *testing.T, path ...string) *cobra.Command {
	t.Helper()
	reattachRealTree()
	cmd, _, err := RootCmd.Find(path)
	if err != nil || cmd == nil || cmd == RootCmd {
		t.Fatalf("real command %v not found: %v", path, err)
	}
	if err := cmd.ParseFlags([]string{"--json"}); err != nil {
		t.Fatalf("%s: %v", cmd.CommandPath(), err)
	}
	t.Cleanup(func() {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Name == "json" {
				_ = f.Value.Set("false")
				f.Changed = false
			}
		})
	})
	return cmd
}

func TestJSONFlagEveryRealCommandHasBoolOrNoLocalJSON(t *testing.T) {
	reattachRealTree()
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, set := range []*pflag.FlagSet{c.LocalFlags(), c.PersistentFlags()} {
			if f := set.Lookup("json"); f != nil && f.Value.Type() != "bool" {
				t.Errorf("%s: json flag is %s, want bool", c.CommandPath(), f.Value.Type())
			}
		}
		for _, ch := range c.Commands() {
			walk(ch)
		}
	}
	walk(RootCmd)
	if f := RootCmd.PersistentFlags().Lookup("json"); f == nil || f.Shorthand != "" {
		t.Fatalf("root --json must exist with no shorthand: %+v", f)
	}
	if st, _, _ := RootCmd.Find([]string{"status"}); st.Flags().ShorthandLookup("j") == nil {
		t.Fatal("status -j shorthand lost")
	}
}

func TestInvocationRealRegistryBuildsAndGuardDecides(t *testing.T) {
	reattachRealTree()
	resetRegistryCache()
	t.Cleanup(resetRegistryCache)
	if _, err := commandRegistry(); err != nil {
		t.Fatalf("live registry must build: %v", err)
	}
	compattest.Both(t, func(t *testing.T) {
		// The mode's dispatch resolves spellings on the prepared tree after
		// the canon rewrite, so the subtest does the same.
		defer prepareTreeWith(&canonTable, RootCmd, compat.V15(), false)()
		// restart: root --json flag, none: E402 in both modes.
		wantE402(t, refuseUnsupportedJSON(parseReal(t, "restart")))
		// D-0216: decrypt-on-deploy prints plaintext secrets, has no json
		// support, so JSON mode must refuse it in both modes. It moves under
		// config in v1.5, so the spelling is the mode's rewrite of the old one.
		dec, _, decErr := rewriteCanonArgsWith(&canonTable, RootCmd, []string{"secrets", "decrypt-on-deploy"}, compat.V15())
		if decErr != nil {
			t.Fatalf("rewrite decrypt-on-deploy: %v", decErr)
		}
		wantE402(t, refuseUnsupportedJSON(parseReal(t, dec...)))
		// status: own legacy json flag, passes.
		if err := refuseUnsupportedJSON(parseReal(t, "status")); err != nil {
			t.Fatalf("status --json refused: %v", err)
		}
		// config validate: ancestor's own json flag; mode-gated.
		err := refuseUnsupportedJSON(parseReal(t, "config", "validate"))
		if compat.V15() {
			wantE402(t, err)
		} else if err != nil {
			t.Fatalf("v1.4 config validate --json must pass through: %v", err)
		}
	})
}

func BenchmarkInstallDecorator(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		root := &cobra.Command{Use: "nself", RunE: func(*cobra.Command, []string) error { return nil }}
		for j := 0; j < 400; j++ {
			root.AddCommand(&cobra.Command{Use: "c" + string(rune('a'+j%26)) + string(rune('a'+j/26)),
				RunE: func(*cobra.Command, []string) error { return nil }})
		}
		b.StartTimer()
		installInvocationDecorator(root)
	}
}

// E401 must not echo what the user typed: an unknown shorthand cluster and the
// value of an invalid secret-named flag are removed (REG-05 review S4).
func TestInvocationE401DoesNotEchoArgv(t *testing.T) {
	compattest.Set(t, true)
	for _, c := range []struct {
		name string
		args []string
		leak string
	}{
		{"shorthand tail", []string{"stop", "-xhunter2tail"}, "hunter2tail"},
		{"secret-named flag value", []string{"arg", "x", "--api-key=hunter2value"}, "hunter2value"},
	} {
		err := newFixture(t).run(c.args...)
		if err == nil || errs.Describe(err).Code != "E401" {
			t.Fatalf("%s: want E401, got %v", c.name, err)
		}
		d := errs.Describe(err)
		for _, text := range []string{err.Error(), d.Message, d.Cause, d.Remediation} {
			if strings.Contains(text, c.leak) {
				t.Errorf("%s: E401 echoed the typed value", c.name)
			}
		}
		if errors.Unwrap(err) != nil {
			t.Errorf("%s: E401 must not keep the raw cobra error as its cause", c.name)
		}
	}
	// A non-secret flag keeps its (redacted) value so the message stays useful.
	err := newFixture(t).run("arg", "x", "--port=zzz")
	if err == nil || !strings.Contains(err.Error(), `"--port"`) {
		t.Fatalf("non-secret invalid value must still name the flag: %v", err)
	}
	// v1.4 keeps cobra's text byte-for-byte.
	compattest.Set(t, false)
	if err := newFixture(t).run("arg", "x", "--api-key=hunter2value"); err == nil || !strings.Contains(err.Error(), "hunter2value") {
		t.Fatalf("v1.4 must return cobra's error untouched: %v", err)
	}
}

// A refused real `bundle` command must not reach the network or write the
// cache: its own PersistentPreRunE fetches bundles.json (REG-05 review S1).
func TestInvocationRefusedBundleDoesNoWork(t *testing.T) {
	reattachRealTree()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("NSELF_BUNDLES_URL", srv.URL)
	resetRegistryCache()
	t.Cleanup(resetRegistryCache)
	decorate(bundleCmd) // what Execute does for the whole tree
	compattest.Both(t, func(t *testing.T) {
		hits = 0
		path := []string{"bundle", "install"}
		if compat.V15() {
			path = []string{"config", "bundles", "install"}
		}
		cmd := parseReal(t, path...)
		wantE402(t, bundleCmd.PersistentPreRunE(cmd, nil))
		if hits != 0 {
			t.Fatalf("refused bundle command made %d network request(s)", hits)
		}
		if n := countFiles(t, home); n != 0 {
			t.Fatalf("refused bundle command wrote %d file(s) under HOME", n)
		}
		// Control: without --json the same hook does reach the network.
		_ = cmd.Flags().Set("json", "false")
		_ = bundleCmd.PersistentPreRunE(cmd, nil)
		if hits == 0 {
			t.Fatal("control failed: the hook no longer fetches, so this test proves nothing")
		}
	})
}

func countFiles(t *testing.T, root string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// In JSON mode the monorepo notice goes to stderr so stdout stays clean; in
// text mode it stays on stdout (REG-05 review S3).
func TestMonorepoNoticeStream(t *testing.T) {
	for _, jsonOn := range []bool{true, false} {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".backend"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".backend", ".env"), []byte("PROJECT_NAME=x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Chdir(root)
		cmd := &cobra.Command{Use: "status"}
		cmd.Flags().Bool("json", jsonOn, "")
		var pre func(*cobra.Command, []string) error = RootCmd.PersistentPreRunE
		stdout, stderr := captureStreams(t, func() {
			if err := pre(cmd, nil); err != nil {
				t.Errorf("pre-run: %v", err)
			}
		})
		const notice = "Detected monorepo layout"
		if jsonOn && (strings.Contains(stdout, notice) || !strings.Contains(stderr, notice)) {
			t.Errorf("json mode: notice must be on stderr only (stdout %q)", stdout)
		}
		if !jsonOn && (!strings.Contains(stdout, notice) || strings.Contains(stderr, notice)) {
			t.Errorf("text mode: notice must stay on stdout (stderr %q)", stderr)
		}
	}
}

// captureStreams runs fn with os.Stdout and os.Stderr redirected to pipes.
func captureStreams(t *testing.T, fn func()) (string, string) {
	t.Helper()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	fn()
	os.Stdout, os.Stderr = oldOut, oldErr
	_ = outW.Close()
	_ = errW.Close()
	o, _ := io.ReadAll(outR)
	e, _ := io.ReadAll(errR)
	return string(o), string(e)
}
