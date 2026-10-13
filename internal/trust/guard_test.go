// Package trust — guard_test.go: unit tests must never shell out to osascript
// or sudo.
//
// Two layers: a static scan that fails when trust, ssl or the dns/trust
// commands call os/exec directly (everything must go through sysexec so tests
// can fake it), and a runtime guard that panics when a test reaches the real
// runner with an admin command. Each layer has a positive control so a green
// run proves the check can fail.
package trust

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/sysexec"
)

// directExecCalls returns "file:line exec.X" for every exec.Command or
// exec.CommandContext call in src (an unresolved selector on an identifier
// named exec, which is how os/exec is imported everywhere in this repo).
func directExecCalls(filename string, src any) ([]string, error) {
	calls, err := directExecCallsBin(filename, src)
	var hits []string
	for _, c := range calls {
		hits = append(hits, c.pos+" exec."+c.fn+"("+c.bin+")")
	}
	return hits, err
}

type execCall struct{ pos, fn, bin string }

// directExecCallsBin is directExecCalls with the literal binary name of each
// call ("?" when it is not a string literal).
func directExecCallsBin(filename string, src any) ([]execCall, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}
	var hits []execCall
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
		if ok && id.Name == "exec" && (sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext") {
			idx := 0
			if sel.Sel.Name == "CommandContext" {
				idx = 1
			}
			bin := "?"
			if len(call.Args) > idx {
				if lit, ok := call.Args[idx].(*ast.BasicLit); ok {
					bin = strings.Trim(lit.Value, `"`)
				}
			}
			hits = append(hits, execCall{fset.Position(call.Pos()).String(), sel.Sel.Name, bin})
		}
		return true
	})
	return hits, nil
}

func TestDirectExecScan_PositiveControl(t *testing.T) {
	bad := `package x
import ("context"; "os/exec")
func f(ctx context.Context) { _ = exec.CommandContext(ctx, "osascript", "-e", "x"); _ = exec.Command("sudo", "true") }`
	hits, err := directExecCalls("bad.go", bad)
	if err != nil || len(hits) != 2 {
		t.Fatalf("scanner missed seeded exec calls: hits=%v err=%v", hits, err)
	}
	cmdBins, _ := directExecCallsBin("bad.go", bad)
	if len(cmdBins) != 2 || cmdBins[0].bin != "osascript" || cmdBins[1].bin != "sudo" {
		t.Fatalf("scanner did not read the binary names: %+v", cmdBins)
	}
	good := `package x
import ("os/exec")
func f() { _, _ = exec.LookPath("mkcert") }`
	if hits, _ := directExecCalls("good.go", good); len(hits) != 0 {
		t.Fatalf("scanner flagged LookPath: %v", hits)
	}
}

// TestNoDirectExecInPrivilegedSetupCode fails when the trust package, the ssl
// CA/hosts code or the dns/trust commands run a command without going through
// sysexec. ssl/mkcert.go keeps one allowed exec.Command for openssl, an
// unprivileged cert generator.
func TestNoDirectExecInPrivilegedSetupCode(t *testing.T) {
	var files []string
	for _, dir := range []string{".", "../ssl"} {
		m, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		files = append(files, m...)
	}
	files = append(files, "../../cmd/commands/dns.go")
	m, _ := filepath.Glob("../../cmd/commands/trust*.go")
	files = append(files, m...)

	// The one allowed direct exec: openssl (an unprivileged cert generator) in
	// ssl/mkcert.go. Any other binary there, mkcert included, must use sysexec.
	scanned := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("scan target missing: %s", f)
		}
		calls, err := directExecCallsBin(f, nil)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		scanned++
		for _, c := range calls {
			if f == filepath.FromSlash("../ssl/mkcert.go") && c.bin == "openssl" {
				continue
			}
			t.Errorf("direct os/exec call; route it through internal/sysexec: %s exec.%s(%s)", c.pos, c.fn, c.bin)
		}
	}
	if scanned < 15 {
		t.Errorf("scan read only %d files; the glob is broken", scanned)
	}
}

// TestRealRunnerGuard_StopsAdminCommandsFromCode drives real package code with
// the real runner. setupResolver issues osascript; the guard must panic before
// anything executes. The positive control is the same call under a fake.
func TestRealRunnerGuard_StopsAdminCommandsFromCode(t *testing.T) {
	hermetic(t)

	runResolver := func() (panicked bool) {
		defer func() {
			if r := recover(); r != nil {
				if msg, _ := r.(string); strings.Contains(msg, "privileged command") {
					panicked = true
					return
				}
				panic(r)
			}
		}()
		restore := sysexec.Set(sysexec.Real())
		defer restore()
		_ = runSetupResolverForGuard()
		return false
	}
	if !runResolver() {
		t.Fatal("real runner let an osascript admin command through during a unit test")
	}

	// Control: under a fake the same code path completes and records osascript.
	fake := &sysexec.Fake{}
	defer sysexec.Set(fake)()
	_ = runSetupResolverForGuard()
	if !fake.Saw("osascript") {
		t.Error("control run did not exercise the admin command")
	}
}

// runSetupResolverForGuard issues the admin command on the platform that has
// one, and a direct osascript through the runner elsewhere, so the guard test
// is meaningful on every OS the suite runs on.
func runSetupResolverForGuard() error {
	return adminProbe(context.Background())
}
