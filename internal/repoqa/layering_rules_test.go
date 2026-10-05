package repoqa

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Layer names. L0 < L1 < L2 < L3; test-only may be imported by nothing.
const (
	layerL0       = "L0"
	layerL1       = "L1"
	layerL2       = "L2"
	layerTestOnly = "test-only"
)

var layerRank = map[string]int{layerL0: 0, layerL1: 1, layerL2: 2, "L3": 3}

// ticketIDRe is the form of a `pending:` ticket id (EPIC Contracts).
var ticketIDRe = regexp.MustCompile(`^P\d+-[A-Z]{2,5}-\d{2,3}$`)

type layerRule struct {
	Match string `yaml:"match"`
	Layer string `yaml:"layer"`
}

// edgeEntry is a domain -> domain pair (EPIC G3); Pending names the Ticket that
// first creates the edge.
type edgeEntry struct {
	From    string `yaml:"from"`
	To      string `yaml:"to"`
	Pending string `yaml:"pending,omitempty"`
}

// layersSpec is testdata/layers.yaml v1: hand-edited, never rewritten.
type layersSpec struct {
	Version   int         `yaml:"version"`
	Default   string      `yaml:"default"`
	Rules     []layerRule `yaml:"rules"`
	Edges     []edgeEntry `yaml:"edges"`
	Ratchet   []edgeEntry `yaml:"ratchet"`
	Forbidden []edgeEntry `yaml:"forbidden"`
}

// edge is a domain pair.
type edge struct{ From, To string }

func (e edge) String() string { return e.From + " -> " + e.To }

// parseLayersSpec decodes and validates layers.yaml content (strict: unknown
// keys fail).
func parseLayersSpec(data []byte) (layersSpec, error) {
	var s layersSpec
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return s, fmt.Errorf("parse layers.yaml: %v", err)
	}
	valid := func(l string) bool { _, ok := layerRank[l]; return ok || l == layerTestOnly }
	if s.Version != 1 {
		return s, fmt.Errorf("layers.yaml: version %d, want 1", s.Version)
	}
	if !valid(s.Default) {
		return s, fmt.Errorf("layers.yaml: default layer %q is not L0-L3 or test-only", s.Default)
	}
	for _, r := range s.Rules {
		if r.Match == "" || !valid(r.Layer) {
			return s, fmt.Errorf("layers.yaml: bad rule {match: %q, layer: %q}", r.Match, r.Layer)
		}
	}
	for _, list := range [][]edgeEntry{s.Edges, s.Ratchet, s.Forbidden} {
		for _, e := range list {
			if e.From == "" || e.To == "" || (e.Pending != "" && !ticketIDRe.MatchString(e.Pending)) {
				return s, fmt.Errorf("layers.yaml: bad edge entry %+v", e)
			}
		}
	}
	return s, nil
}

