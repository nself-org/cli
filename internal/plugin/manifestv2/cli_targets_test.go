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
