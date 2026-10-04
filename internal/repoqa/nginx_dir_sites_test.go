package repoqa

// nginx_dir_sites_test.go — pins every place that builds an nginx directory
// path by hand, so a new writer cannot bypass the served-dir resolver.
//
// Purpose: D-0045. On a fronted deployment (NGINX_FRONTED_BY) the nginx that
// serves the project belongs to another stack, so any writer that joins
// "nginx" onto the project directory writes where nothing reads. The one
// resolver is internal/nginxtopo (ServedNginxDir / ServedSSLDir). This test
// finds every non-test filepath.Join or path.Join call with a literal "nginx"
// (or "nginx/...") argument outside internal/nginxtopo and requires each file's
// count to equal its line in testdata/nginx-own-dir-sites.txt exactly.
// Inputs: the cmd/, internal/ and tools/ trees and the allowlist file
// `<path> <count> # own-dir|served-root: <reason>`.
// Outputs: failures naming unlisted sites, stale entries and count drift.
// Constraints: read-only, no network. The allowlist is shrink-only: its total
// may never exceed nginxDirSitesBasis. A site that moves onto nginxtopo is
// removed from the allowlist and the basis lowered in the same change; raising
// the basis needs the same review as adding a writer. own-dir sites write
// build's own nginx.conf/includes or clean/remove the project's own tree and
// must never follow a fronting stack; served-root sites already receive a root
// resolved by nginxtopo.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// nginxDirSitesBasis is the largest total the allowlist may hold. Lower it
// when entries are removed; never raise it to admit a new writer.
const nginxDirSitesBasis = 18

// nginxDirAllowlist is the allowlist file, relative to the repo root.
const nginxDirAllowlist = "internal/repoqa/testdata/nginx-own-dir-sites.txt"

// nginxPathLiteral reports whether a string literal names the nginx directory.
func nginxPathLiteral(v string) bool {
	return v == "nginx" || strings.HasPrefix(v, "nginx/")
}

// nginxJoinSites counts, per file, the path.Join/filepath.Join calls that take
// a literal nginx path element. Test files, vendor and testdata are skipped, as
// is internal/nginxtopo (the resolver itself).
func nginxJoinSites(t *testing.T, root string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	fset := token.NewFileSet()
	for _, dir := range []string{"cmd", "internal", "tools"} {
		base := filepath.Join(root, dir)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			if info.IsDir() {
				if info.Name() == "testdata" || info.Name() == "vendor" || rel == "internal/nginxtopo" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			f, perr := parser.ParseFile(fset, p, nil, 0)
			if perr != nil {
				return perr
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok && isJoinCall(call) && hasNginxLiteral(call) {
					counts[rel]++
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	return counts
}

// isJoinCall matches filepath.Join(...) and path.Join(...).
func isJoinCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Join" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && (id.Name == "filepath" || id.Name == "path")
}

// hasNginxLiteral reports whether any argument is an nginx path literal.
func hasNginxLiteral(call *ast.CallExpr) bool {
	for _, a := range call.Args {
		lit, ok := a.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		if v, err := strconv.Unquote(lit.Value); err == nil && nginxPathLiteral(v) {
			return true
		}
	}
	return false
}

// readNginxAllowlist parses the allowlist into path -> count. Every line needs
// a count and a "# own-dir:" or "# served-root:" reason.
func readNginxAllowlist(t *testing.T, root string) map[string]int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(nginxDirAllowlist)))
	if err != nil {
		t.Fatalf("read %s: %v", nginxDirAllowlist, err)
	}
	out := map[string]int{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		body, reason, found := strings.Cut(line, "#")
		reason = strings.TrimSpace(reason)
		fields := strings.Fields(body)
		okReason := strings.HasPrefix(reason, "own-dir:") || strings.HasPrefix(reason, "served-root:")
		if !found || len(fields) != 2 || !okReason || strings.TrimSpace(reason[strings.Index(reason, ":")+1:]) == "" {
			t.Fatalf("%s:%d: want `<path> <count> # own-dir|served-root: <reason>`, got %q", nginxDirAllowlist, i+1, line)
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil || n < 1 {
			t.Fatalf("%s:%d: bad count %q", nginxDirAllowlist, i+1, fields[1])
		}
		if _, dup := out[fields[0]]; dup {
			t.Fatalf("%s:%d: %s listed twice", nginxDirAllowlist, i+1, fields[0])
		}
		out[fields[0]] = n
	}
	return out
}

// TestNginxDirSites fails when a hand-built nginx directory path appears that
// is not on the allowlist, when an entry no longer matches the tree, or when
// the allowlist total passes its basis.
func TestNginxDirSites(t *testing.T) {
	root := repoRoot(t)
	found := nginxJoinSites(t, root)
	allowed := readNginxAllowlist(t, root)

	var problems []string
	for path, n := range found {
		switch want, ok := allowed[path]; {
		case !ok:
			problems = append(problems, path+": "+strconv.Itoa(n)+" unlisted nginx path join(s); use nginxtopo.ServedNginxDir (served) or list it with a reason (own-dir)")
		case want != n:
			problems = append(problems, path+": allowlist says "+strconv.Itoa(want)+", tree has "+strconv.Itoa(n)+"; update the entry (lower it when a site moved to nginxtopo)")
		}
	}
	total := 0
	for path, want := range allowed {
		total += want
		if _, ok := found[path]; !ok {
			problems = append(problems, path+": allowlisted but no nginx path join remains; delete the entry")
		}
	}
	if total > nginxDirSitesBasis {
		problems = append(problems, "allowlist total "+strconv.Itoa(total)+" exceeds basis "+strconv.Itoa(nginxDirSitesBasis)+"; the allowlist is shrink-only")
	}
	if total < nginxDirSitesBasis {
		problems = append(problems, "allowlist total "+strconv.Itoa(total)+" is below basis "+strconv.Itoa(nginxDirSitesBasis)+"; lower nginxDirSitesBasis in this file")
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("nginx directory call sites drifted from %s:\n  %s", nginxDirAllowlist, strings.Join(problems, "\n  "))
	}
}
