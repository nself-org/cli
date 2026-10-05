package main

// manifests.go: read every plugin manifest of one tier through manifestv2.
//
// Purpose: manifestv2 is the only manifest reader (Constitution 3.4), so v1 and
// v2 plugin.json files both work and a v1 file and its v2 twin give one result.
// Inputs: the root of a tier tree (free/ or paid/): one directory per plugin
// holding plugin.json. Directories starting with "_" or "." are not plugins and
// are skipped with a note. Outputs: manifests by slug and a report.
// Constraints: the directory name must equal the manifest name (a registry key
// is the slug released CLIs install by); a file carrying both minNselfVersion
// and min_nself_version with different values is refused, not guessed; every
// problem names the full path and is reported, sorted.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

// scanManifests loads <root>/<slug>/plugin.json for every slug directory.
func scanManifests(root string) (map[string]*manifestv2.Manifest, *report) {
	rep := &report{}
	ents, err := os.ReadDir(root)
	if err != nil {
		rep.problem("plugins: %v", err)
		return nil, rep
	}
	out := map[string]*manifestv2.Manifest{}
	for _, e := range ents { // ReadDir returns entries sorted by name
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
			rep.note("skipped directory %s (names starting with _ or . are not plugins)", filepath.Join(root, e.Name()))
			continue
		}
		path := filepath.Join(root, e.Name(), "plugin.json")
		data, err := os.ReadFile(path)
		if err != nil {
			rep.problem("%s: not readable (%v)", path, err)
			continue
		}
		m, err := manifestv2.ParseQuiet(data)
		if err != nil {
			rep.problem("%s: %v", path, err)
			continue
		}
		if m.Name != e.Name() {
			rep.problem("%s: manifest name %q differs from its directory", path, m.Name)
			continue
		}
		if msg := conflictingMinVersion(data); msg != "" {
			rep.problem("%s: %s", path, msg)
			continue
		}
		out[m.Name] = m
	}
	return out, rep
}

// conflictingMinVersion reports a manifest that states its minimum nself
// version twice with different values. manifestv2 reads minNselfVersion only, so
// picking one silently would change what released CLIs enforce.
func conflictingMinVersion(data []byte) string {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return ""
	}
	var camel, snake string
	_, hasCamel := raw["minNselfVersion"]
	_, hasSnake := raw["min_nself_version"]
	if !hasCamel || !hasSnake {
		return ""
	}
	if json.Unmarshal(raw["minNselfVersion"], &camel) != nil || json.Unmarshal(raw["min_nself_version"], &snake) != nil || camel == snake {
		return ""
	}
	return fmt.Sprintf("minNselfVersion %q conflicts with min_nself_version %q; keep one", camel, snake)
}
