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

// systemToolCalls returns "file:line exec.X(bin)" for every os/exec call in
// src that starts a system tool.
func systemToolCalls(filename string, src any) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}
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
		if !ok || id.Name != "exec" || (sel.Sel.Name != "Command" && sel.Sel.Name != "CommandContext") {
			return true
		}
		first := 0
		if sel.Sel.Name == "CommandContext" {
			first = 1
		}
		if len(call.Args) <= first {
			return true
		}
		bin := stringLit(call.Args[first])
		bad := systemTools[filepath.Base(bin)]
		if shellNames[filepath.Base(bin)] {
			for _, a := range call.Args[first+1:] {
				if systemToolWord.MatchString(stringLit(a)) {
					bad = true
				}
			}
		}
		if bad {
			hits = append(hits, fset.Position(call.Pos()).String()+" exec."+sel.Sel.Name+"("+bin+")")
		}
		return true
	})
	return hits, nil
}

func stringLit(e ast.Expr) string {
	if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		return strings.Trim(lit.Value, "\"`")
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
}`
	hits, err := systemToolCalls("bad_test.go", bad)
	if err != nil || len(hits) != 3 {
		t.Fatalf("guard missed seeded calls: hits=%v err=%v", hits, err)
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
