package repoqa

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Unit tests must never reach the host's privileged tools. On a headless CI
// Mac one osascript or sudo call raises an administrator dialog nobody can
// answer and the run hangs (cam handoff 2026-10-13, cli#752). internal/trust
// has the deep guard for its own packages; this one is the repo-wide net: any
// _test.go file that starts osascript, sudo, security, pfctl or launchctl
// through os/exec, directly or inside an `sh -c` string, fails the build.
// Fake binaries named in an argv slice for a stub, and strings that merely
// mention these names, are not calls and are not flagged.

var systemTools = map[string]bool{"osascript": true, "sudo": true, "security": true, "pfctl": true, "launchctl": true}

var systemToolWord = regexp.MustCompile(`(^|[\s;&|(])(osascript|sudo|security|pfctl|launchctl)([\s;&|)]|$)`)

var shellNames = map[string]bool{"sh": true, "bash": true, "zsh": true}

// wrappers start another program named by their later arguments.
var wrappers = map[string]bool{"env": true, "nice": true, "nohup": true, "time": true, "timeout": true, "xargs": true, "command": true, "exec": true, "stdbuf": true}

// fileStrings maps names bound to a string anywhere in the file (const, var,
// := with a literal or a concatenation of literals), so `bin := "sudo"` and
// `const tool = "osascript"` are seen through exec.Command(bin).
func fileStrings(f *ast.File) map[string]string {
	out := map[string]string{}
	resolve := func(e ast.Expr) (string, bool) { return constString(e, out) }
	ast.Inspect(f, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.ValueSpec:
			for i, name := range v.Names {
				if i < len(v.Values) {
					if str, ok := resolve(v.Values[i]); ok {
						out[name.Name] = str
					}
				}
			}
		case *ast.AssignStmt:
			if v.Tok == token.DEFINE || v.Tok == token.ASSIGN {
				for i, lhs := range v.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && i < len(v.Rhs) {
						if str, ok := resolve(v.Rhs[i]); ok {
							out[id.Name] = str
						}
					}
				}
			}
		}
		return true
	})
	return out
}

// constString evaluates a string literal, a name bound earlier in the file, or
// a + concatenation of those.
func constString(e ast.Expr, names map[string]string) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			return strings.Trim(v.Value, "\"`"), true
		}
	case *ast.Ident:
		str, ok := names[v.Name]
		return str, ok
	case *ast.ParenExpr:
		return constString(v.X, names)
	case *ast.BinaryExpr:
		if v.Op == token.ADD {
			l, lok := constString(v.X, names)
			r, rok := constString(v.Y, names)
			return l + r, lok && rok
		}
	}
	return "", false
}

// execName is the local name os/exec is imported under ("" when not imported).
func execName(f *ast.File) string {
	for _, imp := range f.Imports {
		if strings.Trim(imp.Path.Value, "\"") == "os/exec" {
			if imp.Name != nil {
				return imp.Name.Name
			}
			return "exec"
		}
	}
	return ""
}

// systemToolCalls returns "file:line exec.X(bin)" for every os/exec call in
// src that starts a system tool, directly, through a name or constant, in an
// `sh -c` string, or behind a wrapper such as env.
func systemToolCalls(filename string, src any) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}
	local := execName(f)
	if local == "" {
		return nil, nil
	}
	names := fileStrings(f)
	var hits []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Name != local || (sel.Sel.Name != "Command" && sel.Sel.Name != "CommandContext") {
			return true
		}
		first := 0
		if sel.Sel.Name == "CommandContext" {
			first = 1
		}
		if len(call.Args) <= first {
			return true
		}
		var argv []string
		for _, a := range call.Args[first:] {
			str, _ := constString(a, names)
			argv = append(argv, str)
		}
		if bad := startsSystemTool(argv); bad != "" {
			hits = append(hits, fset.Position(call.Pos()).String()+" "+local+"."+sel.Sel.Name+"("+bad+")")
		}
		return true
	})
	return hits, nil
}

// startsSystemTool returns the offending program for an argv, or "".
func startsSystemTool(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	bin := filepath.Base(argv[0])
	switch {
	case systemTools[bin]:
		return bin
	case shellNames[bin]:
		for _, a := range argv[1:] {
			if systemToolWord.MatchString(a) {
				return bin + " " + strings.TrimSpace(a)
			}
		}
	case wrappers[bin]:
		for _, a := range argv[1:] {
			if systemTools[filepath.Base(a)] {
				return bin + " " + a
			}
		}
	}
	return ""
}

func TestNoSystemToolsInUnitTests_PositiveControl(t *testing.T) {
	bad := `package x
import ("context"; "os/exec")
func f(ctx context.Context) {
	_ = exec.CommandContext(ctx, "osascript", "-e", "x")
	_ = exec.Command("sudo", "true")
	_ = exec.Command("sh", "-c", "echo hi && launchctl list")
	bin := "su" + "do"
	_ = exec.Command(bin, "true")
	_ = exec.Command("env", "FOO=1", "pfctl")
	const tool = "security"
	_ = exec.Command(tool, "list-keychains")
}`
	hits, err := systemToolCalls("bad_test.go", bad)
	if err != nil || len(hits) != 6 {
		t.Fatalf("guard missed seeded calls: hits=%v err=%v", hits, err)
	}
	aliased := `package x
import osexec "os/exec"
func f() { _ = osexec.Command("osascript") }`
	if hits, _ := systemToolCalls("alias_test.go", aliased); len(hits) != 1 {
		t.Fatalf("guard missed an aliased os/exec import: %v", hits)
	}
	good := `package x
import ("os/exec")
func f() { _ = exec.Command("git", "status"); _ = exec.Command("sh", "-c", "echo security-note"); _ = "sudo" }`
	if hits, _ := systemToolCalls("good_test.go", good); len(hits) != 0 {
		t.Fatalf("guard flagged harmless code: %v", hits)
	}
}

func TestNoSystemToolsInUnitTests(t *testing.T) {
	root := repoRoot(t)
	scanned := 0
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		scanned++
		found, perr := systemToolCalls(path, nil)
		if perr != nil {
			return perr
		}
		hits = append(hits, found...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 100 {
		t.Fatalf("scanned only %d test files: the walk is broken", scanned)
	}
	for _, h := range hits {
		t.Errorf("unit test starts a system tool (use a fake binary or sysexec): %s", strings.TrimPrefix(h, root+"/"))
	}
}
