package build

// Purpose: sweeps every docker-compose.plugin.yml shipped in the sibling
// plugins/ and plugins-pro/ source repos through the exact normalization
// pipeline DiscoverPluginComposeFiles (plugins.go) runs on install, and
// asserts that no service's `dockerfile:` value changes except for the
// three plugins already known to need the source-repo-layout rewrite
// (cron, push, nself-alert-router) — the regression guard for the
// "Dockerfile.golang" -> "Dockerfilelang" corruption (E2E golden path step
// 13; see plugins_dockerfile_test.go for the isolated unit-level repro).
// Inputs: the sibling repos at /Volumes/UG/Sites/nself/{plugins,plugins-pro}
// on this machine's checkout layout — read-only, copied into a t.TempDir()
// before any normalization runs.
// Outputs: t.Fatalf naming every plugin whose dockerfile: value(s) drifted
// unexpectedly, with before/after values.
// Constraints: this package must never depend on those sibling repos at
// build time (see plugins_build_context_test.go's header) — this test reads
// them only at `go test` runtime, skips cleanly when they are not present
// (e.g. a CI checkout of the cli repo alone), and never writes back to them.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// sweepPluginSourceRoots are the sibling repos' plugin trees, read-only.
var sweepPluginSourceRoots = []string{
	"/Volumes/UG/Sites/nself/plugins",
	"/Volumes/UG/Sites/nself/plugins-pro",
}

// sweepKnownEscapingPlugins are the only plugins whose docker-compose.plugin.yml
// was authored with a source-repo-relative build.context (defect #10 / E2E
// golden path step 13) and therefore legitimately gets its dockerfile: value
// rewritten by normalizeComposeBuildContext.
var sweepKnownEscapingPlugins = map[string]bool{
	"cron":               true,
	"push":               true,
	"nself-alert-router": true,
}

// sweepLegacyDockerfileNames are the stale names normalizeComposeDockerfile
// (plugins.go) cleans up from the Rust->Go migration.
var sweepLegacyDockerfileNames = map[string]bool{
	"Dockerfile.go":     true,
	"Dockerfile.golang": true,
	"Dockerfile.rust":   true,
}

// sweepDockerfileValueRE extracts every "dockerfile: <value>" scalar from a
// compose fragment, independent of indentation.
var sweepDockerfileValueRE = regexp.MustCompile(`(?m)^\s*dockerfile:\s*(\S+)\s*$`)

