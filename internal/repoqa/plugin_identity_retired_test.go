package repoqa

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Scanner for retired plugin-identity literals (D15, P7-PLUG-67) and for
// readers of the project's inter-plugin secret (P7-PLUG-73).
//
// Purpose: keep PLUGIN_INTERNAL_SECRET from travelling anywhere new. The retired
// Q01 client posted it to ping.nself.org; the one remaining reader that sends
// it (nself doctor's ai client) is local-only. Every Go file that names the
// secret or its X-Internal-Token header must be on the allowlist below.
//
// Method: go/parser, not text. Every string literal is a candidate; `+` of
// string literals is folded recursively into one value ("PLUGIN_INTERNAL_" +
// "SECRET"), and the folded value is matched case-insensitively. Comments do
// not count. Findings carry a tag: "retired" or "secret".
//
// Out of scope, by design: dynamic construction (fmt.Sprintf, strings.Join,
// identifiers or constants joined with +). The guard stops accidental
// reintroduction, not a determined author; review covers the rest.

const (
	tagRetired = "retired"
	tagSecret  = "secret"
)

// retiredPluginIdentityLiterals are the strings that only the retired Q01
// plugin-identity client (D15, P7-PLUG-67) used. The client posted the
// project's PLUGIN_INTERNAL_SECRET to ping.nself.org on the first install of
// each plugin and kept per-host keys shared by every project. No non-test Go
// file may name any of them again.
var retiredPluginIdentityLiterals = []string{
	"/plugin/identity",
	"/plugin/token",
	"X-Plugin-Internal-Secret",
	"PLUGIN_TOKEN_ISSUER",
	"identity.key",
}

// pluginSecretLiterals are the names of the inter-plugin secret and its header.
// Only files on the allowlist may name them.
var pluginSecretLiterals = []string{
	"PLUGIN_INTERNAL_SECRET",
	"X-Internal-Token",
}

// secretReaderAllowlist lists the files that may name the secret: directories
// match by prefix, files exactly (paths are slash-separated, module-relative).
var secretReaderAllowlist = []string{
	"internal/build/",
	"internal/config/",
	"internal/setup/setup_env_files.go",
	"internal/secrets/rotation.go",
	"cmd/commands/doctor_ai_client.go",
}

// allowed reports whether the Go file at rel may name the inter-plugin secret.
func allowed(rel string) bool {
	for _, a := range secretReaderAllowlist {
		if strings.HasSuffix(a, "/") {
			if strings.HasPrefix(rel, a) {
				return true
			}
		} else if rel == a {
			return true
		}
	}
	return false
}

// finding is one literal found in one string. lit is the canonical spelling
// from the lists above, whatever the case in the source.
type finding struct {
	tag, name string
	line      int
	lit       string
}

func (f finding) String() string { return fmt.Sprintf("%s:%d: %s", f.name, f.line, f.lit) }

// foldString folds a tree of string literals joined by + (and parentheses) into
// one value. ok is false when any leaf is not a string literal.
func foldString(e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind != token.STRING {
			return "", false
		}
		v, err := strconv.Unquote(x.Value)
		return v, err == nil
	case *ast.ParenExpr:
		return foldString(x.X)
	case *ast.BinaryExpr:
		if x.Op != token.ADD {
			return "", false
		}
		l, lok := foldString(x.X)
		r, rok := foldString(x.Y)
		return l + r, lok && rok
	}
	return "", false
}

// scanLiterals parses src and returns one finding per (string value, literal)
// match, case-insensitively. name only labels findings. A foldable `+` chain
// counts once, at the position of its first operand.
func scanLiterals(name string, src []byte) ([]finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var out []finding
	match := func(pos token.Pos, val string) {
		low := strings.ToLower(val)
		line := fset.Position(pos).Line
		for tag, lits := range map[string][]string{tagRetired: retiredPluginIdentityLiterals, tagSecret: pluginSecretLiterals} {
			for _, lit := range lits {
				if strings.Contains(low, strings.ToLower(lit)) {
					out = append(out, finding{tag: tag, name: name, line: line, lit: lit})
				}
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.BinaryExpr:
			if v, ok := foldString(x); ok {
				match(x.Pos(), v)
				return false
			}
		case *ast.BasicLit:
			if v, ok := foldString(x); ok {
				match(x.Pos(), v)
			}
		}
		return true
	})
	return out, nil
}

