package repoqa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLayeringDebtExceptionGrowth(t *testing.T) {
	spec := loadLayersSpec(t, filepath.Join(repoRoot(t), layersFile))
	spec.Edges = append(spec.Edges, edgeEntry{From: "internal/doctor", To: "internal/deploy"})
	var edges []string
	for _, e := range append(append([]edgeEntry{}, spec.Edges...), spec.Ratchet...) {
		edges = append(edges, e.From+">"+e.To)
	}
	g := synthGraph(edges)
	if got := strings.Join(checkLayering(g, spec, ratchetBasis, acceptedBasis, nil), "\n"); !strings.Contains(got, "acceptedBasis") {
		t.Errorf("new accepted edge bypasses the basis guard:\n%s", got)
	}
	for _, pkg := range []string{"internal/new", "internal/missing"} {
		entries := []orphanEntry{{Pkg: pkg, Pending: "P7-ZZ-01"}}
		if got := strings.Join(checkOrphans(synthGraph(nil, "internal/new"), spec, entries, nil), "\n"); !strings.Contains(got, "pending orphan exceptions are not permitted") {
			t.Errorf("pending marker bypasses D9: %s", got)
		}
	}
}

func TestLayeringDebtHiddenImports(t *testing.T) {
	for _, file := range []string{"hidden_windows.go", "hidden_darwin.go", "hidden_test.go", "hidden_integration.go"} {
		for _, target := range []string{"internal/build", "cmd/commands"} {
			t.Run(file+"/"+target, func(t *testing.T) {
				root := t.TempDir()
				files := map[string]string{
					"go.mod":                  "module example.com/m\n\ngo 1.26.6\n",
					"internal/config/base.go": "package config\n",
					"internal/config/" + file: "//go:build integration\n\npackage config\nimport _ \"example.com/m/" + target + "\"\n",
					target + "/base.go":       "package target\n",
				}
				if file == "hidden_test.go" {
					files["internal/config/"+file] = "package config_test\nimport _ \"example.com/m/" + target + "\"\n"
				}
				if file == "hidden_windows.go" || file == "hidden_darwin.go" {
					files["internal/config/"+file] = "package config\nimport _ \"example.com/m/" + target + "\"\n"
				}
				for path, data := range files {
					path = filepath.Join(root, path)
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				g, err := runGoList(requireTool(t, "go"), root)
				if err != nil {
					t.Fatal(err)
				}
				rule := "D1:"
				if target == "cmd/commands" {
					rule = "D2:"
				}
				if got := strings.Join(checkLayering(g, mustSpec(t), nil, acceptedEntries(mustSpec(t).Edges), nil), "\n"); !strings.Contains(got, rule) {
					t.Fatalf("%s imports hidden in %s bypass %s:\n%s", target, file, rule, got)
				}
			})
		}
	}
}

func TestLayeringBasisMutations(t *testing.T) {
	spec := mustSpec(t)
	basis := acceptedEntries(spec.Edges)
	for name, mutate := range map[string]func([]edgeEntry) []edgeEntry{
		"addition":    func(es []edgeEntry) []edgeEntry { return append(es, edgeEntry{From: "a", To: "b"}) },
		"deletion":    func(es []edgeEntry) []edgeEntry { return es[1:] },
		"replacement": func(es []edgeEntry) []edgeEntry { es[0].To = "different"; return es },
		"pending":     func(es []edgeEntry) []edgeEntry { es[0].Pending = "P7-ZZ-01"; return es },
		"duplicate":   func(es []edgeEntry) []edgeEntry { return append(es, es[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			entries := mutate(append([]edgeEntry{}, spec.Edges...))
			if len(checkAcceptedEdges(entries, basis)) == 0 {
				t.Fatal("fixture mutation widened accepted edges")
			}
		})
	}
	legacy := sourceImport{"internal/config/legacy_test.go", "internal/config", "internal/build"}
	sources := map[sourceImport]bool{legacy: true}
	if got := checkSourceLayering([]sourceImport{legacy}, spec, sources); len(got) != 0 {
		t.Fatal(got)
	}
	if got := strings.Join(checkSourceLayering(nil, spec, sources), "\n"); !strings.Contains(got, "stale exemption") {
		t.Fatal(got)
	}
	copy := legacy
	copy.File = "internal/config/new_test.go"
	if got := strings.Join(checkSourceLayering([]sourceImport{legacy, copy}, spec, sources), "\n"); !strings.Contains(got, "D1:") {
		t.Fatal(got)
	}
}
