package repoqa

import (
	"path/filepath"
	"strings"
	"testing"
)

const layersFile = "internal/repoqa/testdata/layers.yaml"

// ratchetBasis is the set of wrong-way (upward) domain edges recorded at the
// branch point (EPIC G1). layers.yaml's `ratchet:` list must EQUAL it: a new
// wrong-way edge is rejected, and removing one forces a visible edit here, so
// the ratchet can never be loosened or refilled by a data-file change alone.
var ratchetBasis = map[edge]bool{
	{From: "internal/health", To: "internal/build"}:    true,
	{From: "internal/migration", To: "internal/build"}: true,
}

// TestLayering enforces D1-D3 on the real import graph against layers.yaml.
func TestLayering(t *testing.T) {
	g := loadGraph(t)
	spec := loadLayersSpec(t, filepath.Join(repoRoot(t), layersFile))
	if problems := checkLayering(g, spec, ratchetBasis); len(problems) > 0 {
		t.Fatalf("%d layering finding(s):\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

const synthSpec = `
version: 1
default: L1
rules:
  - {match: "cmd/**", layer: L3}
  - {match: "internal/repoqa/**", layer: test-only}
  - {match: "internal/config/**", layer: L0}
  - {match: "internal/build/**", layer: L2}
  - {match: "internal/bundle/**", layer: L2}
  - {match: "internal/deploy/**", layer: L2}
  - {match: "internal/doctor/**", layer: L2}
  - {match: "internal/health/**", layer: L2}
  - {match: "internal/migration/**", layer: L2}
  - {match: "internal/reconcile/**", layer: L2}
edges:
  - {from: internal/build, to: internal/bundle}
  - {from: internal/deploy, to: internal/health}
  - {from: internal/reconcile, to: internal/build, pending: P7-LIVE-03}
ratchet:
  - {from: internal/health, to: internal/build}
  - {from: internal/migration, to: internal/build}
forbidden:
  - {from: internal/build, to: internal/reconcile}
`

func mustSpec(t *testing.T) layersSpec {
	t.Helper()
	s, err := parseLayersSpec([]byte(synthSpec))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// synthGraph builds a graph from "from>to" strings; extra names become packages
// with no edges.
func synthGraph(edges []string, extra ...string) graph {
	g := graph{pkgs: map[string]pkgInfo{}, imports: map[string][]string{}, testImporters: map[string][]string{}}
	for _, e := range edges {
		from, to, _ := strings.Cut(e, ">")
		g.pkgs[from], g.pkgs[to] = pkgInfo{Name: "p"}, pkgInfo{Name: "p"}
		g.imports[from] = append(g.imports[from], to)
	}
	for _, p := range extra {
		g.pkgs[p] = pkgInfo{Name: "p"}
	}
	return g
}

// baseEdges is a graph that satisfies synthSpec exactly.
func baseEdges(extra ...string) []string {
	return append([]string{
		"internal/deploy>internal/health", "internal/build>internal/bundle",
		"internal/health>internal/build", "internal/migration>internal/build",
	}, extra...)
}

// TestLayeringRules proves every rule fires on a planted violation (synthetic
// graphs, no repo edits) and that the legal cases pass.
func TestLayeringRules(t *testing.T) {
	cases := []struct {
		name  string
		edges []string
		tweak func(*layersSpec)
		basis map[edge]bool
		want  []string // every substring must appear; empty means no findings
	}{
		{name: "base passes", edges: baseEdges()},
		{name: "same-domain subpackage edge passes", edges: baseEdges("internal/deploy/bluegreen>internal/deploy")},
		{name: "subpackage edge covered by domain entry", edges: append(baseEdges()[1:], "internal/deploy/bluegreen>internal/health")},
		{name: "pending edge not yet present passes", edges: baseEdges()},
		{name: "L0 -> L2 (D1)", edges: baseEdges("internal/config>internal/build"),
			want: []string{"D1: internal/config imports internal/build (L0 -> L2)"}},
		{name: "internal -> cmd (D2)", edges: baseEdges("internal/doctor>cmd/commands"),
			want: []string{"D2: internal/doctor imports cmd/commands (L2 -> L3)"}},
		{name: "L1 -> L2 upward (D3)", edges: baseEdges("internal/zzscratch>internal/build"),
			want: []string{"D3: internal/zzscratch imports internal/build (L1 -> L2)"}},
		{name: "unlisted L2 -> L2 (D3)", edges: baseEdges("internal/doctor>internal/deploy"),
			want: []string{"D3: internal/doctor imports internal/deploy (L2 -> L2)", "unlisted same-layer edge"}},
		{name: "forbidden build -> reconcile", edges: baseEdges("internal/build>internal/reconcile"),
			want: []string{"D3: internal/build imports internal/reconcile", "forbidden edge"}},
		{name: "unclassified package", edges: baseEdges("weird/pkg>internal/config"),
			want: []string{"unclassified: weird/pkg"}},
		{name: "stale edges entry", edges: baseEdges()[1:],
			want: []string{"stale: layers.yaml edge internal/deploy -> internal/health has no package edge"}},
		{name: "stale ratchet entry", edges: baseEdges()[:3],
			want: []string{"stale: layers.yaml ratchet edge internal/migration -> internal/build"}},
		{name: "pending edge that now exists", edges: baseEdges("internal/reconcile>internal/build"),
			want: []string{"pending: layers.yaml edge internal/reconcile -> internal/build", "remove the pending tag"}},
		{name: "ratchet entry outside the basis", edges: baseEdges(),
			basis: map[edge]bool{{From: "internal/health", To: "internal/build"}: true},
			want:  []string{"ratchet: internal/migration -> internal/build is not in ratchetBasis"}},
		{name: "ratchet list smaller than the basis", edges: baseEdges()[:3],
			tweak: func(s *layersSpec) { s.Ratchet = s.Ratchet[:1] },
			want:  []string{"drop internal/migration -> internal/build from ratchetBasis"}},
		{name: "test-only imported", edges: baseEdges("internal/doctor>internal/repoqa"),
			want: []string{"D3: internal/doctor imports internal/repoqa", "test-only"}},
		{name: "cmd may import anything", edges: baseEdges("cmd/commands>internal/build", "cmd/commands>internal/config")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := mustSpec(t)
			if tc.tweak != nil {
				tc.tweak(&spec)
			}
			basis := tc.basis
			if basis == nil {
				basis = ratchetBasis
			}
			got := strings.Join(checkLayering(synthGraph(tc.edges), spec, basis), "\n")
			if len(tc.want) == 0 && got != "" {
				t.Fatalf("expected no findings, got:\n%s", got)
			}
			if len(tc.want) > 0 && got == "" {
				t.Fatalf("expected findings %q, got none (the rule did not fire)", tc.want)
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("findings do not contain %q:\n%s", w, got)
				}
			}
		})
	}
	t.Run("domain entry with no pair is stale", func(t *testing.T) {
		spec := mustSpec(t)
		spec.Edges = append(spec.Edges, edgeEntry{From: "internal/doctor", To: "internal/deploy"})
		got := strings.Join(checkLayering(synthGraph(baseEdges()), spec, ratchetBasis), "\n")
		if !strings.Contains(got, "stale: layers.yaml edge internal/doctor -> internal/deploy") {
			t.Fatalf("stale domain entry not reported:\n%s", got)
		}
	})
	t.Run("rejects loosened or malformed spec", func(t *testing.T) {
		for name, doc := range map[string]string{
			"unknown key": "version: 1\ndefault: L1\nextra: 1\n",
			"bad version": "version: 2\ndefault: L1\n",
			"bad layer":   "version: 1\ndefault: L9\n",
			"bad pending": "version: 1\ndefault: L1\nedges:\n  - {from: a, to: b, pending: soon}\n",
		} {
			if _, err := parseLayersSpec([]byte(doc)); err == nil {
				t.Errorf("%s: spec accepted", name)
			}
		}
	})
}

// TestLayeringOrphanRules proves D9 fires on planted orphans and stale lines.
func TestLayeringOrphanRules(t *testing.T) {
	spec := mustSpec(t)
	entries := func(lines ...string) []orphanEntry {
		e, problems := parseOrphans(lines)
		if len(problems) > 0 {
			t.Fatalf("bad fixture: %v", problems)
		}
		return e
	}
	g := synthGraph(nil, "internal/old", "internal/used", "internal/zznew")
	g.pkgs["internal/mainpkg"] = pkgInfo{Name: "main"}
	g.imports["internal/user"] = []string{"internal/used"}
	g.pkgs["internal/user"] = pkgInfo{Name: "p"}
	g.imports["internal/user"] = append(g.imports["internal/user"], "internal/old")
	g.pkgs["internal/repoqa"] = pkgInfo{Name: "repoqa"}

	check := func(name string, g graph, lines []string, basis map[string]bool, want ...string) {
		t.Run(name, func(t *testing.T) {
			got := strings.Join(checkOrphans(g, spec, entries(lines...), basis), "\n")
			if len(want) == 0 && got != "" {
				t.Fatalf("expected no findings, got:\n%s", got)
			}
			if len(want) > 0 && got == "" {
				t.Fatalf("expected %q, got none", want)
			}
			for _, w := range want {
				if !strings.Contains(got, w) {
					t.Errorf("findings do not contain %q:\n%s", w, got)
				}
			}
		})
	}
	// internal/old is imported by internal/user, so only user and zznew are orphans.
	base := map[string]bool{"internal/user": true, "internal/zznew": true}
	check("tight list passes", g, []string{"internal/user", "internal/zznew"}, base)
	check("new orphan", g, []string{"internal/user"}, map[string]bool{"internal/user": true},
		"D9: internal/zznew has no importer", "pending:<ticket-id>")
	check("stale line has importers", g, []string{"internal/old", "internal/user", "internal/zznew"}, base,
		"D9: internal/old is listed in orphans.txt but has importers", "(stale)")
	check("stale line does not exist", g, []string{"internal/gone", "internal/user", "internal/zznew"}, base,
		"internal/gone is listed in orphans.txt but does not exist")
	check("plain line outside basis", g, []string{"internal/user", "internal/zznew"}, map[string]bool{"internal/user": true},
		"internal/zznew is a plain orphans.txt line outside orphanBasis", "new orphans are rejected")
	check("basis larger than list", g, []string{"internal/user", "internal/zznew"},
		map[string]bool{"internal/user": true, "internal/zznew": true, "internal/old": true},
		"drop internal/old from orphanBasis")
	check("pending for a package that does not exist yet passes", g,
		[]string{"internal/later pending:P7-ZZ-01", "internal/user", "internal/zznew"}, base)
	check("pending for a package that now has an importer is stale", g,
		[]string{"internal/old pending:P7-ZZ-01", "internal/user", "internal/zznew"}, base,
		"internal/old is listed in orphans.txt but has importers")
	g2 := synthGraph(nil, "internal/helper")
	g2.testImporters["internal/helper"] = []string{"internal/other"}
	g2.pkgs["internal/other"] = pkgInfo{Name: "p"}
	check("importer from another package's tests is not an orphan", g2, []string{"internal/other"}, map[string]bool{"internal/other": true})
	check("main and test-only packages are exempt", synthGraph(nil, "internal/repoqa"), nil, map[string]bool{})

	for name, lines := range map[string][]string{
		"unsorted": {"internal/b", "internal/a"}, "bad pending": {"internal/a pending:soon"},
		"outside internal": {"cmd/x"}, "duplicate pkg": {"internal/a", "internal/a pending:P7-ZZ-01"},
	} {
		if _, problems := parseOrphans(lines); len(problems) == 0 {
			t.Errorf("%s: parser accepted %v", name, lines)
		}
	}
}