// scanModule scans every non-test, non-vendor, non-testdata Go file under root
// and returns the findings with the given tag, labelled by module-relative path.
func scanModule(t *testing.T, root, tag string) []finding {
	t.Helper()
	var out []finding
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			// Hidden dirs (.git, .claude worktrees, .github), vendor and
			// testdata are not first-party shipped code.
			if path != root && (strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		fs, parseErr := scanLiterals(rel, data)
		if parseErr != nil {
			return parseErr
		}
		for _, f := range fs {
			if f.tag == tag {
				out = append(out, f)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func joinFindings(fs []finding) string {
	var parts []string
	for _, f := range fs {
		parts = append(parts, f.String())
	}
	return strings.Join(parts, "\n  ")
}

// TestPluginIdentityRetired fails when any non-test, non-vendor, non-testdata
// Go file names a retired ping plugin-identity path, the X-Plugin-Internal-Secret
// header, PLUGIN_TOKEN_ISSUER or identity.key. Only "retired" findings count.
func TestPluginIdentityRetired(t *testing.T) {
	if v := scanModule(t, repoRoot(t), tagRetired); len(v) > 0 {
		t.Fatalf("the Q01 plugin identity client is retired (D15); these files name it again:\n  %s", joinFindings(v))
	}
}

// TestPluginSecretReaderAllowlist fails when a Go file outside the allowlist
// names PLUGIN_INTERNAL_SECRET or X-Internal-Token. Only "secret" findings count.
func TestPluginSecretReaderAllowlist(t *testing.T) {
	if allowed("internal/plugin/x.go") {
		t.Error(`allowed("internal/plugin/x.go") = true, want false`)
	}
	if !allowed("internal/build/a.go") {
		t.Error(`allowed("internal/build/a.go") = false, want true`)
	}
	var bad []finding
	for _, f := range scanModule(t, repoRoot(t), tagSecret) {
		if !allowed(f.name) {
			bad = append(bad, f)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("PLUGIN_INTERNAL_SECRET (D15) may be read only by the allowlisted files; these name it:\n  %s", joinFindings(bad))
	}
}

// TestPluginIdentityScannerFixture runs the scanner over fixtures with known
// content so the module-wide tests cannot pass because the scanner matches
// nothing: one finding per retired literal, lowercase names, a split literal.
func TestPluginIdentityScannerFixture(t *testing.T) {
	root := repoRoot(t)
	rows := []struct {
		file string
		tag  string
		want int
	}{
		{"leak.go.txt", tagRetired, len(retiredPluginIdentityLiterals)},
		{"leak_lowercase.go.txt", tagSecret, 2},
		{"leak_split.go.txt", tagSecret, 1},
	}
	for _, r := range rows {
		data, err := os.ReadFile(filepath.Join(root, "internal", "repoqa", "testdata", "plugin_identity_retired", r.file))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		all, err := scanLiterals(r.file, data)
		if err != nil {
			t.Fatalf("%s: %v", r.file, err)
		}
		var got []finding
		for _, f := range all {
			if f.tag == r.tag {
				got = append(got, f)
			} else {
				t.Errorf("%s: unexpected %s finding %s", r.file, f.tag, f)
			}
		}
		if len(got) != r.want {
			t.Errorf("%s: expected %d %s findings, got %d: %v", r.file, r.want, r.tag, len(got), got)
		}
		if r.tag == tagRetired {
			for _, lit := range retiredPluginIdentityLiterals {
				n := 0
				for _, f := range got {
					if f.lit == lit {
						n++
					}
				}
				if n != 1 {
					t.Errorf("%s: literal %q: expected 1 finding, got %d", r.file, lit, n)
				}
			}
		}
	}
}
