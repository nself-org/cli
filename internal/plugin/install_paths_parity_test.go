package plugin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestInstallAndRemovePathsShareTheirSteps guards a class of bug rather than an
// instance.
//
// There are two install paths — installLocked for registry plugins and
// installFromURL for third-party ones — and they had drifted. Two fixes made to
// the registry path were never made to the other:
//
//   - publishing the command binary, so a third-party CLI plugin installed
//     cleanly and its command did not exist;
//   - skipping schema creation for a plugin with no tables, so installing a
//     command-line plugin needed Docker and a running Postgres.
//
// Both were found by installing a third-party plugin end to end, months after
// the registry path was fixed. Nothing pointed at the second path.
//
// This asserts the steps that must exist in both. It is deliberately a
// source-level check: the two functions do genuinely different things and
// cannot be collapsed into one, so the only thing worth pinning is that neither
// forgets a step the other performs.
func TestInstallAndRemovePathsShareTheirSteps(t *testing.T) {
	required := []string{
		"linkCLIBinary",    // publish the command, or the plugin installs dead
		"pluginOwnsTables", // do not demand a database from a plugin with no tables
		"checkCLIBinaries", // refuse colliding commands before extraction (D-0259)
	}

	// Remove is the third path, and it had drifted the same way: it never
	// unpublished the command binary and demanded a database from a plugin with
	// no tables. What each path must do differs, so the requirements are
	// per-path rather than one shared list.
	for _, tc := range []struct {
		file, fn string
		steps    []string
	}{
		{"installer_locked.go", "installLocked", required},
		{"install_thirdparty.go", "InstallFromURL", required},
		{"installer_remove_update.go", "Remove", []string{
			"unlinkCLIBinary",  // or the command survives its own removal
			"pluginOwnsTables", // or removal needs a database the plugin never used
		}},
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, tc.file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.file, err)
		}

		var body string
		ast.Inspect(f, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if !ok || fd.Name.Name != tc.fn || fd.Body == nil {
				return true
			}
			start := fset.Position(fd.Body.Pos()).Offset
			end := fset.Position(fd.Body.End()).Offset
			src := readFile(t, tc.file)
			body = src[start:end]
			return false
		})

		if body == "" {
			t.Fatalf("%s: function %s not found — rename it here too", tc.file, tc.fn)
		}
		for _, want := range tc.steps {
			if !contains(body, want) {
				t.Errorf("%s does not call %s. Both install paths must perform this step; "+
					"the last time one of them did not, plugins installed successfully and "+
					"then did not work.", tc.fn, want)
			}
		}
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func contains(hay, needle string) bool {
	return len(hay) >= len(needle) && strings.Contains(hay, needle)
}
