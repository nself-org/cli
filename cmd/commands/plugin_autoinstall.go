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
// Not goroutine-safe: it sets NSELF_PLUGIN_INSTALL_TIER for the call.
// No caller yet: P7-CANON-13 and P7-CANON-24 call it.

import (
	"context"
	"fmt"
	"os"
	"strings"

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
	ctx := plugin.WithCallerReconciles(context.Background())
	manifest, err := autoInstallResolve(ctx, slug, version)
	if err != nil {
		return "", err
	}

	if prev, had := os.LookupEnv(autoInstallTierEnv); had {
		defer func() { _ = os.Setenv(autoInstallTierEnv, prev) }()
	} else {
		defer func() { _ = os.Unsetenv(autoInstallTierEnv) }()
	}
	if err := os.Setenv(autoInstallTierEnv, "free"); err != nil {
		return "", fmt.Errorf("auto-install of %q: %w", slug, err)
	}

	cfg, err := loadConfig()
	if err != nil {
		cfg = &config.Config{} // no project here: a bootstrap runs anywhere
	}
	if err := plugin.Install(ctx, cfg, slug, resolvePluginDir()); err != nil {
		return "", fmt.Errorf("auto-install of %q %s failed: %w (install it by hand: nself add %s)", slug, manifest.Version, err, slug)
	}

	bin := plugin.PublishedBinaryPath(autoInstallBinaryName(slug, manifest))
	if st, err := os.Stat(bin); err != nil || st.IsDir() {
		return "", fmt.Errorf("auto-install of %q: the installer reported success but %s was not published (install it by hand: nself add %s)", slug, bin, slug)
	}
	return bin, nil
}

// autoInstallResolve picks the registry entry and applies every refusal that
// must happen before a download: not a free command plugin, version not
// served, no release_signature, or no pinned key for the tier.
func autoInstallResolve(ctx context.Context, slug, version string) (*plugin.PluginManifest, error) {
	if err := plugin.ValidateName(slug); err != nil {
		return nil, err
	}
	manual := fmt.Sprintf("nself add %s", slug)
	reg, err := plugin.FetchRegistry(ctx, "", plugin.DefaultCacheDir())
	if err != nil {
		return nil, fmt.Errorf("auto-install of %q: fetching the catalogue: %w (install it by hand: %s)", slug, err, manual)
	}
	m, _, err := plugin.ResolvePluginTier(ctx, reg, slug, "free", "", nil)
	if err != nil {
		return nil, fmt.Errorf("auto-install of %q: %w (install it by hand: %s)", slug, err, manual)
	}
	if m.PluginType != "cli" {
		return nil, fmt.Errorf("auto-install of %q: the catalogue entry is not a command plugin (install it by hand: %s)", slug, manual)
	}
	if want := strings.TrimPrefix(version, "v"); want != "" && want != strings.TrimPrefix(m.Version, "v") {
		return nil, fmt.Errorf("auto-install of %q: the catalogue serves %s, not the requested %s (install it by hand: %s)", slug, m.Version, version, manual)
	}
	if m.ReleaseSignature == nil {
		return nil, errs.Newf("E107", "plugin %q %s has no release signature, so it was not auto-installed", slug, m.Version).
			WithFix("Run `" + manual + "` once a signed release exists.")
	}
	if !signature.Pinned(signature.TierFree) {
		return nil, errs.Newf("E107", "plugin %q %s cannot be verified: this CLI pins no free-tier release key, so it was not auto-installed", slug, m.Version).
			WithFix("Run `" + manual + "` with a CLI release that pins the key.")
	}
	return m, nil
}

// autoInstallBinaryName is the name the installer publishes a single-command
// plugin under: the manifest's binaryName, else nself-<slug>.
func autoInstallBinaryName(slug string, m *plugin.PluginManifest) string {
	if m.BinaryName != "" {
		return m.BinaryName
	}
	return "nself-" + slug
}
