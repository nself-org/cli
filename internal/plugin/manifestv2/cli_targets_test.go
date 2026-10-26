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

// TestCLITargetsOf proves the targets match the installer's v1 rule for the
// plugin.json as written (D-0669): a service plugin that lists cliCommands has
// none, a plugin with cliCommands and no pluginType has one per command, and
// the v2 form of each gives the same.
func TestCLITargetsOf(t *testing.T) {
	const meta = `"version":"1.0.0","description":"d","category":"infrastructure"`
	cases := []struct {
		name, doc string
		want      int
	}{
		{"service with cliCommands", `{"name":"retro-gaming",` + meta + `,"pluginType":"service","cliCommands":[{"name":"server","description":"d"}]}`, 0},
		{"cliCommands, no pluginType", `{"name":"tenant",` + meta + `,"cliCommands":[{"name":"tenant","description":"d"},{"name":"billing","description":"d"}]}`, 2},
		{"cli type", `{"name":"tenant",` + meta + `,"pluginType":"cli","binaryName":"nself-tenant","cliCommands":[{"name":"tenant","description":"d"},{"name":"billing","description":"d"}]}`, 2},
	}
	for _, c := range cases {
		got, err := CLITargetsOf([]byte(c.doc))
		if err != nil || len(got) != c.want {
			t.Errorf("%s: got %v, %v want %d targets", c.name, got, err, c.want)
		}
		if c.want == 0 {
			continue // a v2 file has no pluginType to say the plugin is not cli
		}
		if m, err := ParseQuiet([]byte(c.doc)); err != nil {
			t.Errorf("%s: parse: %v", c.name, err)
		} else if b, err := Marshal(m); err != nil {
			t.Errorf("%s: marshal: %v", c.name, err)
		} else if got, err := CLITargetsOf(b); err != nil || len(got) != c.want {
			t.Errorf("%s: v2 form: got %v, %v want %d targets", c.name, got, err, c.want)
		}
	}
}
