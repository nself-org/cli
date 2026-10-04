package errs

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// exportedSentinelNames returns the names of every exported Err* variable
// declared in a non-test file of this package.
func exportedSentinelNames(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				for _, n := range spec.(*ast.ValueSpec).Names {
					if strings.HasPrefix(n.Name, "Err") && n.IsExported() {
						names = append(names, n.Name)
					}
				}
			}
		}
	}
	sort.Strings(names)
	return names
}

// tableSentinelNames parses sentinels.go and returns the identifier in the
// first position of every sentinelTable row.
func tableSentinelNames(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "sentinels.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	ast.Inspect(file, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || vs.Names[0].Name != "sentinelTable" || len(vs.Values) != 1 {
			return true
		}
		for _, el := range vs.Values[0].(*ast.CompositeLit).Elts {
			row := el.(*ast.CompositeLit)
			names = append(names, row.Elts[0].(*ast.Ident).Name)
		}
		return false
	})
	sort.Strings(names)
	return names
}

// TestSentinelTable_CoversEveryErrVar is the AST completeness test: every
// exported Err* in the package has exactly one table row, and no row names
// anything else.
func TestSentinelTable_CoversEveryErrVar(t *testing.T) {
	decl, tab := exportedSentinelNames(t), tableSentinelNames(t)
	if len(decl) != len(tab) {
		t.Errorf("found %d exported Err* vars but %d sentinel-table rows", len(decl), len(tab))
	}
	count := map[string]int{}
	for _, n := range tab {
		count[n]++
	}
	for _, n := range decl {
		if count[n] != 1 {
			t.Errorf("%s has %d sentinel-table rows, want exactly 1 (add it to sentinels.go with a registry code)", n, count[n])
		}
		delete(count, n)
	}
	for n := range count {
		t.Errorf("sentinel table names %s, which is not an exported Err* var", n)
	}
}

// TestSentinelTable_RowsAreValid checks each row's code is registered and the
// error values are distinct and non-nil.
func TestSentinelTable_RowsAreValid(t *testing.T) {
	for i, s := range sentinelTable {
		if s.Err == nil {
			t.Fatalf("row %d (%s): nil sentinel", i, s.Code)
		}
		if _, ok := Registry[s.Code]; !ok {
			t.Errorf("%v maps to unregistered code %s", s.Err, s.Code)
		}
		for j := 0; j < i; j++ {
			if errors.Is(s.Err, sentinelTable[j].Err) {
				t.Errorf("rows %d and %d hold the same sentinel (%v)", j, i, s.Err)
			}
		}
	}
}

// TestExitCodeFor_SentinelMatchesRegistry proves ExitCodeFor and the registry
// agree for every mapped sentinel, bare and wrapped.
func TestExitCodeFor_SentinelMatchesRegistry(t *testing.T) {
	for _, s := range sentinelTable {
		want := Registry[s.Code].Exit
		if got := ExitCodeFor(fmt.Errorf("ctx: %w", s.Err)); got != want {
			t.Errorf("ExitCodeFor(wrapped %v) = %d, want %d (%s)", s.Err, got, want, s.Code)
		}
		if got := ExitCodeFor(s.Err); got != want {
			t.Errorf("ExitCodeFor(%v) = %d, want %d (%s)", s.Err, got, want, s.Code)
		}
		code, ok := CodeForSentinel(fmt.Errorf("ctx: %w", s.Err))
		if !ok || code != s.Code {
			t.Errorf("CodeForSentinel(%v) = %q, %v, want %s", s.Err, code, ok, s.Code)
		}
	}
}

// TestExitCodeFor_Acceptance pins the examples from the Ticket.
func TestExitCodeFor_Acceptance(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"tier not entitled", fmt.Errorf("x: %w", ErrTierNotEntitled), 3},
		{"prune failed", fmt.Errorf("x: %w", ErrBackupPruneFailed), 2},
		{"destructive", fmt.Errorf("x: %w", ErrDestructiveBlocked), 4},
		{"plain", fmt.Errorf("x"), 1},
		{"ExitError over sentinel", ExitWith(7, fmt.Errorf("x: %w", ErrTierNotEntitled)), 7},
		{"ExitError over CLIError", ExitWith(5, New("E403", "no")), 5},
		{"CLIError over sentinel", Wrap("E401", "bad flag", ErrDockerNotRunning), 1},
		{"CLIError E403", New("E403", "blocked"), 4},
		{"CLIError E400 falls to sentinel", Wrap("E400", "x", ErrDockerNotRunning), 2},
		{"CLIError E400 plain", New("E400", "x"), 1},
		{"CLIError unregistered code", Wrap("E999", "x", ErrLicenseExpired), 3},
		{"CLIError through ExitError exit 0", Exit(0), 0},
	} {
		if got := ExitCodeFor(tc.err); got != tc.want {
			t.Errorf("%s: ExitCodeFor = %d, want %d", tc.name, got, tc.want)
		}
	}
}
