package compose

// images_literal_scan_test.go — no image literal outside the lock.
//
// Purpose: ADR 0030 / D-0122. Every image reference the CLI generates or runs
// comes from the image lock (images.yaml -> images.lock.json); this test fails
// on any Go string literal under internal/ and cmd/ that spells an image
// reference, so the three-source drift that caused D-0122 cannot return.
// Inputs: non-test Go files under ../../internal and ../../cmd;
// testdata/image-literal-allowlist.txt, lines `<path> <image> # <reason>`.
// Outputs: a failure per literal that is neither in the lock accessor path nor
// allowlisted, per stale allowlist line, and for an allowlist longer than
// maxAllowlist.
// Constraints: the allowlist may only shrink. A literal is an image reference
// when it matches repo:tag (tag may be %s/%v) and its repository is a locked
// repository, a well-known image name, or carries a registry host (a dotted first path segment). Pure
// host:port strings (numeric tag) are not images.

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// maxAllowlist is the ratchet: the allowlist has exactly the three tool
// entries today and may only shrink.
const maxAllowlist = 3

var imageShapeRE = regexp.MustCompile(`^((?:[a-z0-9][a-z0-9._-]*/)*[a-z0-9][a-z0-9._-]*):(%[sv]|[A-Za-z0-9._-]+)(@sha256:[0-9a-f]{64})?$`)

var wellKnownImages = strings.Fields("golang ubuntu alpine debian node python busybox nginx redis postgres mysql mongo traefik caddy rust openssh linuxserver/openssh-server")

func normalizeRepo(r string) string {
	r = strings.TrimPrefix(r, "docker.io/")
	return strings.TrimPrefix(r, "library/")
}

// imageRepos is the set of repositories an image literal may name.
func imageRepos() map[string]bool {
	set := map[string]bool{}
	for _, w := range wellKnownImages {
		set[w] = true
	}
	for _, r := range LockedImages() {
		set[normalizeRepo(r.Repository)] = true
	}
	return set
}

// isImageLiteral reports whether s spells an image reference.
func isImageLiteral(s string, repos map[string]bool) bool {
	m := imageShapeRE.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	repo, tag := m[1], m[2]
	if i := strings.Index(repo, "/"); i > 0 && strings.Contains(repo[:i], ".") {
		return true // first path segment is a registry host (docker.io, ghcr.io, lscr.io, ...)
	}
	if strings.Trim(tag, "0123456789") == "" && m[3] == "" {
		return false // host:port, not an image
	}
	return repos[normalizeRepo(repo)]
}

// scanImageLiterals returns "<path> <literal>" for every image literal found.
func scanImageLiterals(t *testing.T, roots ...string) []string {
	t.Helper()
	repos := imageRepos()
	var hits []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if perr != nil {
				t.Fatalf("parse %s: %v", path, perr)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if s, uerr := strconv.Unquote(lit.Value); uerr == nil && isImageLiteral(s, repos) {
						rel, _ := filepath.Rel("../..", path)
						hits = append(hits, filepath.ToSlash(rel)+" "+s)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(hits)
	return hits
}

func readImageAllowlist(t *testing.T) map[string]bool {
	t.Helper()
	f, err := os.Open("testdata/image-literal-allowlist.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		body, reason, _ := strings.Cut(line, "#")
		fields := strings.Fields(body)
		if len(fields) != 2 || strings.TrimSpace(reason) == "" {
			t.Fatalf("allowlist line %q must be `<path> <image> # <reason>`", line)
		}
		out[fields[0]+" "+fields[1]] = true
	}
	return out
}

func TestImageLiteralScan(t *testing.T) {
	allow := readImageAllowlist(t)
	if len(allow) > maxAllowlist {
		t.Fatalf("allowlist has %d entries; it may only shrink (max %d)", len(allow), maxAllowlist)
	}
	used := map[string]bool{}
	for _, hit := range scanImageLiterals(t, "../../internal", "../../cmd") {
		if allow[hit] {
			used[hit] = true
			continue
		}
		t.Errorf("image literal outside the lock: %s\n  move it into internal/compose/images.yaml and read it with compose.ImageRef/LockedRef", hit)
	}
	for entry := range allow {
		if !used[entry] {
			t.Errorf("stale allowlist entry %q: the literal is gone, delete the line", entry)
		}
	}
}

func TestImageLiteralScanDetector(t *testing.T) {
	repos := imageRepos()
	for s, want := range map[string]bool{
		"redis:7-alpine": true, "docker.io/pgsty/minio:latest": true, "hasura/graphql-engine:%s": true,
		"nginx:alpine": true, "ghcr.io/acme/tool:v1": true, "golang:1.22-alpine": true,
		"redis:6379": false, "localhost:8080": false, "postgres://u@h:5432/db": false, "key:value": false,
		"nself/nself-admin": false, "a b:c": false,
	} {
		if got := isImageLiteral(s, repos); got != want {
			t.Errorf("isImageLiteral(%q) = %v, want %v", s, got, want)
		}
	}
}
