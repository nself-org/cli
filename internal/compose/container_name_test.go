package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// TestContainerNamesUseSharedConvention prevents service builders from copying
// the project/service naming rule, including builders added in future files.
func TestContainerNamesUseSharedConvention(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fields := 0
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			field, ok := node.(*ast.KeyValueExpr)
			if !ok {
				return true
			}
			key, ok := field.Key.(*ast.Ident)
			if !ok || key.Name != "ContainerName" {
				return true
			}
			fields++
			call, ok := field.Value.(*ast.CallExpr)
			if ok {
				if method, ok := call.Fun.(*ast.SelectorExpr); ok && method.Sel.Name == "containerName" {
					return true
				}
			}
			t.Errorf("%s: ContainerName must use the shared containerName convention", fset.Position(field.Pos()))
			return true
		})
	}
	if fields == 0 {
		t.Fatal("no container names checked")
	}
}

func TestContainerNamePreservesConvention(t *testing.T) {
	for _, tc := range []struct {
		project string
		service string
		want    string
	}{
		{"demo", "postgres", "demo_postgres"},
		{"my_project", "meilisearch_init", "my_project_meilisearch_init"},
		{"my-project", "custom_api-v2", "my-project_custom_api-v2"},
		{"", "auth", "_auth"},
		{"demo", "", "demo_"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			g := NewGenerator(&config.Config{ProjectName: tc.project})
			if got := g.containerName(tc.service); got != tc.want {
				t.Errorf("containerName(%q) = %q, want %q", tc.service, got, tc.want)
			}
		})
	}
}
