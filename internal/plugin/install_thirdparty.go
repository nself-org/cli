package plugin

// Purpose: Install a plugin directly from an arbitrary HTTPS URL, bypassing
//          the official plugins.nself.org registry entirely (CLI-R16).
// Inputs:  context.Context, *config.Config, the source URL, pluginDir path,
//          and an optional expected SHA-256 checksum for the downloaded
//          archive.
// Outputs: error on failure; side effect installs the plugin directory and
//          database schema, same as a registry install.
// Constraints: "official by name, third-party by URL" contract —
//   - `nself plugin install <name>` resolves against the registry and is
//     Ed25519-signature verified against a registry-pinned author key whose
//     checksum comes from the registry entry — a trust anchor external to
//     the downloaded tarball.
//   - `nself plugin install <url>` NEVER touches the registry and is NOT
//     signature verified — there is no registry-pinned key to check an
//     arbitrary URL's tarball against. Checksum verification requires the
//     SAME kind of external anchor: the extracted plugin.json's own
//     "checksum" field is deliberately NOT used for this, because it lives
//     inside the very archive it would be checksumming — a self-referential
//     check that can never meaningfully fail. Callers pass the expected
//     checksum out-of-band instead (e.g. `--checksum <hex>`, copied by the
//     user from the plugin's README or release notes), the same pattern
//     `pip install --hash=` and Homebrew formulas use. An empty checksum
//     skips verification with a visible warning.
//     In v1.5 (compat.V15) that makes the install an E107 refusal unless the
//     caller opts in with --allow-unsigned (NSELF_PLUGIN_ALLOW_UNSIGNED=1),
//     and an archive that names a plugin the registry serves is refused (E106)
//     so a URL can never replace an official plugin's directory.
//   - The interactive warning + confirmation (naming the source host) is the
//     CALLER's responsibility (cmd/commands/plugin.go), so it can honour
//     --yes for non-interactive/CI use without this package depending on a
//     terminal.
//   - HTTPS only; plain HTTP is rejected except for localhost/127.0.0.1 (dev
//     and test use), mirroring enforceRegistryHTTPS in registry.go.
// SPORT: install pipeline; callers: cmd/commands/plugin.go (runPluginInstall)

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/version"
)

// thirdPartyRegistry reads the official registry for the name check. A seam so
// tests stay off the network.
var thirdPartyRegistry = func(ctx context.Context) (*Registry, error) {
	return FetchRegistry(ctx, "", defaultCacheDir())
}

// checkNotRegistryPlugin refuses a URL archive that names a plugin the
// official registry serves (E106): the install would replace the directory of
// the signed official plugin with unsigned code. When the registry cannot be
// read the name cannot be checked: v1.5 refuses, v1.4 warns and goes on.
func checkNotRegistryPlugin(ctx context.Context, name string) error {
	reg, err := thirdPartyRegistry(ctx)
	if err != nil {
		// compat.V15(D-0842): an unreadable registry skips the official-name check with a warning -> refused
		if compat.V15() {
			return errs.Wrap("E106", fmt.Sprintf("cannot check whether %q is an official plugin: the registry is unreadable", name), err)
		}
		fmt.Fprintf(os.Stderr, "warning: could not read the plugin registry (%v); not checking whether %q is an official plugin\n", err, name)
		return nil
	}
	if _, ok := FindPluginByName(reg, name); ok {
		return errs.Newf("E106", "%q is an official registry plugin: a URL install cannot replace it (install it by name: nself plugin install %s)", name, name)
	}
	return nil
}

// IsThirdPartyInstallSource reports whether ref looks like a URL rather than
// a plain registry plugin name. Plugin names are validated elsewhere against
// namePattern (lowercase-with-hyphens, no scheme), so any ref containing a
// "://" is unambiguously a URL, not a name.
func IsThirdPartyInstallSource(ref string) bool {
	return strings.Contains(ref, "://")
}

// ValidateThirdPartyURL parses ref and enforces the transport-security policy
// for third-party plugin sources: HTTPS only, except for localhost/127.0.0.1
// (dev and integration-test use, matching enforceRegistryHTTPS's convention
// for the official registry URL).
func ValidateThirdPartyURL(ref string) (*url.URL, error) {
	u, err := url.Parse(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid plugin source URL %q: %w", ref, err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("plugin source URL %q has no host", ref)
	}
	switch u.Scheme {
	case "https":
		// allowed
	case "http":
		if u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" {
			return nil, fmt.Errorf("third-party plugin sources must use https:// (got %q) — refusing to download plugin code over an insecure channel", ref)
		}
	default:
		return nil, fmt.Errorf("unsupported scheme %q in plugin source URL %q — only https:// is accepted", u.Scheme, ref)
	}
	return u, nil
}

