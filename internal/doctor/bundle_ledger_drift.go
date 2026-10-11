package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// Purpose: doctor finding E136 for contract:cli.bundle-ledger v1 (P7-PLUG-19).
// The bundle install ledger lists a plugin whose <pluginDir>/<slug>/plugin.json
// is gone, so the ledger and the disk disagree. The reverse (D-0323): an
// official plugin is on disk but the ledger has no record of it (the ledger
// write after an install failed), which is a warning.
// Inputs: the plugin directory ($NSELF_PLUGIN_DIR or ~/.nself/plugins).
// Outputs: one failing CheckResult per missing plugin (section "plugins", name
// BUNDLE-LEDGER-DRIFT-<slug>) and one warning per unrecorded official plugin
// (BUNDLE-LEDGER-UNRECORDED-<slug>). No ledger file, or a consistent ledger,
// gives no result at all. An unreadable or invalid ledger file gives one
// warning naming it.
// Constraints: internal/doctor may not import internal/bundle (layers.yaml), so
// this file reads the ledger as a contract consumer: it decodes only
// {"plugins": {<slug>: {...}}} and the bundle names, capped at 4 MiB. The path
// constants mirror ledger.Store.Path (internal/bundle/ledger/store.go).

const (
	ledgerStateDir   = ".nself-state" // ledger.StateDirName
	ledgerFileName   = "bundles.json"
	ledgerReadCap    = 4 << 20
	bundleDriftFixed = "reinstall the bundle (nself bundle install <bundle>) to restore the missing plugin"
	manifestReadCap  = 1 << 20
)

// bundlePluginDir is where plugins (and the ledger) live: $NSELF_PLUGIN_DIR
// first, then ~/.nself/plugins; "" when neither can be resolved.
func bundlePluginDir() string {
	if d := os.Getenv("NSELF_PLUGIN_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".nself", "plugins")
}

// BundleLedgerDriftChecks is the DeepChecks entry point.
func BundleLedgerDriftChecks(_ context.Context, pluginDir string) []CheckResult {
	if pluginDir == "" {
		return nil
	}
	path := filepath.Join(pluginDir, ledgerStateDir, ledgerFileName)
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []CheckResult{driftWarn(fmt.Sprintf("cannot read the bundle ledger %s: %v", path, err))}
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, ledgerReadCap+1))
	if err != nil || len(data) > ledgerReadCap {
		return []CheckResult{driftWarn(fmt.Sprintf("cannot read the bundle ledger %s (unreadable or over %d bytes)", path, ledgerReadCap))}
	}
	var doc struct {
		Plugins map[string]struct {
			InstalledBy []string `json:"installed_by"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return []CheckResult{driftWarn(fmt.Sprintf("the bundle ledger %s is not valid JSON: %v", path, err))}
	}
	slugs := make([]string, 0, len(doc.Plugins))
	for slug := range doc.Plugins {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	var results []CheckResult
	for _, slug := range slugs {
		if filepath.Base(slug) != slug || slug == "." || slug == ".." {
			continue // not a plugin slug; never join it into a path
		}
		if _, err := os.Stat(filepath.Join(pluginDir, slug, "plugin.json")); !errors.Is(err, os.ErrNotExist) {
			continue // present (or unreadable for another reason: not drift)
		}
		e := errs.Newf("E136", "bundle ledger lists plugin %s but %s is gone", slug, filepath.Join(pluginDir, slug, "plugin.json"))
		results = append(results, CheckResult{Section: "plugins", Name: "BUNDLE-LEDGER-DRIFT-" + slug, Status: "fail",
			Message: "E136: " + e.What, FixCmd: bundleDriftFixed})
	}
	recorded := make(map[string]bool, len(doc.Plugins))
	for slug := range doc.Plugins {
		recorded[strings.ToLower(slug)] = true
	}
	return append(results, unrecordedPluginChecks(pluginDir, recorded)...)
}

// unrecordedPluginChecks warns about each official plugin in pluginDir that the
// ledger does not record (D-0323). The ledger key is the plugin.json name in
// lower case, the same key `nself plugin install` and the bootstrap write. A
// plugin from a third-party URL is never recorded by design, so only official
// plugins (author nself or nself-org, or a github.com/nself-org repository) are
// reported. The author field is self-declared: this decides only whether a
// warning is shown, never what is installed or trusted. Dot directories (the
// ledger's own .nself-state) and directories without a readable plugin.json are
// skipped: they are not installed plugins.
func unrecordedPluginChecks(pluginDir string, recorded map[string]bool) []CheckResult {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil
	}
	var results []CheckResult
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		m, ok := readPluginIdentity(filepath.Join(pluginDir, e.Name(), "plugin.json"))
		if !ok || !m.official() {
			continue
		}
		slug := strings.ToLower(strings.TrimSpace(m.Name))
		if slug == "" {
			slug = strings.ToLower(e.Name())
		}
		if recorded[slug] {
			continue
		}
		results = append(results, CheckResult{Section: "plugins", Name: "BUNDLE-LEDGER-UNRECORDED-" + slug, Status: "warn",
			Message: fmt.Sprintf("plugin %s is installed in %s but the bundle ledger has no record of it (a ledger write after its install failed), so bundle state is wrong", slug, filepath.Join(pluginDir, e.Name())),
			FixCmd:  fmt.Sprintf("nself plugin install %s again records it", slug)})
	}
	return results
}

// pluginIdentity is the part of plugin.json the unrecorded check reads.
type pluginIdentity struct {
	Name       string `json:"name"`
	Author     string `json:"author"`
	Repository string `json:"repository"`
}

// official reports whether the manifest names nSelf as its publisher.
func (m pluginIdentity) official() bool {
	switch strings.ToLower(strings.TrimSpace(m.Author)) {
	case "nself", "nself-org":
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Repository)), "https://github.com/nself-org/")
}

// readPluginIdentity decodes plugin.json, capped at manifestReadCap; ok is
// false when it is missing, unreadable, too large or not a JSON object.
func readPluginIdentity(path string) (pluginIdentity, bool) {
	f, err := os.Open(path)
	if err != nil {
		return pluginIdentity{}, false
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, manifestReadCap+1))
	if err != nil || len(data) > manifestReadCap {
		return pluginIdentity{}, false
	}
	var m pluginIdentity
	if json.Unmarshal(data, &m) != nil {
		return pluginIdentity{}, false
	}
	return m, true
}

func driftWarn(msg string) CheckResult {
	return CheckResult{Section: "plugins", Name: "BUNDLE-LEDGER-DRIFT-ledger", Status: "warn", Message: msg}
}
