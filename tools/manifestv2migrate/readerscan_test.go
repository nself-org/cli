package main

import (
	"archive/tar"
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

// The reader scan (Epic D1, review F1): every top-level plugin.json key that a
// released v1.4.12 reader decodes must stay readable in a v2 file, either as a
// shared key (same name and type) or as a generated compatibility key.
//
// The scan extracts the v1.4.12 tag (git archive), parses it with go/ast and
// finds every struct decoded by json.Unmarshal or a json Decoder in a file that
// names "plugin.json" or defines parseManifest. testdata/v1412-readers.txt is
// its committed output: the test fails when a reader appears or disappears.
// NSELF_UPDATE_READERS=1 rewrites the file.

const readerTag = "v1.4.12"

// registryOnly are keys PluginManifest declares for registry entries, never for
// plugin.json: D1 forbids them in a v2 file (E111).
var registryOnly = map[string]bool{"bundles": true, "tier_pair": true, "author_public_key": true, "signature": true, "checksum": true}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for d := dir; d != filepath.Dir(d); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
	}
	t.Fatal("go.mod not found")
	return ""
}

func extractTag(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", root, "archive", "--format=tar", readerTag, "internal", "cmd")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("git archive %s failed (run `git fetch --tags origin`): %v", readerTag, err)
	}
	dst := t.TempDir()
	tr := tar.NewReader(&out)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag != tar.TypeReg || !strings.HasSuffix(h.Name, ".go") || strings.HasSuffix(h.Name, "_test.go") {
			continue
		}
		p := filepath.Join(dst, filepath.FromSlash(h.Name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

type field struct{ key, typ string }

// readerFields returns reader name -> its top-level json fields, for the tree at dir.
func readerFields(t *testing.T, dir string) map[string][]field {
	t.Helper()
	fset := token.NewFileSet()
	structs := map[string]map[string]*ast.StructType{} // package dir -> type name -> struct
	files := map[string]*ast.File{}
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		f, perr := parser.ParseFile(fset, p, nil, 0)
		if perr != nil {
			t.Fatalf("%s: %v", p, perr)
		}
		files[p] = f
		pkg := filepath.Dir(p)
		if structs[pkg] == nil {
			structs[pkg] = map[string]*ast.StructType{}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok {
				if st, ok := ts.Type.(*ast.StructType); ok {
					structs[pkg][ts.Name.Name] = st
				}
			}
			return true
		})
		return nil
	})
	out := map[string][]field{}
	for p, f := range files {
		src, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if !bytes.Contains(src, []byte(`"plugin.json"`)) && !bytes.Contains(src, []byte("func parseManifest(")) {
			continue
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			decls := localVars(fn)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !isDecode(call) {
					return true
				}
				id := decodeTarget(call)
				if id == "" || decls[id] == nil {
					return true
				}
				name, st := resolve(decls[id], structs[filepath.Dir(p)])
				if st != nil {
					out[rel+" "+name] = fieldsOf(st)
				}
				return true
			})
		}
	}
	return out
}

func isDecode(c *ast.CallExpr) bool {
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if x, ok := sel.X.(*ast.Ident); ok && x.Name == "json" && sel.Sel.Name == "Unmarshal" {
		return true
	}
	return sel.Sel.Name == "Decode" && len(c.Args) == 1
}

func decodeTarget(c *ast.CallExpr) string {
	arg := c.Args[len(c.Args)-1]
	if u, ok := arg.(*ast.UnaryExpr); ok {
		arg = u.X
	}
	if id, ok := arg.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// localVars maps `var x T` names in fn to their type expression.
func localVars(fn *ast.FuncDecl) map[string]ast.Expr {
	out := map[string]ast.Expr{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if vs, ok := n.(*ast.ValueSpec); ok && vs.Type != nil {
			for _, nm := range vs.Names {
				out[nm.Name] = vs.Type
			}
		}
		return true
	})
	return out
}

func resolve(e ast.Expr, named map[string]*ast.StructType) (string, *ast.StructType) {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name, named[v.Name]
	case *ast.StructType:
		return "(inline)", v
	}
	return "", nil
}

