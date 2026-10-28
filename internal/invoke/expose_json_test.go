package invoke

// Purpose: a flag that overrides json to legacy or none must close the machine
// surface when the request sets it (D-0614).

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
)

func TestExposureRefusesFlagJSONOverride(t *testing.T) {
	str := func(s string) *string { return &s }
	cmd := *mustCmd(t, fixtureRegistry(t), "fx echo")
	cmd.Flags = append(append([]cmdregistry.Flag{}, cmd.Flags...),
		cmdregistry.Flag{Name: "plainout", Type: "bool", JSON: str(canon.JSONNone)},
		cmdregistry.Flag{Name: "oldjson", Type: "bool", JSON: str(canon.JSONLegacy)},
		cmdregistry.Flag{Name: "docjson", Type: "bool", JSON: str(canon.JSONEnvelope)})
	cases := []struct {
		set []string
		ok  bool
	}{
		{nil, true},
		{[]string{"docjson"}, true},
		{[]string{"plainout"}, false},
		{[]string{"oldjson"}, false},
		{[]string{"docjson", "plainout"}, false},
	}
	for _, c := range cases {
		set := map[string]bool{}
		for _, n := range c.set {
			set[n] = true
		}
		ok, reason := Exposure(&cmd, TransportMCP, set)
		if ok != c.ok || (!ok && !strings.Contains(reason, "envelope")) {
			t.Errorf("set=%v: ok=%v reason=%q, want ok=%v", c.set, ok, reason, c.ok)
		}
	}
}

func TestExposedFlagsExcludeNonEnvelopeJSON(t *testing.T) {
	str := func(s string) *string { return &s }
	cmd := &cmdregistry.Command{Flags: []cmdregistry.Flag{
		{Name: "status", Type: "bool", JSON: str(canon.JSONNone)},
		{Name: "legacy", Type: "bool", JSON: str(canon.JSONLegacy)},
		{Name: "tail", Type: "int"},
	}}
	for _, transport := range []string{TransportMCP, TransportHTTP, TransportHTTPStream} {
		flags := ExposedFlags(cmd, transport)
		if len(flags) != 1 || flags[0].Name != "tail" {
			t.Fatalf("%s flags = %v", transport, flags)
		}
	}
}
