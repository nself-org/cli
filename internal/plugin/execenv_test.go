package plugin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/version"
)

func envValue(env []string, key string) (string, int) {
	val, n := "", 0
	for _, kv := range env {
		if strings.HasPrefix(kv, key+"=") {
			val, n = strings.TrimPrefix(kv, key+"="), n+1
		}
	}
	return val, n
}

func TestExecEnvTable(t *testing.T) {
	yes := func() bool { return true }
	no := func() bool { return false }
	tests := []struct {
		name     string
		base     []string
		v15      func() bool
		wantMode string
	}{
		{"resolver true, unset", []string{"A=1"}, yes, "1"},
		{"resolver false, unset", []string{"A=1"}, no, "0"},
		{"user value passes through", []string{"NSELF_V15=0", "A=1"}, yes, "0"},
		{"user value 1 passes through", []string{"NSELF_V15=1"}, no, "1"},
		{"user odd value passes through", []string{"NSELF_V15=banana"}, yes, "banana"},
		{"user empty value passes through", []string{"NSELF_V15="}, yes, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := append([]string(nil), tt.base...)
			got := ExecEnv(base, tt.v15)
			if m, n := envValue(got, "NSELF_V15"); m != tt.wantMode || n != 1 {
				t.Fatalf("NSELF_V15 = %q (x%d), want %q (x1)", m, n, tt.wantMode)
			}
			if v, n := envValue(got, "NSELF_CLI_VERSION"); v != strings.TrimPrefix(version.GetVersion(), "v") || n != 1 {
				t.Fatalf("NSELF_CLI_VERSION = %q (x%d)", v, n)
			}
			if a, _ := envValue(got, "A"); len(tt.base) > 0 && tt.base[len(tt.base)-1] == "A=1" && a != "1" {
				t.Fatalf("base entry lost: %v", got)
			}
			for i, kv := range tt.base {
				if base[i] != kv {
					t.Fatalf("base modified: %v", base)
				}
			}
		})
	}
}

func TestExecEnvReplacesInheritedVersion(t *testing.T) {
	got := ExecEnv([]string{"NSELF_CLI_VERSION=0.0.1"}, nil)
	if v, n := envValue(got, "NSELF_CLI_VERSION"); n != 1 || v != version.GetVersion() {
		t.Fatalf("version = %q (x%d)", v, n)
	}
}

func TestExecEnvStripsVPrefix(t *testing.T) {
	old := version.Version
	version.Version = "v9.8.7"
	t.Cleanup(func() { version.Version = old })
	if v, _ := envValue(ExecEnv(nil, nil), "NSELF_CLI_VERSION"); v != "9.8.7" {
		t.Fatalf("version = %q", v)
	}
}

// The mode is read on every call: flipping the environment between calls
// changes the result (no caching across calls).
func TestExecEnvNilResolverReadsEveryCall(t *testing.T) {
	t.Setenv(compat.EnvVar, "")
	os.Unsetenv(compat.EnvVar)
	if m, _ := envValue(ExecEnv(nil, nil), "NSELF_V15"); m != "0" {
		t.Fatalf("unset mode = %q", m)
	}
	t.Setenv(compat.EnvVar, "1")
	if m, _ := envValue(ExecEnv(nil, nil), "NSELF_V15"); m != "1" {
		t.Fatalf("set mode = %q", m)
	}
}

// A stub plugin that prints its environment, run through the proxy the mount
// uses, sees the resolved mode and the cli version.
func TestMountStubPluginEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.Unsetenv(compat.EnvVar)
	bin := filepath.Join(home, ".nself", "plugins", "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	proof := filepath.Join(home, "env.txt")
	t.Setenv("ENV_PROOF", proof)
	script := "#!/bin/sh\nenv | grep -E '^NSELF_(V15|CLI_VERSION)=' > \"$ENV_PROOF\"\n"
	if err := os.WriteFile(filepath.Join(bin, "nself-demo"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ set, want string }{{"", "0"}, {"1", "1"}, {"0", "0"}, {"yes", "yes"}} {
		if tc.set == "" {
			os.Unsetenv(compat.EnvVar)
		} else {
			t.Setenv(compat.EnvVar, tc.set)
		}
		if err := ProxyBinary("nself-demo", "demo", nil); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(proof)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		got := map[string]string{}
		for _, l := range lines {
			k, v, _ := strings.Cut(l, "=")
			got[k] = v
		}
		if got["NSELF_V15"] != tc.want || got["NSELF_CLI_VERSION"] != strings.TrimPrefix(version.GetVersion(), "v") || len(lines) != 2 {
			t.Fatalf("set=%q: plugin saw %v", tc.set, lines)
		}
	}
}

// Every function in router.go and runtime_start.go that builds an exec.Cmd
// must call ExecEnv (contract:cli.plugin-exec-env v1). cmd/commands/relocate.go
// carries the same check from P7-CANON-13.
func TestExecSitesUseExecEnv(t *testing.T) {
	for _, file := range []string{"router.go", "runtime_start.go"} {
		for _, f := range execSitesWithoutExecEnv(t, file) {
			t.Errorf("%s: func %s builds an exec.Cmd without calling ExecEnv", file, f)
		}
	}
}

func TestExecSiteCheckDetectsMissingExecEnv(t *testing.T) {
	src := `package plugin
import "os/exec"
func bad() { _ = exec.Command("x") }
func good() { c := exec.CommandContext(nil, "x"); c.Env = ExecEnv(nil, nil) }
func other() {}
`
	got := execSitesInSource(t, "fixture.go", src)
	if len(got) != 1 || got[0] != "bad" {
		t.Fatalf("got %v, want [bad]", got)
	}
}

func execSitesWithoutExecEnv(t *testing.T, file string) []string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return execSitesInSource(t, file, string(src))
}

func execSitesInSource(t *testing.T, name, src string) []string {
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
