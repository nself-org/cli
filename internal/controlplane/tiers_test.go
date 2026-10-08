package controlplane

import (
	"encoding/json"
	"github.com/google/jsonschema-go/jsonschema"
	"os"
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
