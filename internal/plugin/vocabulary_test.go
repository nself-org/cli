package plugin

import (
	"context"
	"testing"
)

func TestVocabularyLabels(t *testing.T) {
	if Label(WireTierFree) != "Free" || Label(WireTierPro) != "Licensed" {
		t.Fatal("registry tiers must have Free and Licensed labels")
	}
	if LicenseValue(WireTierPro) != LicenseLicensed || BundleLabel("Chat") != "Licensed (in the Chat Bundle)" {
		t.Fatal("licensed vocabulary mismatch")
	}
}

func TestLicensedTierOverride(t *testing.T) {
	reg := &Registry{Plugins: []PluginManifest{{Name: "example", Tier: WireTierPro}}}
	got, err := ResolvePlugin(context.Background(), reg, "example", LicenseLicensed, nil)
	if err != nil || got.Tier != WireTierPro {
		t.Fatalf("licensed flag must select the registry wire tier: %v, %v", got, err)
	}
}