func dockerfileValues(content []byte) []string {
	matches := sweepDockerfileValueRE.FindAllSubmatch(content, -1)
	vals := make([]string, 0, len(matches))
	for _, m := range matches {
		vals = append(vals, string(m[1]))
	}
	sort.Strings(vals)
	return vals
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSweepAllShippedPluginComposeDockerfileValuesStable is the full-corpus
// regression guard: every docker-compose.plugin.yml under the sibling
// plugins/ and plugins-pro/ repos, run through the exact pipeline
// DiscoverPluginComposeFiles applies on install, must come out with the same
// set of `dockerfile:` values it went in with — except for
// sweepKnownEscapingPlugins, which are expected to change (that IS the fix
// defect #10 shipped).
func TestSweepAllShippedPluginComposeDockerfileValuesStable(t *testing.T) {
	for _, root := range sweepPluginSourceRoots {
		if _, err := os.Stat(root); err != nil {
			t.Skipf("sibling repo %s not present on this checkout — skipping sweep", root)
		}
	}

	type finding struct {
		plugin string
		before []string
		after  []string
	}
	var unexpected []finding
	var sweptCount int
	var changedEscaping []string
	var changedLegacyCleanup []string

	for _, root := range sweepPluginSourceRoots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || d.Name() != pluginComposeFilename {
				return nil
			}
			pluginSrcDir := filepath.Dir(path)
			pluginName := filepath.Base(pluginSrcDir)

			// Flatten the source {free,paid}/<name> layout into the
			// installed <tempDir>/<name> layout, copying only top-level
			// regular files (docker-compose.plugin.yml + any Dockerfile*
			// siblings) — everything the normalization functions read.
			pluginDir := t.TempDir()
			dstDir := filepath.Join(pluginDir, pluginName)
			if err := os.MkdirAll(dstDir, 0755); err != nil {
				t.Fatalf("mkdir %s: %v", dstDir, err)
			}
			entries, err := os.ReadDir(pluginSrcDir)
			if err != nil {
				t.Fatalf("read plugin dir %s: %v", pluginSrcDir, err)
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				b, err := os.ReadFile(filepath.Join(pluginSrcDir, e.Name()))
				if err != nil {
					t.Fatalf("read %s: %v", e.Name(), err)
				}
				if err := os.WriteFile(filepath.Join(dstDir, e.Name()), b, 0644); err != nil {
					t.Fatalf("write %s: %v", e.Name(), err)
				}
			}

			original, err := os.ReadFile(filepath.Join(dstDir, pluginComposeFilename))
			if err != nil {
				t.Fatalf("read copied compose %s: %v", pluginName, err)
			}

			// Exact pipeline order from DiscoverPluginComposeFiles.
			normalized := normalizeComposeDockerfile(original, pluginDir, pluginName)
			normalized = normalizeComposeNetworkAliases(normalized, pluginName)
			normalized = normalizeComposeImageToBuild(normalized, pluginDir, pluginName)
			normalized = normalizeComposeBuildContext(normalized, pluginDir, pluginName)
			normalized = normalizeComposeDropObsoleteVersion(normalized)

			before := dockerfileValues(original)
			after := dockerfileValues(normalized)
			sweptCount++

			if len(before) == 0 {
				// No dockerfile: line existed before normalization — this is
				// an image:-only fragment gaining a build: block via
				// normalizeComposeImageToBuild (already covered by
				// TestNormalizeComposeImageToBuild_* in
				// plugins_image_to_build_test.go), not a dockerfile *value*
				// changing. Out of scope for this sweep.
				return nil
			}
			if equalStringSlices(before, after) {
				return nil
			}
			if sweepKnownEscapingPlugins[pluginName] {
				changedEscaping = append(changedEscaping, pluginName)
				return nil
			}
			// The other legitimate case: the original value was a legacy
			// Rust->Go migration name (Dockerfile.go/.golang/.rust) that does
			// NOT actually exist as a file in the plugin — a real,
			// independent defect in that plugin's shipped compose fragment,
			// which normalizeComposeDockerfile correctly repairs to the
			// canonical "Dockerfile". This is the fix working as intended on
			// a plugin beyond the three known escaping ones, not a
			// regression — see plugins_dockerfile_test.go.
			if len(before) == 1 && len(after) == 1 && after[0] == "Dockerfile" && sweepLegacyDockerfileNames[before[0]] {
				if _, statErr := os.Stat(filepath.Join(dstDir, before[0])); os.IsNotExist(statErr) {
					changedLegacyCleanup = append(changedLegacyCleanup, pluginName+" ("+before[0]+" -> Dockerfile, "+before[0]+" does not exist on disk)")
					return nil
				}
			}
			unexpected = append(unexpected, finding{plugin: pluginName, before: before, after: after})
			return nil
		})
	}

	t.Logf("swept %d shipped plugin compose fragments across %v", sweptCount, sweepPluginSourceRoots)
	t.Logf("dockerfile: values changed for known escaping-context plugins: %v", changedEscaping)
	t.Logf("dockerfile: values changed via legacy-name cleanup (referenced file did not exist): %v", changedLegacyCleanup)

	if len(unexpected) > 0 {
		for _, f := range unexpected {
			t.Errorf("plugin %q: dockerfile: value(s) unexpectedly changed\n  before: %v\n  after:  %v", f.plugin, f.before, f.after)
		}
		t.Fatalf("%d plugin(s) had unexpected dockerfile: drift — only the known escaping-context plugins (%v) and a genuine missing-legacy-file cleanup may change", len(unexpected), sweepKnownEscapingPlugins)
	}

	for name := range sweepKnownEscapingPlugins {
		found := false
		for _, c := range changedEscaping {
			if c == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected known-escaping plugin %q to be found and rewritten by the sweep, but it was not encountered under %v", name, sweepPluginSourceRoots)
		}
	}
}
