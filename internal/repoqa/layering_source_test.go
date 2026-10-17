package repoqa

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type sourceImport struct {
	File, From, To string
}

// sourceBasis freezes pre-existing test violations discovered by D-0256.
// Exact file/import pairs are shrink-only; stale exemptions fail the guard.
var sourceBasis = map[sourceImport]bool{
	{"internal/config/functions_runtime_test.go", "internal/config", "internal/compose"}: true,
	{"internal/docker/pull_fallback_test.go", "internal/docker", "internal/compose"}:     true,
	{"internal/mcpgen/main_test.go", "internal/mcpgen", "cmd/commands"}:                  true,
}

// sourceImports reads imports without applying build constraints, so D1/D2
// cover every OS, architecture, custom tag and internal/external test file.
// Testdata, vendor, hidden/underscore directories and nested modules are not
// first-party packages in this module and are excluded like Go's ./... walk.
func sourceImports(root, module string) ([]sourceImport, error) {
	var out []sourceImport
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root {
				name := d.Name()
				if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
					return filepath.SkipDir
				}
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				} else if !os.IsNotExist(err) {
					return err
				}
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse imports %s: %w", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		from := filepath.ToSlash(filepath.Dir(rel))
		for _, imp := range file.Imports {
			value, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return fmt.Errorf("import in %s: %w", path, err)
			}
			to, ok := strings.CutPrefix(value, module+"/")
			if value == module {
				to, ok = ".", true
			}
			if ok && from != to {
				out = append(out, sourceImport{filepath.ToSlash(rel), from, to})
			}
		}
		return nil
	})
	return out, err
}

func checkSourceLayering(imports []sourceImport, spec layersSpec, basis map[sourceImport]bool) []string {
	var out []string
	seen := map[sourceImport]bool{}
	for _, imp := range imports {
		from, _ := spec.classify(imp.From)
		to, _ := spec.classify(imp.To)
		if !(strings.HasPrefix(imp.From, "internal/") && isCmd(imp.To)) && !(from == layerL0 && to != layerL0) {
			continue
		}
		seen[imp] = true
		if basis[imp] {
			continue
		}
		switch {
		case strings.HasPrefix(imp.From, "internal/") && isCmd(imp.To):
			out = append(out, fmt.Sprintf("D2: %s imports %s in %s: internal/ must not import cmd/", imp.From, imp.To, imp.File))
		case from == layerL0 && to != layerL0:
			out = append(out, fmt.Sprintf("D1: %s imports %s in %s: L0 may import only L0", imp.From, imp.To, imp.File))
		}
	}
	for imp := range basis {
		if !seen[imp] {
			out = append(out, fmt.Sprintf("sourceBasis: %+v is no longer a violation; remove the stale exemption", imp))
		}
	}
	return out
}
