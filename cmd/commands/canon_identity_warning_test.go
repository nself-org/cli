package commands

// D-1116: an identity break-out row (ci -> ci, functions -> functions) only
// records that the verb is now served by a plugin. The path does not change, so
// no "moved to itself" warning may print and no deprecation may be recorded.

import (
	"os"
	"strings"
	"testing"
)

func TestCanonIdentityBreakoutNoWarning(t *testing.T) {
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	moved := canonNote{Kind: "breakout", Old: "runner", New: "ci nodes", RemovalAt: "v1.6.0", Plugin: "ci", From: []string{"runner"}, To: []string{"ci", "nodes"}}
	cases := []struct {
		name, legacy string
		notes        []canonNote
		wantWarn     bool
	}{
		{"ci to ci", "", []canonNote{{Kind: "breakout", Old: "ci", New: "ci", RemovalAt: "v1.6.0", Plugin: "ci", From: []string{"ci"}, To: []string{"ci"}}}, false},
		{"functions to functions", "", []canonNote{{Kind: "breakout", Old: "functions", New: "functions", RemovalAt: "v1.6.0", Plugin: "functions", From: []string{"functions"}, To: []string{"functions"}}}, false},
		{"real move still warns", "", []canonNote{moved}, true},
	}
	for _, c := range cases {
		resetCanonWarning()
		os.Args = []string{"nself", c.notes[0].Old, "ls"}
		got := captureStderr(t, func() { warnLegacyChain(c.legacy, c.notes) })
		if warned := strings.Contains(got, "moved to"); warned != c.wantWarn {
			t.Errorf("%s: warning %q, want warning=%v", c.name, got, c.wantWarn)
		}
	}
}
