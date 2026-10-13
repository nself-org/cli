package commands

// Purpose: autoInstallPlugin, the non-interactive, verified install seam for a
// command plugin that the CLI itself needs and does not find: the v1.4
// `nself-ci` bootstrap (P7-CANON-24) and the v1.5 break-out auto-add
// (P7-CANON-13). It downloads and executes remote code, so it is CRITICAL.
// Inputs: the plugin slug and an optional version ("" = the catalogue's current).
// Outputs: the path of the published command binary, or a coded error that
// names `nself add <slug>` as the manual route.
// Constraints: refuses, in both compat modes and before anything is downloaded,
// a catalogue entry with no release_signature, and any entry of a tier whose
// key is not pinned in this CLI (an unpinned tier cannot verify). Free tier
// only. The registry is the configured one (NSELF_PLUGIN_REGISTRY or the
// default); nothing is resolved through PATH. For a signed entry it relies on
// plugin.Install's order: checksum, then signature.Verify (E121-E124), then the
// tar-slip-guarded extraction and linkCLIBinary. No prompt is ever shown.
// Not goroutine-safe: it sets installer env vars (tier, strict, no bypass) for the call.
// No caller yet: P7-CANON-13 and P7-CANON-24 call it.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/verify/signature"
)

// autoInstallTierEnv is the tier override plugin.Install reads; the seam pins it
// to free so its pre-check and the installer resolve the same registry entry.
const autoInstallTierEnv = "NSELF_PLUGIN_INSTALL_TIER"

// autoInstallPlugin installs the free command plugin slug (at version, when
// given) and returns the path of its published binary.
func autoInstallPlugin(slug, version string) (string, error) {
	// Load the project config first: it exports the project's .env into the
	// process, and must not be able to overwrite the policy set below.
	cfg, err := loadConfig()
	if err != nil {
		cfg = &config.Config{} // no project here: a bootstrap runs anywhere
	}
	// The caller (a bootstrap) runs no `nself build`, and the installer's hint to do so is noise here.
	ctx := plugin.WithCallerReconciles(context.Background())
	manifest, reg, err := autoInstallResolve(ctx, slug, version)
	if err != nil {
		return "", err
	}

	// plugin.Install fetches the catalogue again. Pin its tier to free, make a
	// missing signature a refusal in 1.4 mode too, and drop the licence bypass,
	// so a catalogue that changes between the two fetches cannot be installed
	// unverified.
	restore, err := autoInstallSetEnv(map[string]string{
		autoInstallTierEnv:                "free",
		"NSELF_PLUGIN_REQUIRE_CHECKSUM":   "1",
		"NSELF_LICENSE_SKIP_VERIFY":       "",
		"NSELF_LICENSE_SKIP_VERIFY_FORCE": "",
		"NSELF_PLUGIN_ALLOW_UNSIGNED":     "",
	})
	defer restore()
	if err != nil {
		return "", fmt.Errorf("auto-install of %q: %w", slug, err)
	}

	// Install fetches the catalogue itself: hand it the one just checked, so a
	// cache that expired or a registry that changed in between cannot swap it.
	if err := plugin.WriteRegistryCache(plugin.DefaultCacheDir(), reg); err != nil {
		return "", fmt.Errorf("auto-install of %q: pinning the checked catalogue: %w (install it by hand: nself add %s)", slug, err, slug)
	}
	// Extraction writes into an existing plugin directory, so a stale file there
	// (an nself-ci the signed archive does not contain) could be published as if
	// verified. Move a leftover directory aside, never delete it, and put it back
	// if the install fails.
	pluginDir := resolvePluginDir()
	undo, err := autoInstallMoveAside(pluginDir, slug)
	if err != nil {
		return "", fmt.Errorf("auto-install of %q: %w (install it by hand: nself add %s)", slug, err, slug)
	}
	if err := plugin.Install(ctx, cfg, slug, pluginDir); err != nil {
		undo()
		return "", fmt.Errorf("auto-install of %q %s failed: %w (install it by hand: nself add %s)", slug, manifest.Version, err, slug)
	}

	bin := plugin.PublishedBinaryPath(autoInstallBinaryName(slug, manifest))
	if got := readInstalledVersion(pluginDir, slug); got != manifest.Version {
		// Unreadable or different: the catalogue changed under us. Remove what this
		// call published (by the checked name; UndoInstall uses the archive's).
		_ = plugin.UndoInstall(ctx, cfg, slug, pluginDir)
		_ = os.Remove(bin)
		undo()
		return "", fmt.Errorf("auto-install of %q: installed version %q but the checked catalogue entry said %s; the catalogue changed during the install (install it by hand: nself add %s)", slug, got, manifest.Version, slug)
	}
	if st, err := os.Stat(bin); err != nil || st.IsDir() {
		return "", fmt.Errorf("auto-install of %q: the installer reported success but %s was not published (install it by hand: nself add %s)", slug, bin, slug)
	}
	return bin, nil
}

