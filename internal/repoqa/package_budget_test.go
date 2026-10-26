package repoqa

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Package budget (D-0033): the per-file 300-line cap cannot see a package that
// stays under it file by file while growing without bound, or one that every
// other package leans on. Two ratchets close that gap.
//
// Size:   a package whose non-test Go code exceeds pkgLOCThreshold lines must
//
//	have a line in testdata/package-size-allowlist.txt ("<dir> <lines>");
//	it may not grow past that ceiling. A new package that crosses the
//	threshold is refused until it is split or a reviewer adds a line.
//
// Fan-in: a package imported by more than pkgFanInThreshold other packages of
//
//	the module must have a line in testdata/package-fanin-allowlist.txt
//	("<dir> <importers>"); the importer count may not grow past it.
//
// Both lists are ceilings, not exact counts: a package that shrinks passes
// without an edit, and `go test ./internal/repoqa -update` lowers the line (or
// drops it once the package is under the threshold). The update only ever
// lowers; it never adds a line or raises a ceiling. The header of each list
// says how to read it.
//
// Scope: every non-_test.go .go file of the root module, whatever its build
// tags. Skipped: vendor/, testdata/, dot-dirs and nested modules (the same
// rules as the funnel scan). Fan-in counts distinct importing packages, from
// non-test files only; a test-only import does not add coupling to the binary.
const (
	pkgLOCThreshold   = 6000
	pkgFanInThreshold = 20

	pkgSizeList  = "testdata/package-size-allowlist.txt"
	pkgFanInList = "testdata/package-fanin-allowlist.txt"
)

// pkgMetrics are the measured numbers of one package directory.
type pkgMetrics struct {
	loc       int
	importers map[string]bool
}

// scanPackages walks the module rooted at root and returns the metrics of
// every package directory, keyed by module-relative slash path ("." is the
// module root). module is the module path from go.mod.
func scanPackages(root, module string) (map[string]*pkgMetrics, error) {
	pkgs := map[string]*pkgMetrics{}
	get := func(dir string) *pkgMetrics {
		m, ok := pkgs[dir]
		if !ok {
			m = &pkgMetrics{importers: map[string]bool{}}
			pkgs[dir] = m
		}
		return m
	}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			if p == root {
				return nil
			}
			name := d.Name()
			if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if _, serr := os.Stat(filepath.Join(p, "go.mod")); serr == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, filepath.Dir(p))
		if rerr != nil {
			return rerr
		}
		dir := filepath.ToSlash(rel)
		src, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		file, perr := parser.ParseFile(token.NewFileSet(), p, src, parser.ImportsOnly|parser.SkipObjectResolution)
		if perr != nil {
			return perr
		}
		get(dir).loc += bytes.Count(src, []byte("\n"))
		for _, imp := range file.Imports {
			ip, uerr := strconv.Unquote(imp.Path.Value)
			if uerr != nil || (ip != module && !strings.HasPrefix(ip, module+"/")) {
				continue
			}
			target := "."
			if ip != module {
				target = strings.TrimPrefix(ip, module+"/")
			}
			if target != dir {
				get(target).importers[dir] = true
			}
		}
		return nil
	})
	return pkgs, err
}

// overThreshold returns, for every package whose metric exceeds threshold, the
// measured value. Only packages that exist in pkgs and have code are counted;
// a directory that is only imported (no non-test source) has loc 0.
func overThreshold(pkgs map[string]*pkgMetrics, threshold int, metric func(*pkgMetrics) int) countList {
	out := countList{}
	for dir, m := range pkgs {
		if n := metric(m); n > threshold {
			out[dir] = n
		}
	}
	return out
}

// comparePackageBudget returns one finding per package that is over the
// threshold without a line, over its ceiling, or listed but no longer over the
// threshold. unit names the metric in the messages.
func comparePackageBudget(unit string, threshold int, list, over countList) []string {
	var out []string
	for _, dir := range sortedKeys(over) {
		n := over[dir]
		m, ok := list[dir]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("%s: %s has %d, over the %d threshold and not in the list; split it along domain lines", unit, dir, n, threshold))
		case n > m:
			out = append(out, fmt.Sprintf("%s: %s has %d, ceiling is %d; the package may not grow, move code out of it", unit, dir, n, m))
		}
	}
	for _, dir := range sortedKeys(list) {
		if _, ok := over[dir]; !ok {
			out = append(out, fmt.Sprintf("%s: %s is under the %d threshold; delete the line (go test ./internal/repoqa -update)", unit, dir, threshold))
		}
	}
	sort.Strings(out)
	return out
}

