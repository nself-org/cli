package commands

// Purpose: tests for the v1.5 break-out relocation (relocate.go, P7-CANON-13) and
// the breakouts canon fragment. Fixture registry, generated ed25519 key swapped
// into signature.Keys (restored by t.Cleanup), throwaway HOME and project; no
// network, no production key.

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/spf13/cobra"
)

// stubScript records the argv and the two contract:cli.plugin-exec-env v1
// variables in $ENV_PROOF.
const stubScript = "#!/bin/sh\n{ echo \"argv=$*\"; env | grep -E '^NSELF_(V15|CLI_VERSION)='; } > \"$ENV_PROOF\"\n"

type seamCall struct{ slug, version string }

// relocateSetup selects v1.5, a project directory (with a lock pinning the ci
// plugin at lockVersion when that is not empty), a proof file for the stub, a
// recording seam over fn and a buffer for the warning.
func relocateSetup(t *testing.T, lockVersion string, fn func(slug, version string) (string, error)) (proof string, calls *[]seamCall, warn *bytes.Buffer) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the plugin stub is a /bin/sh script")
	}
	compattest.Set(t, true)
	proof = filepath.Join(t.TempDir(), "proof")
	t.Setenv("ENV_PROOF", proof)
	t.Setenv(envNoAutoAdd, "")
	dir := t.TempDir()
	if lockVersion != "" {
		l := lock.New()
		l.Plugins["ci"] = lock.PluginPin{Version: lockVersion, Tier: lock.TierFree}
		data, err := l.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(lock.Path(dir), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	calls, warn = &[]seamCall{}, &bytes.Buffer{}
	prevSeam, prevOut := autoAddPlugin, relocateWarnOut
	autoAddPlugin = func(slug, version string) (string, error) {
		*calls = append(*calls, seamCall{slug, version})
		return fn(slug, version)
	}
	relocateWarnOut = warn
	t.Cleanup(func() {
		autoAddPlugin, relocateWarnOut = prevSeam, prevOut
		_ = os.Chdir(wd)
	})
	return proof, calls, warn
}

// fixtureSeam serves a signed one-entry registry whose nself-ci is stubScript
// and returns the real seam over it.
func fixtureSeam(t *testing.T) func(string, string) (string, error) {
	t.Helper()
	tb := aiTarball(t, map[string]string{"nself-ci": stubScript})
	sum := aiSum(tb)
	aiServe(t, tb, sum, aiSigned(t, sum))
	return autoInstallPlugin
}

// publishStub is a seam that publishes stubScript as nself-<slug> without a registry.
func publishStub(t *testing.T) func(string, string) (string, error) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return func(slug, _ string) (string, error) {
		p := plugin.PublishedBinaryPath("nself-" + slug)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", err
		}
		return p, os.WriteFile(p, []byte(stubScript), 0o755)
	}
}

func readProof(t *testing.T, proof string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(proof)
	if err != nil {
		t.Fatalf("the plugin did not run: %v", err)
	}
	got := map[string]string{}
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		k, v, _ := strings.Cut(l, "=")
		got[k] = v
	}
	return got
}

