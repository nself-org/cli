package main

// manifests.go: read every plugin manifest of one tier through manifestv2.
//
// Purpose: manifestv2 is the only manifest reader (Constitution 3.4), so v1 and
// v2 plugin.json files both work and a v1 file and its v2 twin give one result.
// Inputs: the root of a tier tree (free/ or paid/): one directory per plugin
// holding plugin.json. Outputs: manifests by slug.
// Constraints: the directory name must equal the manifest name (a registry key
// is the slug released CLIs install by); every problem is reported, sorted.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

// scanManifests loads <root>/<slug>/plugin.json for every slug directory.
func scanManifests(root string) (map[string]*manifestv2.Manifest, []string) {
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, []string{"plugins: " + err.Error()}
	}
	out := map[string]*manifestv2.Manifest{}
	var probs []string
	for _, e := range ents { // ReadDir returns entries sorted by name
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		path := filepath.Join(root, e.Name(), "plugin.json")
		data, err := os.ReadFile(path)
		if err != nil {
			probs = append(probs, fmt.Sprintf("%s: no readable plugin.json (%v)", e.Name(), err))
			continue
		}
		m, err := manifestv2.ParseQuiet(data)
		if err != nil {
			probs = append(probs, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}
		if m.Name != e.Name() {
			probs = append(probs, fmt.Sprintf("%s: manifest name %q differs from its directory", e.Name(), m.Name))
			continue
		}
		out[m.Name] = m
	}
	return out, probs
}
