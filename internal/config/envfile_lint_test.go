package config

// envfile_lint_test.go — keeps godotenv reads behind config.ReadEnvFile.
//
// Purpose: godotenv parse errors quote the file's text (a secret can be in it),
//          so every read goes through ReadEnvFile / ParseEnvBytes (D-0444,
//          D-0558). This test fails when a non-test file calls a godotenv
//          reader anywhere else, and when an allowlist entry goes stale.
// Inputs:  the module's .go source (parsed, not grepped, so comments and import
//          aliases are handled).
// Outputs: test failures naming file:line. Constraints: read only.

import (
	"go/ast"
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

const godotenvImportPath = "github.com/joho/godotenv"

// godotenvReaders are the godotenv functions that parse a dotenv file.
var godotenvReaders = map[string]bool{
	"Read": true, "Parse": true, "Load": true, "Overload": true,
	"Unmarshal": true, "UnmarshalBytes": true, "Exec": true,
}

// godotenvAllowed lists the files outside envfile.go that may still call
// godotenv, each because the parse error never reaches a user (verified when
// the list was written).
var godotenvAllowed = map[string]string{
	"internal/config/env_overlay.go":             "returns its own 'invalid dotenv syntax in <path>' text (D-0404)",
	"internal/reconcile/overlay_envline.go":      "writer round-trip probes; the errors are replaced by fixed text",
	"cmd/commands/deploy_remote.go":              "Overload with the error discarded",
	"cmd/commands/db_export_env.go":              "parse errors lead to a skip, never printed",
	"internal/doctor/report/inputs.go":           "uses the map only when err == nil",
	"internal/backup/stream_schedule_project.go": "uses the map only when err == nil",
}

// godotenvCalls returns "Sel:line" for every call-site selector on the godotenv
// import in src, honouring an import alias.
func godotenvCalls(filename string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	local := ""
	for _, imp := range f.Imports {
		p, uerr := strconv.Unquote(imp.Path.Value)
		if uerr != nil || p != godotenvImportPath {
			continue
		}
		local = "godotenv"
		if imp.Name != nil {
			local = imp.Name.Name
		}
		if local == "." {
			// A dot import hides the package name, so calls cannot be told
			// apart from local ones: report the import itself.
			return []string{"dot-import:" + strconv.Itoa(fset.Position(imp.Pos()).Line)}, nil
		}
	}
	if local == "" || local == "_" {
		return nil, nil
	}
	var hits []string
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == local && godotenvReaders[sel.Sel.Name] {
			hits = append(hits, sel.Sel.Name+":"+strconv.Itoa(fset.Position(sel.Pos()).Line))
		}
		return true
	})
	return hits, nil
}

// TestDetectorPositiveControl proves the detector sees an aliased call and
// ignores a comment: a lint that cannot fail is not a lint.
func TestDetectorPositiveControl(t *testing.T) {
	hit := "package p\nimport gd \"github.com/joho/godotenv\"\nfunc f() { gd.Read(\"x\") }\n"
	got, err := godotenvCalls("hit.go", []byte(hit))
	if err != nil || len(got) != 1 || !strings.HasPrefix(got[0], "Read:") {
		t.Fatalf("aliased godotenv.Read not detected: %v %v", got, err)
	}
	comment := "package p\n// godotenv.Read(\"x\") is mentioned here only\nfunc f() {}\n"
	got, err = godotenvCalls("comment.go", []byte(comment))
	if err != nil || len(got) != 0 {
		t.Fatalf("comment mention must not count: %v %v", got, err)
	}
	other := "package p\nimport \"example.com/other\"\nfunc f() { other.Read(\"x\") }\n"
	if got, _ = godotenvCalls("other.go", []byte(other)); len(got) != 0 {
		t.Fatalf("another package's Read must not count: %v", got)
	}
	dot := "package p\nimport . \"github.com/joho/godotenv\"\nfunc f() { Read(\"x\") }\n"
	if got, _ = godotenvCalls("dot.go", []byte(dot)); len(got) != 1 || !strings.HasPrefix(got[0], "dot-import:") {
		t.Fatalf("a dot import of godotenv must be reported: %v", got)
	}
}

// TestNoDirectGodotenvReads walks the module and enforces the allowlist.
func TestNoDirectGodotenvReads(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	skipDir := map[string]bool{"vendor": true, ".git": true, ".claude": true, "node_modules": true, "testdata": true}
	seen := map[string]bool{}
	werr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir[d.Name()] && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if !strings.Contains(string(src), godotenvImportPath) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		hits, perr := godotenvCalls(path, src)
		if perr != nil {
			t.Errorf("%s: cannot parse: %v", rel, perr)
			return nil
		}
		if len(hits) > 0 {
			seen[rel] = true
		}
		_, allowed := godotenvAllowed[rel]
		if len(hits) > 0 && !allowed && rel != "internal/config/envfile.go" {
			t.Errorf("%s calls godotenv directly (%s): use config.ReadEnvFile / config.ParseEnvBytes so a parse error never quotes the file", rel, strings.Join(hits, ", "))
		}
		return nil
	})
	if werr != nil {
		t.Fatal(werr)
	}
	var stale []string
	for rel := range godotenvAllowed {
		if !seen[rel] {
			stale = append(stale, rel)
		}
	}
	sort.Strings(stale)
	for _, rel := range stale {
		t.Errorf("allowlist entry %s no longer calls godotenv: remove it from godotenvAllowed", rel)
	}
	if !seen["internal/config/envfile.go"] {
		t.Errorf("internal/config/envfile.go was not seen calling godotenv: the walk is broken")
	}
}