func TestRelocateBreakout(t *testing.T) {
	for _, tc := range []struct {
		argv     []string
		slug     string
		wantArgv string
	}{
		{[]string{"ci", "--help"}, "ci", "--help"},
		{[]string{"ci", "build", "--upload"}, "ci", "build --upload"},
		{[]string{"runner", "verify", "x"}, "ci", "nodes verify x"},
		{[]string{"server", "list"}, "infra", "server list"},
		{[]string{"functions", "list"}, "functions", "list"},
		{[]string{"plugin", "init", "demo"}, "plugin-dev", "init demo"},
		{[]string{"plugin", "new", "demo"}, "plugin-dev", "new demo"},
		{[]string{"plugin", "test"}, "plugin-dev", "test"},
		{[]string{"server", "list", "--json", "--no-deprecation-warnings"}, "infra", "server list --json"},
		{[]string{"infra", "server", "--help"}, "infra", "server --help"},
		{[]string{"infra", "plan"}, "infra", "plan"},
		{[]string{"plugin-dev", "init", "--help"}, "plugin-dev", "init --help"},
	} {
		t.Run(strings.Join(tc.argv, "_"), func(t *testing.T) {
			proof, calls, warn := relocateSetup(t, "", nil)
			autoAddPlugin = recordOver(calls, publishStub(t))
			handled, err := relocateBreakout(tc.argv[0], tc.argv[1:])
			if !handled || err != nil {
				t.Fatalf("handled=%v err=%v", handled, err)
			}
			if !reflect.DeepEqual(*calls, []seamCall{{tc.slug, ""}}) {
				t.Fatalf("seam calls %v", *calls)
			}
			if got := readProof(t, proof)["argv"]; got != tc.wantArgv {
				t.Fatalf("plugin argv %q, want %q", got, tc.wantArgv)
			}
			if n := strings.Count(warn.String(), "warning:"); n != 1 {
				t.Fatalf("%d warnings: %q", n, warn.String())
			}
		})
	}
	t.Run("root flags before the command", func(t *testing.T) {
		proof, calls, _ := relocateSetup(t, "", nil)
		autoAddPlugin = recordOver(calls, publishStub(t))
		handled, err := relocateInvocation([]string{"--json", "--no-deprecation-warnings", "runner", "verify", "x", "--", "--raw"})
		if !handled || err != nil {
			t.Fatalf("handled=%v err=%v", handled, err)
		}
		if got := readProof(t, proof)["argv"]; got != "nodes verify x --json -- --raw" {
			t.Fatalf("plugin argv %q", got)
		}
		if handled, _ := relocateInvocation([]string{"--json"}); handled {
			t.Fatal("a bare flag is not a break-out")
		}
		if handled, _ := relocateInvocation([]string{"--", "ci"}); handled {
			t.Fatal("an argv the engine cannot place is left to cobra")
		}
	})
	t.Run("not a break-out", func(t *testing.T) {
		for _, argv := range [][]string{{"start"}, {"plugin", "install", "x"}, {"plugin"}, {"runnerx"}, {"--help"}, {"help", "ci"}} {
			if _, _, ok := breakoutRow(argv[0], argv[1:]); ok {
				t.Errorf("%v is not a break-out", argv)
			}
		}
	})
	t.Run("mounted plugin runs through cobra", func(t *testing.T) {
		_, calls, _ := relocateSetup(t, "", nil)
		node := walkNames(RootCmd, []string{"ci"})
		if node == nil { // an earlier test left the canon applied: no core ci in the tree
			node = &cobra.Command{Use: "ci"}
			RootCmd.AddCommand(node)
			t.Cleanup(func() { RootCmd.RemoveCommand(node) })
		}
		prev := node.Annotations
		node.Annotations = map[string]string{"nself.plugin": "ci"}
		t.Cleanup(func() { node.Annotations = prev })
		if handled, err := relocateBreakout("ci", []string{"--help"}); handled || err != nil || len(*calls) != 0 {
			t.Fatalf("a mounted plugin must be left to cobra: handled=%v err=%v calls=%v", handled, err, *calls)
		}
	})
	t.Run("disabled plugin is not reinstalled", func(t *testing.T) {
		_, calls, _ := relocateSetup(t, "", nil)
		dir := t.TempDir()
		t.Setenv("NSELF_PLUGIN_DIR", dir)
		if err := os.MkdirAll(filepath.Join(dir, "ci"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ci", ".disabled"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := relocateBreakout("ci", nil)
		if code := aiCode(err); code != "E407" || len(*calls) != 0 {
			t.Fatalf("want E407 and no install, got %v (calls %v)", err, *calls)
		}
	})
}

// recordOver replaces the seam recorder's target while keeping its call log.
func recordOver(calls *[]seamCall, fn func(string, string) (string, error)) func(string, string) (string, error) {
	return func(slug, version string) (string, error) {
		*calls = append(*calls, seamCall{slug, version})
		return fn(slug, version)
	}
}

func TestRelocateAutoAddPinned(t *testing.T) {
	proof, calls, warn := relocateSetup(t, aiVersion, nil)
	autoAddPlugin = recordOver(calls, fixtureSeam(t))
	handled, err := relocateBreakout("runner", []string{"verify", "x"})
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if !reflect.DeepEqual(*calls, []seamCall{{"ci", aiVersion}}) {
		t.Fatalf("the seam must run once with the locked version, got %v", *calls)
	}
	if want := "warning: 'nself runner' now comes from the ci plugin; installed ci@" + aiVersion + "\n"; warn.String() != want {
		t.Fatalf("warning %q, want %q", warn.String(), want)
	}
	got := readProof(t, proof)
	if got["argv"] != "nodes verify x" || got["NSELF_V15"] != "1" || got["NSELF_CLI_VERSION"] == "" {
		t.Fatalf("the published binary ran with %v", got)
	}

	t.Run("lock pin the catalogue does not serve", func(t *testing.T) {
		_, calls, _ := relocateSetup(t, "9.9.9", nil)
		autoAddPlugin = recordOver(calls, fixtureSeam(t))
		_, err := relocateBreakout("ci", nil)
		assertBreakoutRefused(t, err, "ci")
		if _, statErr := os.Stat(aiPublished()); statErr == nil {
			t.Fatal("a binary was published for a version nothing served")
		}
	})
}

func TestRelocateAutoAddCatalogue(t *testing.T) {
	proof, calls, warn := relocateSetup(t, "", nil)
	autoAddPlugin = recordOver(calls, fixtureSeam(t))
	handled, err := relocateBreakout("ci", []string{"--check"})
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if !reflect.DeepEqual(*calls, []seamCall{{"ci", ""}}) {
		t.Fatalf("with no lock the seam takes the catalogue version, got %v", *calls)
	}
	if strings.Count(warn.String(), "\n") != 1 || !strings.Contains(warn.String(), "installed ci@"+aiVersion) {
		t.Fatalf("want one warning naming ci@%s, got %q", aiVersion, warn.String())
	}
	if got := readProof(t, proof); got["argv"] != "--check" || got["NSELF_V15"] != "1" {
		t.Fatalf("the published binary ran with %v", got)
	}

	t.Run("silenced warning", func(t *testing.T) {
		_, calls, warn := relocateSetup(t, "", nil)
		autoAddPlugin = recordOver(calls, publishStub(t))
		old := os.Args
		os.Args = []string{"nself", "ci", "--quiet"}
		t.Cleanup(func() { os.Args = old })
		if _, err := relocateBreakout("ci", []string{"--quiet"}); err != nil || warn.Len() != 0 {
			t.Fatalf("err=%v warning=%q", err, warn.String())
		}
	})
}

func assertBreakoutRefused(t *testing.T, err error, slug string) {
	t.Helper()
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E404" || !strings.Contains(ce.Fix, "nself add "+slug) {
		t.Fatalf("want E404 with a fix naming nself add %s, got %v", slug, err)
	}
}

func TestRelocateNoAutoAdd(t *testing.T) {
	t.Run("NSELF_NO_AUTO_ADD=1", func(t *testing.T) {
		_, calls, _ := relocateSetup(t, "", nil)
		t.Setenv(envNoAutoAdd, "1")
		autoAddPlugin = recordOver(calls, publishStub(t))
		handled, err := relocateBreakout("server", []string{"list"})
		if !handled {
			t.Fatal("the break-out must be handled")
		}
		assertBreakoutRefused(t, err, "infra")
		if len(*calls) != 0 {
			t.Fatalf("the seam ran with auto-add off: %v", *calls)
		}
	})
	t.Run("unreachable registry", func(t *testing.T) {
		_, calls, _ := relocateSetup(t, "", nil)
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
		t.Setenv("NSELF_PLUGIN_REGISTRY", "http://127.0.0.1:9")
		autoAddPlugin = recordOver(calls, autoInstallPlugin)
		_, err := relocateBreakout("ci", nil)
		assertBreakoutRefused(t, err, "ci")
		if len(*calls) != 1 {
			t.Fatalf("seam calls %v", *calls)
		}
	})
	t.Run("unsigned release", func(t *testing.T) {
		_, calls, warn := relocateSetup(t, "", nil)
		tb := aiTarball(t, map[string]string{"nself-ci": stubScript})
		aiSigned(t, "") // pins a free-tier key; the served entry carries no signature
		downloads := aiServe(t, tb, aiSum(tb), nil)
		autoAddPlugin = recordOver(calls, autoInstallPlugin)
		_, err := relocateBreakout("ci", nil)
		assertBreakoutRefused(t, err, "ci")
		if atomic.LoadInt32(downloads) != 0 || warn.Len() != 0 {
			t.Fatalf("downloads=%d warning=%q", atomic.LoadInt32(downloads), warn.String())
		}
		if _, statErr := os.Stat(aiPublished()); statErr == nil {
			t.Fatal("an unsigned release was published")
		}
	})
	t.Run("tampered release", func(t *testing.T) {
		_, calls, warn := relocateSetup(t, "", nil)
		tb := aiTarball(t, map[string]string{"nself-ci": stubScript})
		want := aiSum([]byte("the published tarball"))
		aiServe(t, tb, want, aiSigned(t, want))
		autoAddPlugin = recordOver(calls, autoInstallPlugin)
		_, err := relocateBreakout("ci", nil)
		assertBreakoutRefused(t, err, "ci")
		if warn.Len() != 0 {
			t.Fatalf("warned about an install that failed: %q", warn.String())
		}
		if _, statErr := os.Stat(aiPublished()); statErr == nil {
			t.Fatal("a tampered release was published")
		}
	})
}

func TestRelocateRemovedE410(t *testing.T) {
	for _, argv := range [][]string{{"plugin", "marketplace"}, {"plugin", "marketplace", "list"}, {"plugin", "submit", "x"}} {
		_, _, err := rewriteCanonArgs(argv, true)
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E410" || !strings.Contains(err.Error(), "no plugin marketplace") || !strings.Contains(err.Error(), "v1.5.0") {
			t.Errorf("v1.5 %v: want E410 with the relocation text, got %v", argv, err)
		}
		if out, _, err := rewriteCanonArgs(argv, false); err != nil || !reflect.DeepEqual(out, argv) {
			t.Errorf("v1.4 %v must be untouched, got %v %v", argv, out, err)
		}
	}
}

