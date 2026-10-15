package manifestv2

import (
	"reflect"
	"testing"
)

func TestCLITargets(t *testing.T) {
	cases := []struct {
		name, typ, bin string
		cmds           []string
		want           []CLITarget
	}{
		{"multi", "", "nself-tenant", []string{"tenant", "billing"}, []CLITarget{{"nself-tenant", "tenant"}, {"nself-billing", "billing"}}},
		{"cliCommands win over binaryName", "cli", "nself-x", []string{"a"}, []CLITarget{{"nself-a", "a"}}},
		{"binaryName only", "", "nself-webhooks", nil, []CLITarget{{"nself-webhooks", "webhooks"}}},
		{"cli type, no binary", "cli", "", nil, []CLITarget{{"nself-p", "p"}}},
		{"empty names ignored", "", "", []string{""}, nil},
		{"not cli", "service", "nself-x", []string{"a"}, nil},
		{"nothing", "", "", nil, nil},
	}
	for _, c := range cases {
		if got := CLITargets("p", c.typ, c.bin, c.cmds); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestCLITargetsOfV1PluginType(t *testing.T) {
	for _, typ := range []string{"service", "cli", ""} {
		t.Run(typ, func(t *testing.T) {
			m, err := Normalize([]byte(`{"name":"retro-gaming","version":"1.0.0",
				"pluginType":"` + typ + `","binaryName":"nself-server",
				"cliCommands":[{"name":"server"}]}`))
			if err != nil {
				t.Fatal(err)
			}
			want := CLITargets(m.Name, typ, "nself-server", []string{"server"})
			if got := CLITargetsOf(m); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v, want installer targets %v", got, want)
			}
		})
	}
}