// autoInstallResolve picks the registry entry and applies every refusal that
// must happen before a download: not a free command plugin, version not
// served, no release_signature, or no pinned key for the tier.
func autoInstallResolve(ctx context.Context, slug, version string) (*plugin.PluginManifest, *plugin.Registry, error) {
	if err := plugin.ValidateName(slug); err != nil {
		return nil, nil, err
	}
	manual := fmt.Sprintf("nself add %s", slug)
	reg, err := plugin.FetchRegistry(ctx, "", plugin.DefaultCacheDir())
	if err != nil {
		return nil, nil, fmt.Errorf("auto-install of %q: fetching the catalogue: %w (install it by hand: %s)", slug, err, manual)
	}
	m, _, err := plugin.ResolvePluginTier(ctx, reg, slug, "free", "", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("auto-install of %q: %w (install it by hand: %s)", slug, err, manual)
	}
	if m.PluginType != "cli" {
		return nil, nil, fmt.Errorf("auto-install of %q: the catalogue entry is not a command plugin (install it by hand: %s)", slug, manual)
	}
	if m.RequiresLicense || m.IsCommercial || (m.Tier != "" && m.Tier != plugin.WireTierFree) {
		return nil, nil, fmt.Errorf("auto-install of %q: the catalogue entry is licensed, and only free plugins are auto-installed (install it by hand: %s)", slug, manual)
	}
	if !autoInstallSafeBinaryName(autoInstallBinaryName(slug, m)) {
		return nil, nil, fmt.Errorf("auto-install of %q: the catalogue entry names an unsafe command binary %q (install it by hand: %s)", slug, autoInstallBinaryName(slug, m), manual)
	}
	if len(m.Dependencies) > 0 || len(m.OptionalDependencies) > 0 {
		return nil, nil, fmt.Errorf("auto-install of %q: the entry has dependencies, which the installer would add before verifying it (install it by hand: %s)", slug, manual)
	}
	if want := strings.TrimPrefix(version, "v"); want != "" && want != strings.TrimPrefix(m.Version, "v") {
		return nil, nil, fmt.Errorf("auto-install of %q: the catalogue serves %s, not the requested %s (install it by hand: %s)", slug, m.Version, version, manual)
	}
	if m.ReleaseSignature == nil {
		return nil, nil, errs.Newf("E107", "plugin %q %s has no release signature, so it was not auto-installed", slug, m.Version).
			WithFix("Run `" + manual + "` once a signed release exists.")
	}
	if !signature.Pinned(signature.TierFree) {
		return nil, nil, errs.Newf("E107", "plugin %q %s cannot be verified: this CLI pins no free-tier release key, so it was not auto-installed", slug, m.Version).
			WithFix("Run `" + manual + "` with a CLI release that pins the key.")
	}
	return m, reg, nil
}

// autoInstallBinaryName is the name the installer publishes a single-command
// plugin under: the manifest's binaryName, else nself-<slug>.
func autoInstallBinaryName(slug string, m *plugin.PluginManifest) string {
	if m.BinaryName != "" {
		return m.BinaryName
	}
	return "nself-" + slug
}

// autoInstallSetEnv sets each variable and returns a func that puts the
// previous values back (unsetting the ones that were absent).
func autoInstallSetEnv(vars map[string]string) (func(), error) {
	type prev struct {
		val string
		had bool
	}
	saved := map[string]prev{}
	restore := func() {
		for k, p := range saved {
			if p.had {
				_ = os.Setenv(k, p.val)
			} else {
				_ = os.Unsetenv(k)
			}
		}
	}
	for k, v := range vars {
		old, had := os.LookupEnv(k)
		saved[k] = prev{old, had}
		if err := os.Setenv(k, v); err != nil {
			return restore, err
		}
	}
	return restore, nil
}

// readInstalledVersion returns the version in <pluginDir>/<slug>/plugin.json, or "".
func readInstalledVersion(pluginDir, slug string) string {
	b, err := os.ReadFile(filepath.Join(pluginDir, slug, "plugin.json"))
	if err != nil {
		return ""
	}
	var m struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	return m.Version
}

// autoInstallMoveAside renames <pluginDir>/<slug> to ../plugins.aside/<slug>.pre-autoinstall-<ts>
// when it exists (and no install holds the lock), and returns a func that puts
// it back if nothing took its place. With nothing there the func does nothing.
func autoInstallMoveAside(pluginDir, slug string) (func(), error) {
	pluginDir = filepath.Clean(pluginDir)
	dir := filepath.Join(pluginDir, slug)
	if _, err := os.Stat(dir); err != nil {
		return func() {}, nil
	}
	// Another installer holds the lock: do not rename a tree it may be writing.
	if _, err := os.Stat(filepath.Join(pluginDir, ".install.lock")); err == nil {
		return nil, fmt.Errorf("another plugin install is running in %s", pluginDir)
	}
	// Outside pluginDir, so plugin discovery never lists the leftover as a plugin.
	asideRoot := filepath.Join(filepath.Dir(pluginDir), "plugins.aside")
	if err := os.MkdirAll(asideRoot, 0o700); err != nil {
		return nil, fmt.Errorf("creating %s: %w", asideRoot, err)
	}
	aside := filepath.Join(asideRoot, fmt.Sprintf("%s.pre-autoinstall-%d", slug, time.Now().UnixNano()))
	if err := os.Rename(dir, aside); err != nil {
		return nil, fmt.Errorf("moving the existing %s aside: %w", dir, err)
	}
	// Put it back only when nothing took its place, and never delete what is there.
	return func() {
		if _, err := os.Stat(dir); err != nil {
			_ = os.Rename(aside, dir)
		}
	}, nil
}

// autoInstallSafeBinaryName reports whether name is a plain nself-<x> file name:
// no separator, no dot-dot, so publishing or removing it stays in the bin dir.
func autoInstallSafeBinaryName(name string) bool {
	if !strings.HasPrefix(name, "nself-") || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return false
	}
	for _, r := range name {
		if r != '-' && r != '_' && r != '.' && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}
