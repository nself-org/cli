package repoqa

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stdoutBasisTotal pins the tight allowlist total. The list and this constant
// only lower as domain writers move behind the output seam.
const stdoutBasisTotal = 75

// countStdoutWriters counts direct os.Stdout references and fmt.Print* calls.
func countStdoutWriters(rel string, src []byte) (int, error) {
	f, err := parser.ParseFile(token.NewFileSet(), rel, src, parser.SkipObjectResolution)
	if err != nil {
		return 0, err
	}
	osName, fmtName := importName(f, "os"), importName(f, "fmt")
	n := 0
	ast.Inspect(f, func(node ast.Node) bool {
		switch x := node.(type) {
		case *ast.SelectorExpr:
			if osName != "" && isSelector(x, osName, "Stdout") {
				n++
			}
		case *ast.CallExpr:
			for _, name := range []string{"Print", "Printf", "Println"} {
				if fmtName != "" && isSelector(x.Fun, fmtName, name) {
					n++
				}
			}
		}
		return true
	})
	return n, nil
}

// scanStdoutWriters walks non-test Go source outside presentation and command
// packages. It parses all build-tag and platform variants without compiling.
func scanStdoutWriters(root string) (countList, error) {
	actual := countList{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			name := d.Name()
			if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if !underDir(rel, "internal") {
				return filepath.SkipDir
			}
			if underDir(rel, "internal/ui") || underDir(rel, "internal/output") || underDir(rel, "internal/repoqa") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		n, err := countStdoutWriters(rel, src)
		if err != nil {
			return err
		}
		if n > 0 {
			actual[rel] = n
		}
		return nil
	})
	return actual, err
}

// TestStdoutWriters keeps the current domain stdout sites tight and shrink-only.
func TestStdoutWriters(t *testing.T) {
	root := repoRoot(t)
	actual, err := scanStdoutWriters(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "internal/repoqa/testdata/stdout-writers-allowlist.txt")
	list := readCountList(t, path)
	if *update {
		writeCountListDown(t, path, list, actual)
		list = readCountList(t, path)
	}
	findings := compareCountList("stdout", list, actual)
	total := countTotal(list)
	if total > stdoutBasisTotal {
		findings = append(findings, fmt.Sprintf("stdout: allowlist total %d exceeds stdoutBasisTotal %d", total, stdoutBasisTotal))
	}
	if total < stdoutBasisTotal && !*update {
		findings = append(findings, fmt.Sprintf("stdout: lower stdoutBasisTotal to %d", total))
	}
	if len(findings) > 0 {
		t.Fatalf("stdout writer ratchet:\n%s", strings.Join(findings, "\n"))
	}
}