func TestCanonBreakoutsRegistry(t *testing.T) {
	want := map[string]string{
		"ci": "ci", "runner": "ci nodes", "functions": "functions", "server": "infra server",
		"plugin init": "plugin-dev init", "plugin new": "plugin-dev new", "plugin dev": "plugin-dev dev",
		"plugin debug": "plugin-dev debug", "plugin link": "plugin-dev link", "plugin unlink": "plugin-dev unlink",
		"plugin test": "plugin-dev test",
	}
	got := map[string]string{}
	for _, r := range canonTable.Breakouts {
		got[strings.Join(r.From, " ")] = strings.Join(r.To, " ")
		if r.Since != "v1.5.0" {
			t.Errorf("break-out %v since %q", r.From, r.Since)
		}
	}
	for from, to := range want {
		if got[from] != to {
			t.Errorf("break-out %q -> %q, want %q", from, got[from], to)
		}
	}
	removed := map[string]bool{}
	for _, r := range canonTable.Removed {
		removed[strings.Join(r.From, " ")] = true
		if r.Message == "" {
			t.Errorf("removed %v has no relocation text", r.From)
		}
	}
	if !removed["plugin marketplace"] || !removed["plugin submit"] {
		t.Errorf("removed rows %v", removed)
	}

	v14, err := canon.Effective(false)
	if err != nil {
		t.Fatal(err)
	}
	v15, err := canon.Effective(true)
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{"ci", "runner", "functions", "server"} {
		if e, ok := v14.Commands[root]; !ok || e.Canon != canon.CanonPending {
			t.Errorf("v1.4 %q: %+v %v", root, e, ok)
		}
		if _, ok := v15.Commands[root]; ok {
			t.Errorf("v1.5 still lists the core command %q", root)
		}
	}
	for from := range want {
		if strings.HasPrefix(from, "plugin ") {
			if _, ok := v14.Commands[from]; !ok {
				t.Errorf("v1.4 lost %q", from)
			}
			if _, ok := v15.Commands[from]; ok {
				t.Errorf("v1.5 still lists %q", from)
			}
		}
	}
	// exactly the four pending roots leave the v1.5 view because of break-outs
	// (other moves also change the view; the binary-level count is in the verification)
	gone := 0
	for _, r := range canonTable.Breakouts {
		root := r.From[0]
		if _, in15 := v15.Commands[root]; len(r.From) == 1 && v14.Commands[root].Canon == canon.CanonPending && !in15 {
			gone++
		}
	}
	if gone != 4 {
		t.Errorf("%d pending roots leave the v1.5 view through break-outs, want 4", gone)
	}
}