// InstallFromURL downloads a plugin tarball from an arbitrary HTTPS URL,
// extracts it, validates its bundled plugin.json, and installs it exactly
// like a registry plugin (schema creation, route/table conflict checks)
// EXCEPT for the registry-specific trust steps that don't apply to a source
// with no registry entry: license check (there is no license tier for a URL)
// and Ed25519 signature verification (there is no registry-pinned author key
// to verify against). Checksum verification still runs against
// expectedChecksum when the caller supplies one (see the package doc comment
// above for why that must come from outside the tarball, not from the
// extracted plugin.json).
//
// Callers MUST obtain user confirmation (or an explicit non-interactive
// opt-in such as --yes) BEFORE calling this — it downloads and executes
// arbitrary third-party code with no vetting beyond what's described above.
func InstallFromURL(ctx context.Context, cfg *config.Config, sourceURL string, pluginDir string, expectedChecksum string) error {
	u, err := ValidateThirdPartyURL(sourceURL)
	if err != nil {
		return err
	}
	// compat.V15(D-0842): a URL install has no release signature and installs with a warning -> refused with E107 unless --allow-unsigned
	if compat.V15() && os.Getenv(allowUnsignedEnv) != "1" {
		return errs.Newf("E107", "plugin from %s has no release signature; a URL install is unsigned. Pass --allow-unsigned to install it anyway", u.Host)
	}

	lock, err := acquireInstallLock(pluginDir)
	if err != nil {
		return err
	}
	defer releaseInstallLock(lock, pluginDir)

	archivePath, err := downloadFromURL(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("downloading third-party plugin from %s: %w", u.Host, err)
	}
	defer func() { _ = os.Remove(archivePath) }()

	// Checksum policy for unofficial sources: verify ONLY when the caller
	// supplied an expected value (out-of-band, e.g. --checksum). Checked
	// against the raw downloaded archive, before extraction, exactly like the
	// registry install path. verifyChecksum's "stable status requires a
	// checksum" gate is a registry-trust rule and does not apply here — an
	// empty publishStatus argument keeps that gate off.
	if expectedChecksum != "" {
		if err := verifyChecksum(archivePath, expectedChecksum, ""); err != nil {
			return fmt.Errorf("checksum verification for third-party plugin from %s: %w", u.Host, err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "warning: no checksum supplied for %s — file integrity is unverified (pass --checksum <sha256> if the source publishes one)\n", u.Host)
	}

	// Stage the extraction inside pluginDir (not os.TempDir) so the final
	// os.Rename into place below is a same-filesystem rename, not a
	// cross-device copy — pluginDir is guaranteed to exist at this point
	// because acquireInstallLock creates it.
	stagingDir, err := os.MkdirTemp(pluginDir, ".staging-thirdparty-*")
	if err != nil {
		return fmt.Errorf("creating staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	if err := extractTarGz(archivePath, stagingDir); err != nil {
		return fmt.Errorf("extracting third-party plugin archive from %s: %w", u.Host, err)
	}

	// We don't know the plugin's name until after extraction, which is why
	// this can't reuse installLocked's name-first flow. parseManifest runs
	// the full manifest validation (name format, semver, np_ table prefix,
	// permission allowlist) — same rules as a registry plugin. Note:
	// manifest.Checksum (if the author's plugin.json sets it) is NOT used for
	// tarball integrity here — see the package doc comment for why.
	manifest, err := parseManifest(filepath.Join(stagingDir, "plugin.json"))
	if err != nil {
		return fmt.Errorf("third-party plugin at %s: %w", u.Host, err)
	}

	// No signature verification: third-party sources have no registry-pinned
	// author key to check Signature/AuthorPublicKey against, even if the
	// manifest happens to carry those fields.

	if err := CheckCLICompat(manifest.Compat, version.GetVersion()); err != nil {
		return fmt.Errorf("compatibility check failed for %q: %w", manifest.Name, err)
	}
	if err := checkTablePrefixConflict(pluginDir, manifest.Name, manifest.Tables); err != nil {
		return err
	}
	if err := checkTableConflicts(pluginDir, manifest); err != nil {
		return err
	}
	existingRoutes := collectInstalledPluginRoutes(pluginDir, manifest.Name)
	if err := checkPluginRouteConflict(manifest, existingRoutes); err != nil {
		return err
	}

	if err := ValidateName(manifest.Name); err != nil {
		return err
	}
	if err := checkNotRegistryPlugin(ctx, manifest.Name); err != nil {
		return err
	}
	// D-0259: refuse a command the proxy cannot serve or another installed
	// plugin already publishes while the download is still staged, before the
	// installed copy is removed. A refusal after the swap would roll back, and
	// so remove, an upgrade.
	if err := checkCLIBinaries(pluginDir, manifest.Name, manifest); err != nil {
		return err
	}
	destDir := filepath.Join(pluginDir, manifest.Name)
	if _, statErr := os.Stat(destDir); statErr == nil {
		if err := os.RemoveAll(destDir); err != nil {
			return fmt.Errorf("removing existing plugin directory %q for reinstall: %w", manifest.Name, err)
		}
	}
	if err := os.Rename(stagingDir, destDir); err != nil {
		return fmt.Errorf("installing third-party plugin %q: %w", manifest.Name, err)
	}

	// Publish the command binary, same as the registry install path. Without
	// this a third-party CLI plugin installs cleanly and its command does not
	// exist — the exact failure the registry path was fixed for, sitting
	// unfixed in the parallel path because nobody had installed a third-party
	// plugin end to end.
	if err := linkCLIBinary(destDir, manifest.Name, manifest); err != nil {
		rollbackInstall(ctx, cfg, manifest.Name, destDir)
		return fmt.Errorf("publishing third-party plugin %q command: %w", manifest.Name, err)
	}

	// Skipped for a plugin that owns no tables, same as the registry install
	// path. Creating an empty schema reaches for Postgres through Docker, so a
	// command-line plugin could not be installed on a machine with no stack
	// running. The registry path was fixed for this; this one was missed, and
	// only turned up by installing a third-party plugin end to end.
	if pluginOwnsTables(manifest) {
		if err := createPluginSchema(ctx, cfg, manifest.Name); err != nil {
			rollbackInstall(ctx, cfg, manifest.Name, destDir)
			return fmt.Errorf("creating schema for third-party plugin %q: %w", manifest.Name, err)
		}
	}

	fmt.Fprintf(os.Stderr, "Plugin %q (v%s) installed from third-party source %s.\n", manifest.Name, manifest.Version, u.Host)
	printBuildHint(ctx, manifest.Name)
	return nil
}
