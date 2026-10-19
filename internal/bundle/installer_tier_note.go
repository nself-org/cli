package bundle

// installer_tier_note.go — say so when a bundle install keeps a free copy.
//
// Purpose: a bundle install leaves a member that is already installed at the
// bundle's version (or higher) untouched. When that copy is the free build of
// a free/licensed tier pair, the run reports success while the operator who
// bought the Bundle still runs the free build (D-0320). Print the way to switch.
// Inputs: the plugin directory, the member name, the registry (via a hook).
// Outputs: one warning line on the install stream; nothing else changes.
// Constraints: advisory only. An upgrade needs no note: the member loop moves the
// old copy aside first, so the install re-resolves the tier by entitlement.

import (
	"context"
	"fmt"

	"github.com/nself-org/cli/internal/plugin"
)

// registryHasLicensedPair is the production licensedPair hook.
func registryHasLicensedPair(ctx context.Context, name string) bool {
	reg, err := plugin.FetchRegistry(ctx, "", defaultRegistryCacheDir())
	return err == nil && plugin.HasLicensedPair(reg, name)
}

// noteKeptFree prints the switch command when the kept copy of name is free and
// the registry also serves a Licensed build. A free bundle gets no note.
func (m *memberRun) noteKeptFree(ctx context.Context, name string) {
	if m.b.IsFree() {
		return
	}
	if tier, _, ok := plugin.InstalledTier(m.pluginDir, name); !ok || tier != plugin.LicenseFree {
		return
	}
	pair := m.opts.licensedPair
	if pair == nil {
		pair = registryHasLicensedPair
	}
	if pair(ctx, name) {
		_, _ = fmt.Fprintf(m.out, "  ⚠ %s: kept the installed free copy; run 'nself plugin update %s --tier licensed' to switch to the Licensed build\n", name, name)
	}
}
