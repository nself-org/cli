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

// TestCLITargetsOfNonCLIType proves a service plugin that lists cliCommands
// yields no target (D-0669): the v2 conversion drops pluginType, so only the
// commands block says the plugin publishes a binary, as the installer's v1 path
// reads pluginType service.
func TestCLITargetsOfNonCLIType(t *testing.T) {
	const svc = `{"name":"retro-gaming","version":"1.0.0","description":"d","category":"gaming","pluginType":"service","cliCommands":[{"name":"server","description":"d"}]}`
	m, err := ParseQuiet([]byte(svc))
	if err != nil {
		t.Fatal(err)
	}
	if got := CLITargetsOf(m); got != nil {
		t.Errorf("service plugin with cliCommands: got %v want none", got)
	}
	const cli = `{"name":"tenant","version":"1.0.0","description":"d","category":"infrastructure","pluginType":"cli","binaryName":"nself-tenant","cliCommands":[{"name":"tenant","description":"d"},{"name":"billing","description":"d"}]}`
	m, err = ParseQuiet([]byte(cli))
	if err != nil {
		t.Fatal(err)
	}
	if got := CLITargetsOf(m); len(got) != 2 {
		t.Errorf("cli plugin: got %v want 2 targets", got)
	}
}
