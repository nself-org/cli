package compose

// image_inputs_callsites_test.go — the table in image_inputs.go covers every
// place the CLI renders an image from the image lock (P7-LIVE-09, round 8).
//
// Purpose: enumerate the call sites by parsing the Go sources, so a new
// generator image cannot be added without a table row (and so without a decision
// about which env variables feed it).
// Inputs: the non-test Go sources under internal/ and cmd/.
// Outputs: test failures naming file, line and the missing row.
// Constraints: parse only, no network; a call with a non-literal entry name is
// allowed only in the files that define the lookup itself.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// notPinResolved are lock entries rendered with ImageRef and never passed to
// ResolveImage, so no project pin is ever consulted for them. The monitoring
// generator (internal/compose/monitoring) is the only such renderer.
var notPinResolved = map[string]string{
	"tempo":          "monitoring/tempo.go renders it with ImageRef only",
	"otel-collector": "monitoring/tempo.go renders it with ImageRef only",
}

// lookupFiles may call ImageRef with a name that is not a literal: they are the
// lookup itself (DefaultImage forwards its argument).
var lookupFiles = map[string]bool{"images.go": true, "images_lock.go": true}

type callSite struct {
	pos     string
	service string   // ResolveImage first argument ("" for a bare ImageRef)
	entries []string // lock entries rendered at the site
}

func calleeName(e ast.Expr) string {
	switch f := e.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

func stringLit(e ast.Expr) (string, bool) {
	b, ok := e.(*ast.BasicLit)
	if !ok || b.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(b.Value)
	return s, err == nil
}

// scanCallSites parses every non-test Go file under internal/ and cmd/ and
// returns the ResolveImage sites, the bare ImageRef sites and the problems.
func scanCallSites(t *testing.T) (resolve, bare []callSite, problems []string) {
	t.Helper()
	fset := token.NewFileSet()
	for _, root := range []string{"../../internal", "../../cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "testdata" {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if perr != nil {
				return perr
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				pos := fset.Position(call.Pos()).String()
				switch calleeName(call.Fun) {
				case "ImageRef":
					if entry, ok := stringLit(call.Args[0]); ok {
						bare = append(bare, callSite{pos: pos, entries: []string{entry}})
					} else if !lookupFiles[filepath.Base(path)] || !strings.Contains(filepath.ToSlash(path), "internal/compose/") {
						problems = append(problems, pos+": ImageRef with a non-literal entry name")
					}
				case "ResolveImage", "resolveHasuraImage":
					if len(call.Args) < 2 {
						return true
					}
					svc, ok := stringLit(call.Args[0])
					if !ok {
						problems = append(problems, pos+": ResolveImage with a non-literal service name")
						return true
					}
					site := callSite{pos: pos, service: svc}
					ast.Inspect(call.Args[1], func(m ast.Node) bool {
						c, ok := m.(*ast.CallExpr)
						if !ok {
							return true
						}
						switch calleeName(c.Fun) {
						case "ImageRef":
							if e, ok := stringLit(c.Args[0]); ok {
								site.entries = append(site.entries, e)
							}
						case "ResolvePostgresImage":
							site.entries = append(site.entries, "postgres", "pgvector")
						}
						return true
					})
					if len(site.entries) == 0 {
						problems = append(problems, pos+": ResolveImage("+svc+") renders an image this test cannot see; pass ImageRef(\"<entry>\", ...) or ResolvePostgresImage(...)")
					}
					resolve = append(resolve, site)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("scanning %s: %v", root, err)
		}
	}
	return resolve, bare, problems
}

// TestImageInputsCoverEveryCallSite: every image a call site renders from the
// lock has a row in the table, and every row has a call site.
func TestImageInputsCoverEveryCallSite(t *testing.T) {
	resolve, bare, problems := scanCallSites(t)
	if len(resolve) < 10 || len(bare) < 10 {
		t.Fatalf("the scan found %d ResolveImage and %d ImageRef sites: the parser lost the sources", len(resolve), len(bare))
	}
	rows := map[string]bool{}
	byEntry := map[string]bool{}
	for _, in := range ImageInputs() {
		rows[in.Service+"/"+in.Entry] = true
		byEntry[in.Entry] = true
		if _, ok := LockedRef(in.Entry); !ok {
			t.Errorf("row %s/%s names an entry the image lock lacks", in.Service, in.Entry)
		}
	}
	// A row's Service is the key ResolveImage and the lock use: the generator's
	// compose service name or, when the catalog gives one, its ImageKey.
	generated := map[string]bool{}
	for _, e := range serviceCatalog {
		generated[e.Name], generated[PinKey(e.Name)] = true, true
	}
	for _, name := range []string{"meilisearch-init"} { // services built outside the catalog
		generated[name] = true
	}
	for _, in := range ImageInputs() {
		if !generated[in.Service] {
			t.Errorf("row %s/%s: Service is neither a generated compose service nor a catalog ImageKey", in.Service, in.Entry)
		}
		if got := PinKey(in.Service); got != in.Service {
			t.Errorf("row %s/%s: Service must be the pin key, PinKey maps it to %q", in.Service, in.Entry, got)
		}
	}
	seen := map[string]bool{}
	for _, s := range resolve {
		for _, e := range s.entries {
			seen[s.service+"/"+e] = true
			if !rows[s.service+"/"+e] {
				problems = append(problems, s.pos+": ResolveImage("+s.service+") renders lock entry "+e+" which has no row in image_inputs.go")
			}
		}
		if _, exempt := notPinResolved[s.service]; exempt {
			problems = append(problems, s.pos+": "+s.service+" is listed as not pin-resolved but is passed to ResolveImage")
		}
	}
	for _, s := range bare {
		e := s.entries[0]
		if _, exempt := notPinResolved[e]; !exempt && !byEntry[e] {
			problems = append(problems, s.pos+": ImageRef renders lock entry "+e+" which has no row in image_inputs.go and is not a documented not-pin-resolved image")
		}
	}
	for row := range rows {
		if !seen[row] {
			problems = append(problems, "row "+row+" has no ResolveImage call site: remove it or add the generator")
		}
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Error(p)
	}
}

// TestImageInputsTableShape: rows are unique, *_IMAGE variables come before
// *_VERSION ones (the generator reads the image first), and no variable repeats.
func TestImageInputsTableShape(t *testing.T) {
	seen := map[string]bool{}
	for _, in := range ImageInputs() {
		key := in.Service + "/" + in.Entry
		if seen[key] {
			t.Errorf("duplicate row %s", key)
		}
		seen[key] = true
		sawVersion := false
		for _, v := range in.Vars {
			switch {
			case strings.HasSuffix(v, "_VERSION"):
				sawVersion = true
			case strings.HasSuffix(v, "_IMAGE"):
				if sawVersion {
					t.Errorf("%s: %s must come before the *_VERSION variables", key, v)
				}
			default:
				t.Errorf("%s: %s is neither *_IMAGE nor *_VERSION", key, v)
			}
		}
	}
}
