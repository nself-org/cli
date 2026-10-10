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

	"github.com/nself-org/cli/internal/errs"
)

// Purpose: doctor finding E136 for contract:cli.bundle-ledger v1 (P7-PLUG-19).
// The bundle install ledger lists a plugin whose <pluginDir>/<slug>/plugin.json
// is gone, so the ledger and the disk disagree.
// Inputs: the plugin directory ($NSELF_PLUGIN_DIR or ~/.nself/plugins).
// Outputs: one failing CheckResult per missing plugin (section "plugins", name
// BUNDLE-LEDGER-DRIFT-<slug>). No ledger file, or a consistent ledger, gives no
// result at all. An unreadable or invalid ledger file gives one warning naming it.
// Constraints: internal/doctor may not import internal/bundle (layers.yaml), so
// this file reads the ledger as a contract consumer: it decodes only
// {"plugins": {<slug>: {...}}} and the bundle names, capped at 4 MiB. The path
// constants mirror ledger.Store.Path (internal/bundle/ledger/store.go).

const (
	ledgerStateDir   = ".nself-state" // ledger.StateDirName
	ledgerFileName   = "bundles.json"
	ledgerReadCap    = 4 << 20
	bundleDriftFixed = "reinstall the bundle (nself bundle install <bundle>), or delete <plugin dir>/.nself-state/bundles.json to re-bootstrap it from the installed plugins"
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
	return results
}

func driftWarn(msg string) CheckResult {
	return CheckResult{Section: "plugins", Name: "BUNDLE-LEDGER-DRIFT-ledger", Status: "warn", Message: msg}
}
