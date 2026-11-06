package controlplane

import "testing"

func TestEnvName(t *testing.T) {
	inv := &Inventory{Environments: map[string]Environment{"Prod": {}, "stage": {}, "STAGE": {}}}
	if got, ok := EnvName(inv, "prod"); !ok || got != "Prod" {
		t.Errorf("case-insensitive match: %q %v", got, ok)
	}
	for _, env := range []string{"stage", "dev"} { // ambiguous, missing
		if got, ok := EnvName(inv, env); ok {
			t.Errorf("%s: got %q, want no match", env, got)
		}
	}
	if _, ok := EnvName(nil, "prod"); ok {
		t.Error("nil inventory matched")
	}
}

func TestPrimaryServer(t *testing.T) {
	inv := &Inventory{Environments: map[string]Environment{
		"p":     {Servers: []Server{{Name: "db"}, {Name: "app", Role: RoleApp}, {Name: "main", Primary: true}}},
		"a":     {Servers: []Server{{Name: "db"}, {Name: "app", Role: RoleApp}}},
		"f":     {Servers: []Server{{Name: "only"}}},
		"empty": {},
	}}
	for env, want := range map[string]string{"p": "main", "a": "app", "f": "only"} {
		if s, ok := PrimaryServer(inv, env); !ok || s.Name != want {
			t.Errorf("%s: got %q %v, want %s", env, s.Name, ok, want)
		}
	}
	for _, env := range []string{"empty", "missing"} {
		if _, ok := PrimaryServer(inv, env); ok {
			t.Errorf("%s: want no server", env)
		}
	}
}
