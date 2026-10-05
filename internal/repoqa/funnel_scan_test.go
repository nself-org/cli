package repoqa

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Funnel scan (rules D4 and D5, EPIC G4): an AST count of direct http.Client
// construction and direct docker process execution.
//
// Purpose:     feeds TestHTTPFunnel and TestDockerFunnel; the scan parses files
//
//	and never builds or runs them.
//
// Scope:       every non-_test.go .go file of the root module, whatever its
//
//	build tags or GOOS suffix (the parser ignores tags). Test files are
//	NOT scanned: a site in a _test.go file is out of scope (D-0256 blind
//	spot, stated in Architecture-Guardrails.md#funnels). Skipped: vendor/,
//	testdata/, dot-dirs and any directory with its own go.mod.
//
// Exempt:      internal/httptimeout/ (http) and internal/docker/ (docker).
// Blind spot:  a binary path obtained from exec.LookPath("docker").
const (
	httpFunnelDir   = "internal/httptimeout"
	dockerFunnelDir = "internal/docker"
)

// scanSource counts the funnel sites in one source file. filename is the
// module-relative path (any separator); an exempt directory yields no sites.
func scanSource(filename string, src []byte) (httpSites, dockerSites int, err error) {
	rel := strings.ReplaceAll(filepath.ToSlash(filename), `\`, "/") // ToSlash is a no-op off Windows; accept both
	file, err := parser.ParseFile(token.NewFileSet(), rel, src, parser.SkipObjectResolution)
	if err != nil {
		return 0, 0, err
	}
	httpName, execName := importName(file, "net/http"), importName(file, "os/exec")
	exemptHTTP := underDir(rel, httpFunnelDir)
	exemptDocker := underDir(rel, dockerFunnelDir)
	dockerIdents := dockerBoundIdents(file)

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CompositeLit:
			if httpName != "" && !exemptHTTP && isSelector(x.Type, httpName, "Client") {
				httpSites++
			}
		case *ast.CallExpr:
			if httpName != "" && !exemptHTTP && isBuiltinNewOf(x, httpName, "Client") {
				httpSites++
			}
			if execName != "" && !exemptDocker && isDockerExec(x, execName, dockerIdents) {
				dockerSites++
			}
		}
		return true
	})
	return httpSites, dockerSites, nil
}

// importName returns the local name of an import path, or "" when the file does
// not import it (or imports it with `.` or `_`).
func importName(f *ast.File, path string) string {
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != path {
			continue
		}
		if imp.Name == nil {
			return filepath.Base(path)
		}
		if imp.Name.Name == "." || imp.Name.Name == "_" {
			return ""
		}
		return imp.Name.Name
	}
	return ""
}

// dockerBoundIdents returns the const/var identifiers declared in the file
// with the string literal "docker" as their value.
func dockerBoundIdents(f *ast.File) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || len(vs.Names) != len(vs.Values) {
			return true
		}
		for i, v := range vs.Values {
			if isDockerLit(v) {
				out[vs.Names[i].Name] = true
			}
		}
		return true
	})
	return out
}

func isDockerLit(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	s, err := strconv.Unquote(lit.Value)
	return err == nil && s == "docker"
}

func isSelector(e ast.Expr, pkg, name string) bool {
	if star, ok := e.(*ast.StarExpr); ok {
		e = star.X
	}
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg
}

func isBuiltinNewOf(c *ast.CallExpr, pkg, name string) bool {
	id, ok := c.Fun.(*ast.Ident)
	return ok && id.Name == "new" && len(c.Args) == 1 && isSelector(c.Args[0], pkg, name)
}

// isDockerExec matches exec.Command("docker", ...) and
// exec.CommandContext(ctx, "docker", ...), where the command is the literal or
// an identifier bound to it in the same file.
func isDockerExec(c *ast.CallExpr, execName string, bound map[string]bool) bool {
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if id, ok := sel.X.(*ast.Ident); !ok || id.Name != execName {
		return false
	}
	idx := -1
	switch sel.Sel.Name {
	case "Command":
		idx = 0
	case "CommandContext":
		idx = 1
	}
	if idx < 0 || len(c.Args) <= idx {
		return false
	}
	arg := c.Args[idx]
	if isDockerLit(arg) {
		return true
	}
	id, ok := arg.(*ast.Ident)
	return ok && bound[id.Name]
}

func underDir(rel, dir string) bool { return rel == dir || strings.HasPrefix(rel, dir+"/") }

// scanFunnels walks the root module and returns the per-file site counts.
func scanFunnels(root string) (httpSites, dockerSites countList, err error) {
	httpSites, dockerSites = countList{}, countList{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			name := d.Name()
			if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if _, serr := os.Stat(filepath.Join(path, "go.mod")); serr == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		h, dk, perr := scanSource(rel, src)
		if perr != nil {
			return perr
		}
		if h > 0 {
			httpSites[rel] = h
		}
		if dk > 0 {
			dockerSites[rel] = dk
		}
		return nil
	})
	return httpSites, dockerSites, err
}

// TestFunnelScan pins every pattern the scan counts and the ones it must not.
func TestFunnelScan(t *testing.T) {
	const (
		hdr    = "package x\n\nimport (\n\t\"context\"\n\t\"net/http\"\n\t\"os/exec\"\n)\n\nvar _ = context.Background\nvar _ = http.StatusOK\nvar _ = exec.ErrNotFound\n"
		hdrAlt = "package x\n\nimport (\n\t\"context\"\n\tnh \"net/http\"\n\tosexec \"os/exec\"\n)\n\nvar _ = context.Background\n"
	)
	cases := []struct {
		name         string
		file, src    string
		wantH, wantD int
	}{
		{"address-of literal", "a.go", hdr + "var c = &http.Client{}\n", 1, 0},
		{"literal with field", "a.go", hdr + "var c = http.Client{Timeout: 5}\n", 1, 0},
		{"new", "a.go", hdr + "var c = new(http.Client)\n", 1, 0},
		{"aliased net/http", "a.go", hdrAlt + "var c = nh.Client{}\n", 1, 0},
		{"exec.Command docker", "a.go", hdr + "var c = exec.Command(\"docker\", \"ps\")\n", 0, 1},
		{"CommandContext docker", "a.go", hdr + "func f(ctx context.Context) { _ = exec.CommandContext(ctx, \"docker\", \"ps\") }\n", 0, 1},
		{"const bound", "a.go", hdr + "const bin = \"docker\"\n\nfunc f() { _ = exec.Command(bin) }\n", 0, 1},
		{"var bound", "a.go", hdr + "var bin = \"docker\"\n\nfunc f(ctx context.Context) { _ = exec.CommandContext(ctx, bin) }\n", 0, 1},
		{"aliased os/exec", "a.go", hdrAlt + "func f() { _ = osexec.Command(\"docker\") }\n", 0, 1},
		{"comment", "a.go", hdr + "// http.Client{} and exec.Command(\"docker\") in a comment\n", 0, 0},
		{"string", "a.go", hdr + "var s = \"http.Client{} exec.Command(\\\"docker\\\")\"\n", 0, 0},
		{"type position", "a.go", hdr + "func f(c *http.Client) *http.Client { var d http.Client; _ = d; return c }\n", 0, 0},
		{"git is not docker", "a.go", hdr + "var c = exec.Command(\"git\", \"status\")\n", 0, 0},
		{"unbound identifier", "a.go", hdr + "func f(bin string) { _ = exec.Command(bin) }\n", 0, 0},
		{"unaliased name not imported", "a.go", "package x\n\nimport nh \"net/http\"\n\nvar _ = nh.StatusOK\nvar c = http.Client{}\n", 0, 0},
		{"two sites in one file", "a.go", hdr + "var a, b = &http.Client{}, &http.Client{}\n", 2, 0},
		{"windows path exempt http", `internal\httptimeout\x.go`, hdr + "var c = &http.Client{}\n", 0, 0},
		{"windows path exempt docker", `internal\docker\x.go`, hdr + "var c = exec.Command(\"docker\")\n", 0, 0},
		{"docker dir does not exempt http", "internal/docker/x.go", hdr + "var c = &http.Client{}\n", 1, 0},
		{"similar dir not exempt", "internal/httptimeoutx/x.go", hdr + "var c = &http.Client{}\n", 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, d, err := scanSource(tc.file, []byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			if h != tc.wantH || d != tc.wantD {
				t.Fatalf("http=%d docker=%d, want http=%d docker=%d", h, d, tc.wantH, tc.wantD)
			}
		})
	}
}

// TestFunnelScanSkipsTestFilesAndDirs proves the walk skips _test.go files,
// vendor/, testdata/, dot-dirs and nested modules, and counts all build tags.
func TestFunnelScanSkipsTestFilesAndDirs(t *testing.T) {
	root := t.TempDir()
	const src = "package x\n\nimport \"net/http\"\n\nvar c = &http.Client{}\n"
	files := map[string]string{
		"a/counted.go":              src,
		"a/tagged_windows.go":       "//go:build windows\n\n" + src,
		"a/skip_test.go":            src,
		"vendor/v/v.go":             src,
		"a/testdata/t.go":           src,
		".hidden/h.go":              src,
		"nested/go.mod":             "module nested\n",
		"nested/n.go":               src,
		"internal/httptimeout/f.go": src,
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h, d, err := scanFunnels(root)
	if err != nil {
		t.Fatal(err)
	}
	want := countList{"a/counted.go": 1, "a/tagged_windows.go": 1}
	if len(h) != len(want) || h["a/counted.go"] != 1 || h["a/tagged_windows.go"] != 1 || len(d) != 0 {
		t.Fatalf("http=%v docker=%v, want http=%v", h, d, want)
	}
}

// funnelSpec describes one shrink-only funnel ratchet.
type funnelSpec struct {
	kind, funnel, list   string
	basisName, basisFile string
	basis                int
	pick                 func(httpSites, dockerSites countList) countList
}

// runFunnelRatchet compares the scan with the allowlist (tight, both ways) and
// pins the list total to a hard-coded basis, so the list cannot be loosened
// without a visible edit of the test. -update only lowers lines.
func runFunnelRatchet(t *testing.T, s funnelSpec) {
	t.Helper()
	root := repoRoot(t)
	h, d, err := scanFunnels(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	actual := s.pick(h, d)
	path := filepath.Join(root, "internal", "repoqa", filepath.FromSlash(s.list))
	list := readCountList(t, path)
	if *update {
		writeCountListDown(t, path, list, actual)
		list = readCountList(t, path)
		t.Logf("set %s in %s to %d", s.basisName, s.basisFile, countTotal(list))
	}
	var findings []string
	findings = append(findings, compareCountList(s.funnel, list, actual)...)
	total := countTotal(list)
	switch {
	case total > s.basis:
		findings = append(findings, fmt.Sprintf("%s: allowlist total %d is above %s %d; the list may only shrink, new sites are rejected", s.funnel, total, s.basisName, s.basis))
	case total < s.basis && !*update:
		findings = append(findings, fmt.Sprintf("%s: lower %s in %s to %d", s.funnel, s.basisName, s.basisFile, total))
	}
	if len(findings) > 0 {
		t.Fatalf("%s funnel ratchet:\n%s", s.kind, strings.Join(findings, "\n"))
	}
}

func countTotal(l countList) int {
	n := 0
	for _, c := range l {
		n += c
	}
	return n
}
