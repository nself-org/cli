package plugin

import "testing"

func TestHasLicensedPair(t *testing.T) {
	reg := &Registry{Plugins: []PluginManifest{
		{Name: "cron", Tier: "free", TierPair: true},
		{Name: "cron", Tier: "pro", TierPair: true},
		{Name: "solo", Tier: "free"},
		{Name: "dup", Tier: "free"},
		{Name: "dup", Tier: "pro"}, // two entries but not a declared pair
	}}
	for _, tc := range []struct {
		reg  *Registry
		name string
		want bool
	}{
		{reg, "cron", true},
		{reg, "CRON", true},
		{reg, "solo", false},
		{reg, "dup", false},
		{reg, "missing", false},
		{nil, "cron", false},
	} {
		if got := HasLicensedPair(tc.reg, tc.name); got != tc.want {
			t.Errorf("HasLicensedPair(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
