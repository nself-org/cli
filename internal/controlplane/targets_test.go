package controlplane

import (
	"strings"
	"testing"
)

func TestResolveTargets(t *testing.T) {
	inv := &Inventory{Environments: map[string]Environment{
		"local": {Name: "local", Kind: "local", Servers: []Server{{Name: "z", Role: RoleApp}}},
		"qa":    {Name: "qa", Kind: "remote", Servers: []Server{{Name: "b", Role: RoleLB}, {Name: "a", Role: RoleApp}, {Name: "o", Role: RoleObservability}}},
	}}
	got, err := ResolveTargets(inv, Selector{Tier: TierLocalServers})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Server.Name != "o" || got[1].Server.Name != "a" || got[2].Server.Name != "b" {
		t.Fatalf("order: %+v", got)
	}
	got, err = ResolveTargets(inv, Selector{Env: "qa", Server: "a"})
	if err != nil || len(got) != 1 || got[0].Server.Name != "a" {
		t.Fatalf("env+server selector: %+v %v", got, err)
	}
	got, err = ResolveTargets(inv, Selector{Env: "local"})
	if err != nil || len(got) != 1 || got[0].Tier != TierLocal {
		t.Fatalf("env selector: %+v %v", got, err)
	}
	_, err = ResolveTargets(inv, Selector{Server: "absent"})
	if err == nil || !strings.Contains(err.Error(), "E486") {
		t.Fatalf("empty selection: %v", err)
	}
}
