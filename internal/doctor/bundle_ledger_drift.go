package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

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
// (BUNDLE-LEDGER-UNRECORDED-<slug>, the command to run in its message). No ledger file, or a consistent ledger,
// gives no result at all. An unreadable or invalid ledger file gives one
// warning naming it.
// Constraints: internal/doctor may not import internal/bundle (layers.yaml; the
// ledger package is in that layer), so this file reads the ledger as a contract
// consumer: parseLedgerShape (D-0474) checks the same shape ledger.Parse
// requires of the one key it reads, "plugins", capped at 4 MiB. The path constants mirror
// ledger.Store.Path (internal/bundle/ledger/store.go).

const (
	ledgerStateDir   = ".nself-state" // ledger.StateDirName
	ledgerFileName   = "bundles.json"
	ledgerReadCap    = 4 << 20
	bundleDriftFixed = "reinstall the bundle (nself bundle install <bundle>) to restore the missing plugin"
	manifestReadCap  = 1 << 20
)

// ledgerSlugRE mirrors ledger.SlugPattern (internal/bundle/ledger/ledger.go):
// only such a name can be a ledger key, and only such a name is safe to print
// into a check name or a command hint.
var ledgerSlugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

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
	doc, err := parseLedgerShape(data)
	if err != nil {
		return []CheckResult{driftWarn(fmt.Sprintf("the bundle ledger %s is not valid: %v", path, err))}
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
		// ENOTDIR (the plugin directory was replaced by a file) is as gone as ENOENT.
		if _, err := os.Stat(filepath.Join(pluginDir, slug, "plugin.json")); !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
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
		if !ledgerSlugRE.MatchString(slug) || recorded[slug] {
			continue // an invalid slug can never be recorded (the bootstrap skips it too)
		}
		// No FixCmd on purpose: doctor --fix runs an "nself ..." FixCmd, and a
		// reinstall (a download and a write) is the owner's call, not doctor's.
		results = append(results, CheckResult{Section: "plugins", Name: "BUNDLE-LEDGER-UNRECORDED-" + slug, Status: "warn",
			Message: fmt.Sprintf("plugin %s is installed in %q but the bundle ledger has no record of it (a ledger write after its install failed), so bundle state is wrong; run nself plugin install %s again to record it", slug, filepath.Join(pluginDir, e.Name()), slug)})
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

// ledgerDoc is the part of the ledger this check reads.
type ledgerDoc struct {
	Plugins map[string]struct {
		InstalledBy []string `json:"installed_by"`
	} `json:"plugins"`
}

// parseLedgerShape decodes the ledger and refuses what the writers' strict
// parser (ledger.Parse) would refuse among the keys read here: a document that
// is not an object (null, an array), a missing or null "plugins"
// (so {} and {"plugins":null} are damage, not a clean ledger), a "plugins" that
// is not an object, and a plugin key that is not a valid slug.
func parseLedgerShape(data []byte) (ledgerDoc, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return ledgerDoc{}, errors.New("not a JSON object")
	}
	if v := strings.TrimSpace(string(root["plugins"])); v == "" || v == "null" {
		return ledgerDoc{}, errors.New(`"plugins" is missing or null`)
	}
	var doc ledgerDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return ledgerDoc{}, err
	}
	for slug := range doc.Plugins {
		if !ledgerSlugRE.MatchString(slug) {
			return ledgerDoc{}, fmt.Errorf("plugin key %q is not a valid slug", slug)
		}
	}
	return doc, nil
}
