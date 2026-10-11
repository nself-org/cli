package commands

// Purpose: P7-PLUG-20 fix round 2: `plugin debug` and `plugin info` refuse a
// path-shaped name before any port, dlv, build or file access. Pre-fix, a name
// like ../../victim controlled `go build -o` and overwrote that file.
// Constraints: no dlv, no go build, no network.

import (
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestPluginDebugAndInfoRefusePathNamesCmd(t *testing.T) {
	for _, name := range []string{"../../../home/user/victim", "./x", "a/b", ".."} {
		for _, portOnly := range []bool{false, true} {
			if portOnly {
				setFlags(t, pluginDebugCmd, "port-only")
			}
			err := runPluginDebug(pluginDebugCmd, []string{name})
			if d := errs.Describe(err); d == nil || d.Code != "E401" {
				t.Errorf("plugin debug %q (port-only=%v): err = %v, want E401", name, portOnly, err)
			}
			resetFlag(pluginDebugCmd, "port-only")
		}
		if err := runPluginInfo(pluginInfoCmd, []string{name}); errs.Describe(err) == nil {
			t.Errorf("plugin info %q: err = %v, want E401", name, err)
		}
		if _, err := resolvePluginPath(name); err == nil {
			t.Errorf("resolvePluginPath(%q) = nil error", name)
		}
	}
}