// relocate.go may only start a plugin through plugin.ProxyBinary or with
// ExecEnv on the command (contract:cli.plugin-exec-env v1).
func TestRelocateExecUsesExecEnv(t *testing.T) {
	src, err := os.ReadFile("relocate.go")
	if err != nil {
		t.Fatal(err)
	}
	if bad := execSitesWithoutExecEnv(t, "relocate.go", string(src)); len(bad) != 0 {
		t.Fatalf("relocate.go: %v build an exec.Cmd without ExecEnv", bad)
	}
	fixture := "package commands\nimport \"os/exec\"\nfunc bad() { _ = exec.Command(\"x\") }\nfunc good() { c := exec.Command(\"x\"); c.Env = plugin.ExecEnv(nil, nil) }\n"
	if got := execSitesWithoutExecEnv(t, "fixture.go", fixture); !reflect.DeepEqual(got, []string{"bad"}) {
		t.Fatalf("the check must flag exactly bad, got %v", got)
	}
}

func execSitesWithoutExecEnv(t *testing.T, name, src string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var bad []string
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		builds, calls := false, false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				if x, ok := fun.X.(*ast.Ident); ok && x.Name == "exec" && (fun.Sel.Name == "Command" || fun.Sel.Name == "CommandContext") {
					builds = true
				}
				if fun.Sel.Name == "ExecEnv" {
					calls = true
				}
			case *ast.Ident:
				if fun.Name == "ExecEnv" {
					calls = true
				}
			}
			return true
		})
		if builds && !calls {
			bad = append(bad, fn.Name.Name)
		}
	}
	return bad
}

