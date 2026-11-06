package pingcontract

// Static scan: finds every ping.nself.org endpoint the CLI calls, as
// (method, path), by parsing non-test Go files. No network.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// endpoint is one call site: the paths and methods are what the code says.
type endpoint struct {
	Method string
	Path   string
	// Prefix is true when the literal is a base URL the code appends to
	// (a constant named *BaseURL), so the spec path may extend it.
	Prefix bool
	Where  string
}

// pingURLLiteral matches a whole string literal that is a ping URL.
var pingURLLiteral = regexp.MustCompile(`^https?://ping\.nself\.org(/[A-Za-z0-9_./{}%-]*)?(\?.*)?$`)

// pingIdent matches the identifiers that hold the ping base URL.
var pingIdent = regexp.MustCompile(`(?i)ping`)

var methodNames = map[string]bool{
	http.MethodGet: true, http.MethodPost: true, http.MethodPut: true,
	http.MethodDelete: true, http.MethodHead: true, http.MethodPatch: true,
}

func repoRoot(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}

// scanRepo parses every non-test .go file outside vendor and this package.
func scanRepo(t testing.TB, root string) []endpoint {
	t.Helper()
	var out []endpoint
	skip := map[string]bool{"vendor": true, ".git": true, "node_modules": true}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			if skip[d.Name()] || rel == filepath.Join("internal", "pingcontract") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		eps, err := scanSource(rel, src)
		if err != nil {
			return err
		}
		out = append(out, eps...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// scanSource finds ping endpoints in one Go source file.
func scanSource(name string, src []byte) ([]endpoint, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var out []endpoint
	emit := func(lit *ast.BasicLit, path string, prefix bool, fn ast.Node, usageOf string) {
		methods := methodsIn(fn)
		if fn == nil && usageOf != "" {
			methods = methodsOfUsages(f, usageOf)
		}
		if len(methods) == 0 {
			methods = []string{http.MethodGet}
		}
		for _, m := range methods {
			out = append(out, endpoint{Method: m, Path: path, Prefix: prefix,
				Where: name + ":" + strconv.Itoa(fset.Position(lit.Pos()).Line)})
		}
	}
	seen := map[*ast.BasicLit]bool{}
	for _, decl := range f.Decls {
		// Package-level constants and vars bound to a full ping URL.
		if gd, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, v := range vs.Values {
					lit, ok := v.(*ast.BasicLit)
					if !ok || i >= len(vs.Names) {
						continue
					}
					if path, ok := urlPath(lit); ok {
						seen[lit] = true
						emit(lit, path, strings.HasSuffix(vs.Names[i].Name, "BaseURL"), nil, vs.Names[i].Name)
					}
				}
			}
			continue
		}
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if path, ok := urlPath(x); ok && !seen[x] {
					seen[x] = true
					emit(x, path, false, fd, "")
				}
			case *ast.BinaryExpr:
				if x.Op != token.ADD {
					return true
				}
				ops := flatten(x)
				if !hasPingOperand(ops) {
					return true
				}
				for _, o := range ops {
					lit, ok := o.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING || seen[lit] {
						continue
					}
					s, _ := strconv.Unquote(lit.Value)
					if strings.HasPrefix(s, "/") {
						seen[lit] = true
						emit(lit, strings.SplitN(s, "?", 2)[0], false, fd, "")
					}
				}
				return false
			}
			return true
		})
	}
	return out, nil
}

func urlPath(lit *ast.BasicLit) (string, bool) {
	if lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	m := pingURLLiteral.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	if m[1] == "" {
		return "/", true
	}
	return m[1], true
}

// flatten returns the operands of a chain of + expressions, left to right.
func flatten(e ast.Expr) []ast.Expr {
	if b, ok := e.(*ast.BinaryExpr); ok && b.Op == token.ADD {
		return append(flatten(b.X), flatten(b.Y)...)
	}
	return []ast.Expr{e}
}

// hasPingOperand reports whether a non-literal operand names the ping base.
func hasPingOperand(ops []ast.Expr) bool {
	for _, o := range ops {
		if _, ok := o.(*ast.BasicLit); ok {
			continue
		}
		found := false
		ast.Inspect(o, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && pingIdent.MatchString(id.Name) {
				found = true
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

// methodsIn lists the HTTP methods a function uses, sorted.
func methodsIn(fn ast.Node) []string {
	if fn == nil {
		return nil
	}
	set := map[string]bool{}
	ast.Inspect(fn, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok && id.Name == "http" && strings.HasPrefix(x.Sel.Name, "Method") {
				set[strings.ToUpper(strings.TrimPrefix(x.Sel.Name, "Method"))] = true
			}
			switch x.Sel.Name {
			case "Get", "Head":
				set[strings.ToUpper(x.Sel.Name)] = true
			case "Post", "PostForm":
				set[http.MethodPost] = true
			}
		case *ast.BasicLit:
			if s, err := strconv.Unquote(x.Value); err == nil && x.Kind == token.STRING && methodNames[s] {
				set[s] = true
			}
		}
		return true
	})
	return sortedKeys(set)
}

// methodsOfUsages finds the methods of the functions that reference name.
func methodsOfUsages(f *ast.File, name string) []string {
	set := map[string]bool{}
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		uses := false
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == name {
				uses = true
			}
			return !uses
		})
		if uses {
			for _, m := range methodsIn(fd) {
				set[m] = true
			}
		}
	}
	return sortedKeys(set)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestScanFindsTheDocumentedEndpoints pins the endpoint list the ticket names.
func TestScanFindsTheDocumentedEndpoints(t *testing.T) {
	eps := scanRepo(t, repoRoot(t))
	have := map[string]bool{}
	for _, e := range eps {
		have[e.Method+" "+e.Path] = true
	}
	want := []string{
		"POST /license/validate", "GET /license/revocation-list", "GET /license/events",
		"GET /health", "GET /version", "POST /telemetry",
		"POST /install-counter/increment", "GET /assets/pglite",
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("scan did not find %s; found %v", w, sortedKeys(have))
		}
	}
}

// TestScanSourceJoinsAndLiterals proves the scanner on inline sources.
func TestScanSourceJoinsAndLiterals(t *testing.T) {
	src := `package x
import "net/http"
func a(pingURL string) { http.NewRequest(http.MethodPost, pingURL+"/license/x?y=1", nil) }
func b() { http.Get("https://ping.nself.org/health") }
func c() { _ = "Validate key against ping.nself.org" }
`
	eps, err := scanSource("x.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range eps {
		got[e.Method+" "+e.Path] = true
	}
	if len(got) != 2 || !got["POST /license/x"] || !got["GET /health"] {
		t.Fatalf("got %v", got)
	}
}
