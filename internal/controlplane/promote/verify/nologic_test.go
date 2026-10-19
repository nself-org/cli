package verify

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// evidenceWords are evidence semantics that belong to the ci plugin only.
var evidenceWords = regexp.MustCompile(`(?i)\bfresh|binding|unbound|stale|lockfile|input.?digest|worktree|policy.?digest|supersed`)

// TestReleaseVerifyNoEvidenceLogic proves cli holds no evidence-semantics
// code: no identifier or string literal of the production files in this
// package or its command wiring names freshness, binding or staleness.
func TestReleaseVerifyNoEvidenceLogic(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, filepath.Join("..", "..", "..", "..", "cmd", "commands", "release_manifest_flag.go"))
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		checked++
		parsed, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.Ident:
				if evidenceWords.MatchString(v.Name) {
					t.Errorf("%s: identifier %q is evidence semantics", f, v.Name)
				}
			case *ast.BasicLit:
				if v.Kind == token.STRING && evidenceWords.MatchString(v.Value) {
					t.Errorf("%s: literal %s is evidence semantics", f, v.Value)
				}
			}
			return true
		})
	}
	if checked < 6 {
		t.Fatalf("only %d production files checked; the glob is wrong", checked)
	}
}