func fieldsOf(st *ast.StructType) []field {
	var out []field
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 || !f.Names[0].IsExported() {
			continue
		}
		key := strings.ToLower(f.Names[0].Name)
		if f.Tag != nil {
			tag := reflect.StructTag(strings.Trim(f.Tag.Value, "`")).Get("json")
			if tag == "-" {
				continue
			}
			if n := strings.Split(tag, ",")[0]; n != "" {
				key = n
			}
		}
		out = append(out, field{key, exprText(f.Type)})
	}
	return out
}

func exprText(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return "*" + exprText(v.X)
	case *ast.ArrayType:
		return "[]" + exprText(v.Elt)
	case *ast.MapType:
		return "map[" + exprText(v.Key) + "]" + exprText(v.Value)
	case *ast.SelectorExpr:
		return exprText(v.X) + "." + v.Sel.Name
	}
	return "?"
}

// renderReaders is the committed form of the scan.
func renderReaders(readers map[string][]field) string {
	byKey := map[string][]string{}
	var names []string
	for r, fs := range readers {
		names = append(names, r)
		short := r[strings.LastIndex(r, " ")+1:]
		for _, f := range fs {
			byKey[f.key] = append(byKey[f.key], short)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("// GENERATED BY tools/manifestv2migrate TestReaderScan from cli tag " + readerTag + " - DO NOT HAND EDIT\n")
	b.WriteString("// NSELF_UPDATE_READERS=1 go test ./tools/manifestv2migrate -run TestReaderScan rewrites this file.\n")
	for _, n := range names {
		b.WriteString("reader " + n + "\n")
	}
	var keys []string
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sort.Strings(byKey[k])
		var uniq []string
		for i, r := range byKey[k] {
			if i == 0 || r != byKey[k][i-1] {
				uniq = append(uniq, r)
			}
		}
		b.WriteString("key " + k + " " + strings.Join(uniq, ",") + "\n")
	}
	return b.String()
}

func TestReaderScan(t *testing.T) {
	root := repoRoot(t)
	readers := readerFields(t, extractTag(t, root))
	if len(readers) < 5 {
		t.Fatalf("reader scan found only %d readers; the scan is broken", len(readers))
	}
	got := renderReaders(readers)
	path := filepath.Join("testdata", "v1412-readers.txt")
	if os.Getenv("NSELF_UPDATE_READERS") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatalf("testdata/v1412-readers.txt is stale: a v1.4.12 reader was added, missed or changed.\n--- committed\n%s\n--- scan\n%s", want, got)
	}
	checkCoverage(t, readers)
}

// checkCoverage: every key is a shared v2 key (same name and simple type), a
// compatibility key, or a registry-only key that no plugin.json-only reader
// decodes.
func checkCoverage(t *testing.T, readers map[string][]field) {
	shared := jsonFields(reflect.TypeOf(manifestv2.Manifest{}))
	polymorphic := map[string]bool{"apiEndpoints": true, "webhooks": true, "envVars": true, "dependencies": true, "permissions": true,
		"systemDependencies": true, "multiApp": true, "compat": true, "deprecation": true, "graphql": true, "cliCommands": true, "rest_routes": true}
	simple := map[string]bool{"string": true, "int": true, "bool": true, "[]string": true, "map[string]string": true}
	for reader, fs := range readers {
		isManifest := strings.HasSuffix(reader, " PluginManifest")
		for _, f := range fs {
			if registryOnly[f.key] {
				if !isManifest {
					t.Errorf("%s decodes %q, which is forbidden in v2", reader, f.key)
				}
				continue
			}
			want, ok := shared[f.key]
			if !ok {
				t.Errorf("%s decodes %q: neither a shared v2 key nor a compatibility key", reader, f.key)
				continue
			}
			if simple[f.typ] && want != f.typ && !polymorphic[f.key] {
				t.Errorf("%s decodes %q as %s but v2 types it %s", reader, f.key, f.typ, want)
			}
			if !simple[f.typ] && !polymorphic[f.key] {
				t.Errorf("%s decodes %q as %s: add it to the polymorphic list with a shape test", reader, f.key, f.typ)
			}
		}
	}
}

// jsonFields maps json key -> Go type string for the (flattened) fields of t.
func jsonFields(t reflect.Type) map[string]string {
	out := map[string]string{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous {
			for k, v := range jsonFields(f.Type) {
				out[k] = v
			}
			continue
		}
		if n := strings.Split(f.Tag.Get("json"), ",")[0]; n != "" && n != "-" {
			out[n] = strings.TrimPrefix(f.Type.String(), "*")
		}
	}
	return out
}
