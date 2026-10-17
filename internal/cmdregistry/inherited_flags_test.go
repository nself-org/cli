package cmdregistry

import (
	"testing"
	"time"
)

// A parent cycle (a command whose parent chain loops) must end the walk, not
// hang it: fuzzed and hand-edited registries can carry one (D-0835).
func TestWithInheritedFlagsParentCycle(t *testing.T) {
	reg := &Registry{Commands: []Command{
		{Path: "x", Parent: "x", Flags: []Flag{{Name: "env", Persistent: true}}},
		{Path: "y", Parent: "x"},
	}}
	reg.reindex()
	done := make(chan *Command, 1)
	go func() { c, _ := reg.Lookup("y"); done <- reg.WithInheritedFlags(c) }()
	select {
	case got := <-done:
		if len(got.Flags) != 1 || got.Flags[0].Name != "env" {
			t.Fatalf("want the inherited env flag once, got %+v", got.Flags)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WithInheritedFlags did not return on a parent cycle")
	}
}
