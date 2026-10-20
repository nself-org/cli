package commands

import (
	"reflect"
	"testing"
)

func TestBreakoutPluginScaffold(t *testing.T) {
	for _, name := range []string{"init", "scaffold"} {
		t.Run(name, func(t *testing.T) {
			row, args, ok := breakoutRow("plugin", []string{name, "example", "--json"})
			if !ok || row.Plugin != "plugin-dev" || !reflect.DeepEqual(args, []string{"init", "example", "--json"}) {
				t.Fatalf("plugin %s: row=%+v args=%v ok=%v", name, row, args, ok)
			}
		})
	}
}

func TestRewritePluginScaffold(t *testing.T) {
	for _, name := range []string{"init", "scaffold"} {
		got, _, err := rewriteCanonArgs([]string{"plugin", name, "example"}, true)
		if err != nil || !reflect.DeepEqual(got, []string{"plugin-dev", "init", "example"}) {
			t.Errorf("plugin %s: args=%v err=%v", name, got, err)
		}
	}
}