func loadLayersSpec(t testing.TB, path string) layersSpec {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	s, err := parseLayersSpec(data)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// classify returns the layer of a module-relative package: the first matching
// rule ("x/**" matches x and everything below it, otherwise exact), else the
// default for internal/**. ok is false for a package no rule matches.
func (s layersSpec) classify(pkg string) (string, bool) {
	for _, r := range s.Rules {
		if base, glob := strings.CutSuffix(r.Match, "/**"); glob {
			if pkg == base || strings.HasPrefix(pkg, base+"/") {
				return r.Layer, true
			}
		} else if pkg == r.Match {
			return r.Layer, true
		}
	}
	if strings.HasPrefix(pkg, "internal/") {
		return s.Default, true
	}
	return "", false
}

// domain is the first two path segments (internal/deploy/bluegreen ->
// internal/deploy), or three for internal/cmd/<x> (EPIC G3).
func domain(pkg string) string {
	seg := strings.Split(pkg, "/")
	n := 2
	if len(seg) >= 3 && seg[0] == "internal" && seg[1] == "cmd" {
		n = 3
	}
	if len(seg) < n {
		n = len(seg)
	}
	return strings.Join(seg[:n], "/")
}

func entrySet(list []edgeEntry, pending bool) map[edge]bool {
	out := map[edge]bool{}
	for _, e := range list {
		if pending || e.Pending == "" {
			out[edge{e.From, e.To}] = true
		}
	}
	return out
}

func isCmd(pkg string) bool { return pkg == "cmd" || strings.HasPrefix(pkg, "cmd/") }

// checkLayering applies D1, D2 and D3 to the graph and checks layers.yaml for
// stale or loosened entries. ratchetBasis is the hard-coded set the ratchet
// list must equal (EPIC G1). It returns sorted findings; none means pass.
// Edge findings read `D<n>: <a> imports <b> (<la> -> <lb>): <fix>`.
func checkLayering(g graph, spec layersSpec, ratchetBasis map[edge]bool) []string {
	var out []string
	add := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }
	ratchet := entrySet(spec.Ratchet, true)
	forbidden := entrySet(spec.Forbidden, true)
	listed := entrySet(spec.Edges, true)
	seen := map[edge]bool{}

	for _, p := range sortedKeys(g.pkgs) {
		if _, ok := spec.classify(p); !ok {
			add("unclassified: %s matches no layers.yaml rule and is not under internal/; add a rule for it", p)
		}
	}
	for _, a := range sortedKeys(g.imports) {
		la, okA := spec.classify(a)
		for _, b := range g.imports[a] {
			if _, ok := g.pkgs[b]; !ok || domain(a) == domain(b) {
				continue
			}
			pair := edge{domain(a), domain(b)}
			seen[pair] = true
			lb, okB := spec.classify(b)
			head := fmt.Sprintf("%s imports %s (%s -> %s)", a, b, la, lb)
			switch {
			case forbidden[pair]:
				add("D3: %s: %s is a forbidden edge in layers.yaml; remove the import", head, pair)
			case strings.HasPrefix(a, "internal/") && isCmd(b):
				add("D2: %s: internal/ must not import cmd/; move the shared code under internal/", head)
			case !okA || !okB:
				// reported once above as unclassified
			case lb == layerTestOnly:
				add("D3: %s: nothing may import a test-only package", head)
			case la == layerL0 && lb != layerL0:
				add("D1: %s: L0 may import only L0; move the code down or reclassify the package", head)
			case layerRank[lb] > layerRank[la] && !ratchet[pair]:
				add("D3: %s: imports point down; move the code down, or invert the dependency", head)
			case layerRank[lb] == layerRank[la] && (la == layerL1 || la == layerL2) && !listed[pair] && !ratchet[pair]:
				add("D3: %s: unlisted same-layer edge; add {from: %s, to: %s} to edges in layers.yaml and the line to overview section 4.2", head, pair.From, pair.To)
			}
		}
	}
	for _, e := range spec.Edges {
		pair := edge{e.From, e.To}
		switch {
		case e.Pending == "" && !seen[pair]:
			add("stale: layers.yaml edge %s has no package edge; delete the entry", pair)
		case e.Pending != "" && seen[pair]:
			add("pending: layers.yaml edge %s (pending:%s) now exists; remove the pending tag", pair, e.Pending)
		}
	}
	inRatchet := map[edge]bool{}
	for _, e := range spec.Ratchet {
		pair := edge{e.From, e.To}
		inRatchet[pair] = true
		if !seen[pair] {
			add("stale: layers.yaml ratchet edge %s has no package edge; delete the entry and drop it from ratchetBasis in layering_test.go", pair)
		}
		if !ratchetBasis[pair] {
			add("ratchet: %s is not in ratchetBasis; new wrong-way edges are rejected", pair)
		}
	}
	for pair := range ratchetBasis {
		if !inRatchet[pair] {
			add("ratchet: layers.yaml no longer lists %s; drop %s from ratchetBasis in layering_test.go", pair, pair)
		}
	}
	sort.Strings(out)
	return out
}
