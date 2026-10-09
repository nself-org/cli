package controlplane

import (
	"encoding/json"
	"github.com/google/jsonschema-go/jsonschema"
	"os"
	"strings"
	"testing"
)

func TestProdClassTier(t *testing.T) {
	inv := &Inventory{Environments: map[string]Environment{
		"live": {Name: "live", Kind: "remote", Tier: TierProd},
		"qa":   {Name: "qa", Kind: "remote"},
	}}
	for _, tc := range []struct {
		name string
		want bool
	}{{"live", true}, {"prod", true}, {"production", true}, {"qa", false}} {
		if got := IsProdClass(inv, tc.name); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTierRank(t *testing.T) {
	for _, tc := range []struct {
		tier Tier
		rank int
	}{{TierLocal, 0}, {TierLocalServers, 1}, {TierProd, 2}, {Tier("invalid"), 3}} {
		if got := TierRank(tc.tier); got != tc.rank {
			t.Errorf("rank(%q)=%d, want %d", tc.tier, got, tc.rank)
		}
	}
}

func TestInventoryRejectsContradictoryTiers(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		tier       Tier
	}{
		{"prod", "remote", TierLocalServers},
		{"production", "remote", TierLocal},
		{"qa", "remote", TierLocal},
		{"qa", "local", TierProd},
		{"qa", "local", TierLocalServers},
	} {
		t.Run(tc.name+"/"+tc.kind+"/"+string(tc.tier), func(t *testing.T) {
			inv := &Inventory{SchemaVersion: 2, Environments: map[string]Environment{
				"local": {Name: "local", Kind: "local", Tier: TierLocal},
				tc.name: {Name: tc.name, Kind: tc.kind, Tier: tc.tier},
			}}
			if err := ValidateInventory(inv); err == nil || !strings.Contains(err.Error(), ".tier") {
				t.Fatalf("contradictory tier accepted or wrong refusal: %v", err)
			}
		})
	}
}

func TestInventoryMigrateV1(t *testing.T) {
	inv := &Inventory{SchemaVersion: 1, Environments: map[string]Environment{
		"local":      {Name: "local", Kind: "local"},
		"qa":         {Name: "qa", Kind: "remote"},
		"production": {Name: "production", Kind: "remote"},
	}}
	if err := Migrate(inv); err != nil {
		t.Fatal(err)
	}
	if inv.SchemaVersion != 2 || inv.Environments["local"].Tier != TierLocal || inv.Environments["qa"].Tier != TierLocalServers || inv.Environments["production"].Tier != TierProd {
		t.Fatalf("bad migration: %+v", inv)
	}
	schemaBytes, err := os.ReadFile("../../schemas/control-plane.v2.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(&jsonschema.ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(b, &value); err != nil {
		t.Fatal(err)
	}
	if err := resolved.Validate(value); err != nil {
		t.Fatalf("migrated inventory violates schema: %v", err)
	}
}
