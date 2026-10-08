package repoqa

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// orphanBasis is the set of plain-line orphans recorded at the branch point
// (EPIC G1). orphans.txt's plain lines must EQUAL this set: a new orphan is
// rejected, and a wired-up or deleted package forces a visible edit here, so
// the list can never refill after a shrink. `pending:` lines are outside the
// basis; they name a Ticket that gives the package its first importer.
// -update drops stale plain lines and prints the literal to paste.
var orphanBasis = map[string]bool{
	"internal/aiprofile":        true,
	"internal/controlplane/sim": true,
	"internal/cost":             true,
	"internal/domain":           true,
	"internal/installmeta":      true,
	"internal/model":            true,
	"internal/plugin/lifecycle": true,
	"internal/runbook":          true,
	"internal/runtime":          true,
	"internal/upgrade":          true,
	"internal/webhook":          true,
}

const orphansFile = "internal/repoqa/testdata/orphans.txt"

// orphanEntry is one orphans.txt line: a package, optionally pending a Ticket.
type orphanEntry struct {
	Pkg     string
	Pending string
}

// parseOrphans reads orphans.txt content: `<import-path>` or
// `<import-path> pending:<ticket-id>`, `#` comments and blank lines ignored,
// bytewise sorted, no package twice.
func parseOrphans(lines []string) ([]orphanEntry, []string) {
	var entries []orphanEntry
	problems := checkSortedLines("orphans.txt", lines)
	seen := map[string]bool{}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		e := orphanEntry{Pkg: f[0]}
		switch {
		case len(f) == 2 && strings.HasPrefix(f[1], "pending:") && ticketIDRe.MatchString(strings.TrimPrefix(f[1], "pending:")):
			e.Pending = strings.TrimPrefix(f[1], "pending:")
		case len(f) != 1:
			problems = append(problems, fmt.Sprintf("orphans.txt: %q is not \"<pkg>\" or \"<pkg> pending:<ticket-id>\"", line))
			continue
		}
		if !strings.HasPrefix(e.Pkg, "internal/") || seen[e.Pkg] {
			problems = append(problems, fmt.Sprintf("orphans.txt: %q is not under internal/ or is listed twice", line))
			continue
		}
		seen[e.Pkg] = true
		entries = append(entries, e)
	}
	return entries, problems
}

// orphansOf returns the D9 orphans of g: internal/** packages that are not
// main, not test-only, and have no non-test importer and no test importer from
// another package (a package used only by another's tests is test support).
func orphansOf(g graph, spec layersSpec) map[string]bool {
	imported := g.importersOf()
	out := map[string]bool{}
	for p, info := range g.pkgs {
		layer, _ := spec.classify(p)
		if strings.HasPrefix(p, "internal/") && info.Name != "main" && layer != layerTestOnly &&
			len(imported[p]) == 0 && len(g.testImporters[p]) == 0 {
			out[p] = true
		}
	}
	return out
}

// staleOrphanLines returns the plain entries that are stale: the package has
// importers, or no longer exists. Pending lines for a missing package are valid.
func staleOrphanLines(g graph, spec layersSpec, entries []orphanEntry) map[string]string {
	orphans := orphansOf(g, spec)
	out := map[string]string{}
	for _, e := range entries {
		_, exists := g.pkgs[e.Pkg]
		switch {
		case !exists && e.Pending == "":
			out[e.Pkg] = "does not exist"
		case exists && !orphans[e.Pkg]:
			out[e.Pkg] = "has importers"
		}
	}
	return out
}

// checkOrphans compares the graph with orphans.txt and the basis (D9).
func checkOrphans(g graph, spec layersSpec, entries []orphanEntry, basis map[string]bool) []string {
	var out []string
	listed := map[string]bool{}
	plain := map[string]bool{}
	for _, e := range entries {
		listed[e.Pkg] = true
		if e.Pending == "" {
			plain[e.Pkg] = true
		}
	}
	for p := range orphansOf(g, spec) {
		if !listed[p] {
			out = append(out, fmt.Sprintf("D9: %s has no importer; wire it, delete it, or add \"%s pending:<ticket-id>\" to orphans.txt", p, p))
		}
	}
	for p, why := range staleOrphanLines(g, spec, entries) {
		out = append(out, fmt.Sprintf("D9: %s is listed in orphans.txt but %s; delete the line (stale)", p, why))
	}
	for p := range plain {
		if !basis[p] {
			out = append(out, fmt.Sprintf("D9: %s is a plain orphans.txt line outside orphanBasis; new orphans are rejected (wire it, or use pending:)", p))
		}
	}
	for p := range basis {
		if !plain[p] {
			out = append(out, fmt.Sprintf("D9: orphans.txt no longer lists %s; drop %s from orphanBasis in orphans_test.go", p, p))
		}
	}
	sort.Strings(out)
	return out
}

// TestOrphans enforces D9 on the real module against testdata/orphans.txt.
func TestOrphans(t *testing.T) {
	g := loadGraph(t)
	root := repoRoot(t)
	spec := loadLayersSpec(t, filepath.Join(root, layersFile))
	path := filepath.Join(root, orphansFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", orphansFile, err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	entries, problems := parseOrphans(lines)

	if *update {
		stale := staleOrphanLines(g, spec, entries)
		var keep []string
		for _, line := range lines {
			f := strings.Fields(line)
			if len(f) == 1 && !strings.HasPrefix(f[0], "#") && stale[f[0]] != "" {
				continue
			}
			keep = append(keep, line)
		}
		if next := strings.Join(keep, "\n") + "\n"; next != string(data) {
			if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
				t.Fatal(err)
			}
			entries, problems = parseOrphans(keep)
		}
		var plain []string
		for _, e := range entries {
			if e.Pending == "" {
				plain = append(plain, e.Pkg)
			}
		}
		t.Logf("-update: set orphanBasis in orphans_test.go to exactly: %s", strings.Join(plain, ", "))
	}

	problems = append(problems, checkOrphans(g, spec, entries, orphanBasis)...)
	if len(problems) > 0 {
		t.Fatalf("%d orphan finding(s):\n%s", len(problems), strings.Join(problems, "\n"))
	}
}
