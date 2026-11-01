package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// templateImageLiterals finds image strings and Dockerfile FROM operands.
// Inputs: decoded Go string or template content and known repositories.
// Outputs: image references that bypass the lock. Constraints: ignore stage
// aliases, template expressions and host:port values.
func templateImageLiterals(s string, repos map[string]bool) []string {
	if isImageLiteral(s, repos) {
		return []string{s}
	}
	var hits []string
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "FROM") {
			continue
		}
		for _, field := range fields[1:] {
			if strings.HasPrefix(field, "--") {
				continue
			}
			if isImageLiteral(field, repos) {
				hits = append(hits, field)
			}
			break
		}
	}
	return hits
}

// concatenatedImagePrefix checks the leftmost literal of a concatenation.
// An unknown suffix still makes a known repository prefix an image reference.
func concatenatedImagePrefix(n ast.Node, repos map[string]bool) string {
	bin, ok := n.(*ast.BinaryExpr)
	if !ok || bin.Op != token.ADD {
		return ""
	}
	s, _ := staticImagePrefix(bin)
	if isImageLiteral(s, repos) {
		return s
	}
	if strings.HasSuffix(s, ":") && isImageLiteral(s+"%s", repos) {
		return s + "%s"
	}
	return ""
}

// staticImagePrefix folds string additions up to their first dynamic operand.
func staticImagePrefix(expr ast.Expr) (string, bool) {
	switch x := expr.(type) {
	case *ast.ParenExpr:
		return staticImagePrefix(x.X)
	case *ast.BasicLit:
		if x.Kind == token.STRING {
			s, err := strconv.Unquote(x.Value)
			return s, err == nil
		}
	case *ast.BinaryExpr:
		if x.Op == token.ADD {
			left, complete := staticImagePrefix(x.X)
			if !complete {
				return left, false
			}
			right, complete := staticImagePrefix(x.Y)
			return left + right, complete
		}
	}
	return "", false
}

func TestTemplateImageLiterals(t *testing.T) {
	repos := imageRepos()
	for _, tc := range []struct {
		src  string
		want []string
	}{
		{"FROM node:20-alpine AS build\nFROM --platform=$BUILDPLATFORM python:3.12-slim", []string{"node:20-alpine", "python:3.12-slim"}},
		{"from nginx:alpine", []string{"nginx:alpine"}},
		{"FROM {{.BaseImage}}\nFROM build\nEXPOSE 8080", nil},
		{"redis:6379", nil},
	} {
		if got := templateImageLiterals(tc.src, repos); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("scan %q = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestConcatenatedImagePrefix(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`"postgres:" + version`, "postgres:%s"},
		{`("docker.io/library/postgres:") + version`, "docker.io/library/postgres:%s"},
		{`"post" + "gres:" + version`, "postgres:%s"},
		{`"python:3.12" + "-slim"`, "python:3.12-slim"},
		{`"localhost:" + port`, ""},
		{`"key:" + value`, ""},
	} {
		expr, err := parser.ParseExpr(tc.src)
		if err != nil {
			t.Fatal(err)
		}
		if got := concatenatedImagePrefix(expr, imageRepos()); got != tc.want {
			t.Errorf("scan %s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestImageScanIncludesTemplateFiles(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"Dockerfile.tmpl": "FROM node:20-alpine AS build\nFROM {{.RuntimeImage}}",
		"restore.go":      "package fixture\nvar image = \"postgres:\" + version\n",
		"embedded.go":     "package fixture\nvar dockerfile = `FROM nginx:alpine`\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	hits := scanImageLiterals(t, dir)
	if len(hits) != 3 {
		t.Fatalf("hits = %v; want three image references", hits)
	}
	for _, want := range []string{" node:20-alpine", " nginx:alpine", " postgres:%s"} {
		found := false
		for _, hit := range hits {
			found = found || strings.HasSuffix(hit, want)
		}
		if !found {
			t.Errorf("hits %v must contain %q", hits, want)
		}
	}
}
