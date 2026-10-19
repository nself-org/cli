package plugin

// tier_pair.go: whether a registry slug is served as a free/licensed pair.
//
// Purpose: lets a caller that keeps an installed copy (the bundle install loop)
// tell the operator a Licensed build exists, without resolving the tier itself.
// Inputs: a registry and a plugin name. Outputs: true for a declared tier pair.
// Constraints: read-only; same pair test as ResolvePluginTier (splitTierPair).

// HasLicensedPair reports whether name is served twice as a declared tier pair
// (one free and one Licensed entry).
func HasLicensedPair(reg *Registry, name string) bool {
	if reg == nil {
		return false
	}
	_, _, ok := splitTierPair(findPluginEntries(reg, name))
	return ok
}
