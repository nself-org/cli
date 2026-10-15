package plugin

// Purpose: license validation, EOL blocking, and author-CRL revocation checks — the license-gate half of the plugin security pipeline defined in security.go.
// Inputs: a plugin name/author and a context with timeout for remote validation.
// Outputs: an error when the plugin fails a license, EOL, or revocation check; nil on pass or non-fatal network conditions.
// Constraints: split out of security.go as a pure move (CLI-R12); no behavior change. Never reorder relative to the checksum/signature checks in security.go within manager.go's frozen load order (registry fetch -> license check -> checksum verify -> systemDependencies -> schema -> dep resolution -> runtime env).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
)

// checkLicense validates that a license key exists and is acceptable.
// It checks all configured keys (multi-key support) against the local
// entitlement cache first, then falls through to remote validation.
func checkLicense(ctx context.Context, name string) error {
	keys := license.CollectLicenseKeys()

	if len(keys) == 0 {
		key := os.Getenv("NSELF_PLUGIN_LICENSE_KEY")
		if key == "" {
			keyPath := licenseKeyPath()
			data, err := os.ReadFile(keyPath)
			if err != nil {
				return fmt.Errorf("plugin %q requires a license key. Run 'nself license add <key>' or visit %s",
					name, "nself.org/pricing")
			}
			key = strings.TrimSpace(string(data))
		}
		if key != "" {
			keys = []string{key}
		}
	}

	if len(keys) == 0 {
		return fmt.Errorf("plugin %q requires a license key. Run 'nself license add <key>' or visit %s",
			name, "nself.org/pricing")
	}

	cacheDir := installCacheDir(ctx, licenseCacheDir(), "license")

	// A fresh cached entitlement list that omits this plugin, for a tier that is
	// not all-access, is a denial. A cached-valid key must not override it (the
	// server refuses the download), so such a denial is confirmed live below
	// instead of trusting the key cache or the offline fallback (D-0474).
	cachedDenial := false
	if allowed, found := checkEntitlements(cacheDir, name); found {
		if allowed {
			return nil
		}
		cachedDenial = !license.IsAllAccessTier(cachedEntitlementTier(cacheDir))
	}

	var lastErr error
	for _, key := range keys {
		if err := validateLicenseFormat(key); err != nil {
			continue
		}

		if valid, found := checkLicenseCache(key, cacheDir); found {
			if !valid {
				continue
			}
			if !cachedDenial {
				return nil
			}
		}

		valid, lvr, err := validateLicenseRemoteWithEntitlements(ctx, key, pingAPIURL())
		if err != nil {
			if strings.Contains(err.Error(), "expired") {
				lastErr = fmt.Errorf("plugin %q: %w", name, errs.ErrLicenseExpired)
				continue
			}
			if offlineValid, offlineFound := checkLicenseCacheOffline(key, cacheDir); offlineFound && offlineValid && !cachedDenial {
				return nil
			}
			lastErr = fmt.Errorf("plugin %q: %w", name, errs.ErrLicenseNetworkUnavailable)
			continue
		}
		_ = CacheLicense(key, valid, cacheDir)

		if lvr != nil && len(lvr.Plugins) > 0 {
			_ = cacheEntitlements(cacheDir, lvr.Tier, lvr.Plugins)
			for _, p := range lvr.Plugins {
				if p == name {
					return nil
				}
			}
		}

		// An all-access tier entitles every plugin, whether or not the server
		// bothered to enumerate them.
		//
		// Without this, a valid ɳSelf+ licence was refused outright. The server
		// returns tier "plus" with an EMPTY plugins list — all-access has
		// nothing to enumerate — and the only grant above is membership of that
		// list, so the empty list fell through to ErrLicenseTierTooLow:
		//
		//   error installing "ai": plugin "ai": license tier does not include
		//   this plugin
		//
		// which is precisely backwards for the tier that includes everything.
		// That is step 9 of the golden path, and it had never run before today.
		//
		// This does not widen anything for a bundle tier: "chat" is not
		// all-access, so a ɳChat licence still only grants what its plugins
		// list names.
		if lvr != nil && license.IsAllAccessTier(lvr.Tier) {
			_ = cacheEntitlements(cacheDir, lvr.Tier, lvr.Plugins)
			return nil
		}

		if valid {
			lastErr = fmt.Errorf("plugin %q: %w", name, errs.ErrLicenseTierTooLow)
			continue
		}
		lastErr = fmt.Errorf("plugin %q: license key is not valid", name)
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("plugin %q requires a license key. Get one at %s", name, "nself.org/pricing")
}

// CheckEOLBlock fetches the registry entry for name and returns an error if
// the plugin's status is "eol" and allowEOL is false. (S58-T03)
// If the registry cannot be reached or the plugin is not found, this function
// returns nil (install will fail downstream with a more specific error).
func CheckEOLBlock(ctx context.Context, name string, allowEOL bool) error {
	cacheDir := installCacheDir(ctx, defaultCacheDir(), "registry")
	reg, err := FetchRegistry(ctx, "", cacheDir)
	if err != nil {
		return nil
	}
	manifest, found := findPlugin(reg, name)
	if !found {
		return nil
	}
	if manifest.PublishStatus == "eol" && !allowEOL {
		return fmt.Errorf(
			"plugin %q has reached end-of-life and cannot be installed\n"+
				"use --allow-eol to override (not recommended)\n"+
				"run 'nself plugin info %s' for details",
			name, name,
		)
	}
	return nil
}

// checkAuthorRevocation fetches the author CRL from plugins.nself.org and
// returns an error if the plugin author appears in the revocation list. (S58-T09)
// Short-circuits on any network / parse error — install proceeds and the
// full install step will catch other issues.
func checkAuthorRevocation(ctx context.Context, author string) error {
	if author == "" {
		return nil
	}

	url := "https://plugins.nself.org/.well-known/revoked-authors.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "nself-cli")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fetch author revocation list (offline?): %v\n", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	type revokedEntry struct {
		AuthorKey string `json:"authorKey"`
		RevokedAt string `json:"revokedAt"`
		Reason    string `json:"reason,omitempty"`
	}
	type crlResponse struct {
		RevokedAuthors []revokedEntry `json:"revokedAuthors"`
	}

	var crl crlResponse
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil
	}
	if err := json.Unmarshal(body, &crl); err != nil {
		return nil
	}

	for _, entry := range crl.RevokedAuthors {
		if strings.EqualFold(entry.AuthorKey, author) {
			msg := fmt.Sprintf(
				"plugin author %q has been revoked and cannot be installed (revoked: %s).\n"+
					"Remove any plugins from this author and contact support@nself.org.\n"+
					"See https://nself.org/security/revocations for details.",
				author, entry.RevokedAt,
			)
			if entry.Reason != "" {
				msg += "\nReason: " + entry.Reason
			}
			return fmt.Errorf("%s", msg)
		}
	}
	return nil
}
