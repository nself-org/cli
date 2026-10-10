package bundle

// installer_licence.go — the licence pre-flight of `nself bundle install`.
//
// Purpose: refuse before any filesystem change when the operator is not
// entitled. Two phases, always run for a paid bundle (--force never bypasses):
//
//	Phase 1 (bundle-level): ping_api /bundle/entitled grants this bundle as a unit.
//	Phase 2 (plugin-level fallback): defaultLicenseChecker catches the no-key case.
//
// A free bundle (Bundle.IsFree) skips Phase 1 only: IsFree trusts the Price
// string, so Phase 2 (a local check that fails only when a member needs a licence
// and no key exists) still runs for it.
// Inputs: the bundle, the planned plugins and InstallOpts (test hooks).
// Outputs: nil, or the "license validation failed" error.
// Constraints: moved from installer.go to keep that file under 300 lines.

import (
	"context"
	"fmt"

	"github.com/nself-org/cli/internal/license"
)

func checkLicence(ctx context.Context, b Bundle, planned []string, opts InstallOpts) error {
	// Phase 1: bundle-level entitlement via ping_api (or test hook); not for a free bundle.
	if !b.IsFree() {
		key := license.CollectLicenseKey()
		entitledFn := opts.bundleEntitledChecker
		if entitledFn == nil {
			entitledFn = license.BundleEntitled
		}
		entitled, entitleErr := entitledFn(ctx, key, b.Slug)
		if entitleErr != nil || !entitled {
			msg := fmt.Sprintf("bundle %q: not entitled", b.Slug)
			if entitleErr != nil {
				msg = entitleErr.Error()
			}
			return fmt.Errorf("license validation failed: %s\n\nBuy at: https://nself.org/pricing or run 'nself license set <key>'", msg)
		}
	}

	// Phase 2: plugin-level pre-flight (catches no-key-at-all cases for free).
	check := opts.licenseChecker
	if check == nil {
		check = defaultLicenseChecker
	}
	if err := check(ctx, planned); err != nil {
		return fmt.Errorf("license validation failed for bundle %q: %w", b.Slug, err)
	}
	return nil
}
