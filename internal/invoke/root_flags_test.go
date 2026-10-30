package invoke

import (
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
)

// A plugin root command that declares flags builds argv from flags, refuses
// argv, and a root that declares none still takes free-form argv.
func TestPluginRootDeclaredFlags(t *testing.T) {
	cmd := &cmdregistry.Command{Path: "nself ciroot", Canon: canon.CanonPlugin,
		Flags: []cmdregistry.Flag{{Name: "detach", Type: "bool"}}}
	argv, err := BuildArgv(cmd, Request{Flags: map[string]any{"detach": true}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"ciroot", "--json", "--detach=true"}; !reflect.DeepEqual(argv, want) {
		t.Errorf("argv %v, want %v", argv, want)
	}
	if _, err := BuildArgv(cmd, Request{Argv: []string{"--detach"}}); err == nil {
		t.Error("argv must be refused on a root that declares flags")
	}
	if _, err := BuildArgv(cmd, Request{Flags: map[string]any{"nope": true}}); err == nil {
		t.Error("an undeclared root flag must be refused")
	}
	props := ParamsSchema(cmd)["properties"].(map[string]any)
	if props["argv"] != nil || props["flags"] == nil {
		t.Errorf("schema %v", props)
	}
	bare := &cmdregistry.Command{Path: "nself plainroot", Canon: canon.CanonPlugin}
	if _, err := BuildArgv(bare, Request{Flags: map[string]any{"x": true}}); err == nil {
		t.Error("a flagless root takes argv, not flags")
	}
	if argv, err := BuildArgv(bare, Request{Argv: []string{"x"}}); err != nil || len(argv) != 3 {
		t.Errorf("free-form argv: %v %v", argv, err)
	}
}