func TestProxyBinaryName(t *testing.T) {
	for in, want := range map[string]string{
		"/p/bin/nself-ci":     "nself-ci",
		`/p/bin/nself-ci.exe`: "nself-ci",
		"nself-runner":        "nself-runner",
	} {
		if got := proxyBinaryName(in); got != want {
			t.Errorf("proxyBinaryName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRelocateExecFailureKeepsE406 pins D-0908: a break-out binary the OS will
// not run (noexec HOME, quarantine, bad format) must come back as a coded E406
// error, so report() can emit the --json envelope. It used to be flattened to
// a silent exit 1 after a plain "Plugin error:" line.
func TestRelocateExecFailureKeepsE406(t *testing.T) {
	for name, seam := range map[string]func(slug, version string) (string, error){
		"not executable": func(slug, _ string) (string, error) {
			p := plugin.PublishedBinaryPath("nself-" + slug)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return "", err
			}
			return p, os.WriteFile(p, []byte(stubScript), 0o644)
		},
		"exec refused by the OS": func(slug, _ string) (string, error) {
			p := plugin.PublishedBinaryPath("nself-" + slug)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return "", err
			}
			return p, os.WriteFile(p, []byte("not a program\n"), 0o755)
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, calls, _ := relocateSetup(t, "", nil)
			t.Setenv("HOME", t.TempDir())
			autoAddPlugin = recordOver(calls, seam)
			handled, err := relocateBreakout("ci", []string{"--json"})
			if !handled {
				t.Fatal("the break-out must be handled")
			}
			if code := aiCode(err); code != "E406" {
				t.Fatalf("want a coded E406 error, got %q (%v)", code, err)
			}
			var silent errs.Silencer
			if errors.As(err, &silent) && silent.Silent() {
				t.Fatalf("a silent error prints no JSON envelope: %v", err)
			}
		})
	}
}

// TestWarningsSilencedStopsAtDoubleDash pins D-0908: after `--` every word
// belongs to the plugin, so --quiet there must not hide the relocation warning.
func TestWarningsSilencedStopsAtDoubleDash(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"nself", "ci", "--quiet"}, true},
		{[]string{"nself", "--no-deprecation-warnings", "ci"}, true},
		{[]string{"nself", "ci", "--", "--quiet"}, false},
		{[]string{"nself", "ci", "--", "--no-deprecation-warnings"}, false},
		{[]string{"nself", "ci", "--quiet", "--", "x"}, true},
	} {
		if got := warningsSilenced(tc.args); got != tc.want {
			t.Errorf("warningsSilenced(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}
