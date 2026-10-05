package repoqa

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// pkgInfo is what the guards need to know about one package.
type pkgInfo struct {
	Name string
}

// graph is the module's import graph, module-relative ("internal/build").
//
// Purpose:     the one data structure the layering (D1-D3) and orphan (D9)
//
//	rules read; synthetic graphs in tests have the same shape.
//
// Fields:      pkgs, every package in the module; imports, importer -> imported
//
//	(non-test imports, module packages only); testImporters, imported ->
//	importing packages that use it from test files (other packages only).
type graph struct {
	pkgs          map[string]pkgInfo
	imports       map[string][]string
	testImporters map[string][]string
}

var (
	graphOnce sync.Once
	graphVal  graph
	graphErr  error
)

// loadGraph returns the real module graph. One `go list` call per test binary
// (EPIC G3): fixed GOOS/GOARCH, vendor mode, so the result does not depend on
// the host. A missing `go` skips locally and fails in CI (requireTool).
func loadGraph(t *testing.T) graph {
	t.Helper()
	goBin := requireTool(t, "go")
	root := repoRoot(t)
	graphOnce.Do(func() { graphVal, graphErr = runGoList(goBin, root) })
	if graphErr != nil {
		t.Fatalf("load import graph: %v", graphErr)
	}
	return graphVal
}

func runGoList(goBin, root string) (graph, error) {
	module, err := modulePath(filepath.Join(root, "go.mod"))
	if err != nil {
		return graph{}, err
	}
	cmd := exec.Command(goBin, "list", "-mod=vendor", "-e",
		"-json=ImportPath,Name,Imports,TestImports,XTestImports,Error", "./...")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0", "GOFLAGS=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return graph{}, fmt.Errorf("go list: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return decodeGoList(bytes.NewReader(out), module)
}

type listedPkg struct {
	ImportPath   string
	Name         string
	Imports      []string
	TestImports  []string
	XTestImports []string
	Error        *struct{ Err string }
}

// decodeGoList builds a graph from a `go list -json` stream. Any package with
// an Error fails the load: a broken package would hide its edges.
func decodeGoList(r io.Reader, module string) (graph, error) {
	rel := func(p string) (string, bool) {
		switch {
		case p == module:
			return ".", true
		case strings.HasPrefix(p, module+"/"):
			return strings.TrimPrefix(p, module+"/"), true
		}
		return "", false
	}
	g := graph{pkgs: map[string]pkgInfo{}, imports: map[string][]string{}, testImporters: map[string][]string{}}
	dec := json.NewDecoder(r)
	for {
		var p listedPkg
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			return graph{}, fmt.Errorf("decode go list output: %v", err)
		}
		if p.Error != nil {
			return graph{}, fmt.Errorf("package %s: %s", p.ImportPath, strings.TrimSpace(p.Error.Err))
		}
		self, ok := rel(p.ImportPath)
		if !ok {
			continue
		}
		g.pkgs[self] = pkgInfo{Name: p.Name}
		for _, imp := range p.Imports {
			if to, ok := rel(imp); ok && to != self {
				g.imports[self] = append(g.imports[self], to)
			}
		}
		seen := map[string]bool{}
		for _, imp := range append(append([]string{}, p.TestImports...), p.XTestImports...) {
			if to, ok := rel(imp); ok && to != self && !seen[to] {
				seen[to] = true
				g.testImporters[to] = append(g.testImporters[to], self)
			}
		}
	}
	if len(g.pkgs) == 0 {
		return graph{}, fmt.Errorf("go list returned no packages for module %s", module)
	}
	for k := range g.imports {
		sort.Strings(g.imports[k])
	}
	return g, nil
}

// modulePath reads the module path from a go.mod file.
func modulePath(goMod string) (string, error) {
	f, err := os.Open(goMod)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), "\""), nil
		}
	}
	return "", fmt.Errorf("%s has no module line", goMod)
}

// importersOf returns, for each package, the other packages that import it
// from non-test files.
func (g graph) importersOf() map[string][]string {
	out := map[string][]string{}
	for from, tos := range g.imports {
		for _, to := range tos {
			out[to] = append(out[to], from)
		}
	}
	return out
}

// TestLayeringGraphDecode covers the loader's decoding without running go:
// module-relative names, test importers from other packages only, and a
// package Error failing the load.
func TestLayeringGraphDecode(t *testing.T) {
	const m = "example.com/m"
	stream := `{"ImportPath":"example.com/m/internal/a","Name":"a","Imports":["fmt","example.com/m/internal/b","example.com/other/x"],"XTestImports":["example.com/m/internal/a","example.com/m/internal/c"]}
{"ImportPath":"example.com/m/internal/b","Name":"b"}
{"ImportPath":"example.com/m/internal/c","Name":"c","TestImports":["example.com/m/internal/b"]}`
	g, err := decodeGoList(strings.NewReader(stream), m)
	if err != nil {
		t.Fatal(err)
	}
	if got := g.imports["internal/a"]; len(got) != 1 || got[0] != "internal/b" {
		t.Fatalf("imports[a] = %v, want [internal/b]", got)
	}
	if got := g.testImporters["internal/c"]; len(got) != 1 || got[0] != "internal/a" {
		t.Fatalf("testImporters[c] = %v, want [internal/a]", got)
	}
	if _, self := g.testImporters["internal/a"]; self {
		t.Fatal("a package must not count as its own test importer")
	}
	if g.pkgs["internal/b"].Name != "b" || len(g.pkgs) != 3 {
		t.Fatalf("pkgs = %v", g.pkgs)
	}
	bad := `{"ImportPath":"example.com/m/internal/z","Error":{"Err":"boom"}}`
	if _, err := decodeGoList(strings.NewReader(bad), m); err == nil || !strings.Contains(err.Error(), "internal/z") {
		t.Fatalf("package Error must fail the load naming the package, got %v", err)
	}
	if _, err := decodeGoList(strings.NewReader(""), m); err == nil {
		t.Fatal("an empty go list must fail, not pass vacuously")
	}
}