// runPackageRatchet is the shared body of the size and fan-in tests.
func runPackageRatchet(t *testing.T, unit, list string, threshold int, metric func(*pkgMetrics) int) {
	t.Helper()
	root := repoRoot(t)
	module, err := modulePath(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("module path: %v", err)
	}
	pkgs, err := scanPackages(root, module)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	over := overThreshold(pkgs, threshold, metric)
	listPath := filepath.Join(root, "internal", "repoqa", filepath.FromSlash(list))
	recorded := readCountList(t, listPath)
	if *update {
		// Lines whose package is no longer over the threshold are dropped, the
		// rest are lowered to the measured value, never raised.
		writeCountListDown(t, listPath, recorded, over)
		recorded = readCountList(t, listPath)
	}
	if findings := comparePackageBudget(unit, threshold, recorded, over); len(findings) > 0 {
		t.Fatalf("%s ratchet:\n%s", unit, strings.Join(findings, "\n"))
	}
}

// TestPackageSizeBudget refuses package growth past the recorded ceilings.
func TestPackageSizeBudget(t *testing.T) {
	runPackageRatchet(t, "package-size", pkgSizeList, pkgLOCThreshold, func(m *pkgMetrics) int { return m.loc })
}

// TestPackageFanInBudget refuses growth of the number of importing packages.
func TestPackageFanInBudget(t *testing.T) {
	runPackageRatchet(t, "package-fan-in", pkgFanInList, pkgFanInThreshold, func(m *pkgMetrics) int { return len(m.importers) })
}

// TestScanPackages pins what the scan counts: non-test lines, distinct
// importing packages, no self import, no external import, and the skipped dirs.
func TestScanPackages(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"internal/a/a.go":          "package a\n\nimport _ \"example.com/m/internal/b\"\n",
		"internal/a/a2.go":         "package a\n\nimport _ \"example.com/m/internal/b\"\nimport _ \"example.com/m/internal/a\"\n",
		"internal/a/a_test.go":     "package a\n\nimport _ \"example.com/m/internal/c\"\n",
		"internal/b/b.go":          "package b\n\nimport _ \"example.com/other/internal/c\"\n",
		"cmd/x/x.go":               "package x\n\nimport _ \"example.com/m/internal/b\"\nimport _ \"example.com/m\"\n",
		"main.go":                  "package main\n",
		"vendor/v/v.go":            "package v\n\nimport _ \"example.com/m/internal/b\"\n",
		"internal/b/testdata/t.go": "package t\n\nimport _ \"example.com/m/internal/b\"\n",
		"nested/go.mod":            "module nested\n",
		"nested/n.go":              "package n\n\nimport _ \"example.com/m/internal/b\"\n",
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
	pkgs, err := scanPackages(root, "example.com/m")
	if err != nil {
		t.Fatal(err)
	}
	if got := pkgs["internal/a"].loc; got != 7 {
		t.Errorf("internal/a loc = %d, want 7 (test file excluded)", got)
	}
	var b []string
	for imp := range pkgs["internal/b"].importers {
		b = append(b, imp)
	}
	sort.Strings(b)
	if want := "cmd/x,internal/a"; strings.Join(b, ",") != want {
		t.Errorf("internal/b importers = %v, want %s (vendor, testdata, nested module skipped)", b, want)
	}
	if got := len(pkgs["internal/a"].importers); got != 0 {
		t.Errorf("internal/a importers = %d, want 0 (self import and test import ignored)", got)
	}
	if got := pkgs["."].importers; !got["cmd/x"] || len(got) != 1 {
		t.Errorf("module root importers = %v, want cmd/x", got)
	}
	if _, ok := pkgs["internal/c"]; ok {
		t.Error("internal/c is only imported from a test file and an external module path; it must not appear")
	}
}

// TestComparePackageBudget covers the three findings and the passing shapes.
func TestComparePackageBudget(t *testing.T) {
	list := countList{"p/above": 100, "p/shrunk": 100, "p/stale": 50, "p/same": 80}
	over := countList{"p/above": 101, "p/shrunk": 90, "p/same": 80, "p/new": 70}
	got := comparePackageBudget("package-size", 60, list, over)
	want := []string{
		"package-size: p/above has 101, ceiling is 100; the package may not grow, move code out of it",
		"package-size: p/new has 70, over the 60 threshold and not in the list; split it along domain lines",
		"package-size: p/stale is under the 60 threshold; delete the line (go test ./internal/repoqa -update)",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if extra := comparePackageBudget("x", 1, countList{"a": 2}, countList{"a": 2}); len(extra) != 0 {
		t.Fatalf("clean list produced findings: %v", extra)
	}
}
