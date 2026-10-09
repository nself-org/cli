package plugin

// Purpose: keep registry wire tiers separate from the licence labels shown by the CLI.
// Inputs: registry tier strings. Outputs: display labels and public licence values.
// Constraints: registry and licence API wire values remain unchanged.
const (
	WireTierFree    = "free" // wire: registry tier and counts key
	WireTierPro     = "pro"  // wire: registry tier and licence API product
	LicenseFree     = "free"
	LicenseLicensed = "licensed"
)

// Label returns the human licence label for a registry tier.
func Label(tier string) string {
	switch tier {
	case WireTierFree:
		return "Free"
	case WireTierPro, LicenseLicensed:
		return "Licensed"
	default:
		return tier
	}
}

// LicenseValue returns the public licence value for a registry tier.
func LicenseValue(tier string) string {
	if tier == WireTierPro {
		return LicenseLicensed
	}
	return tier
}

// BundleLabel names the bundle that includes a licensed plugin.
func BundleLabel(bundle string) string { return "Licensed (in the " + bundle + " Bundle)" }
