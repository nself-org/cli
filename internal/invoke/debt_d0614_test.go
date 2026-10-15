package invoke

import (
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
)

func TestD0614ExposureJSONOverrides(t *testing.T) {
	for _, kind := range []string{canon.JSONNone, canon.JSONLegacy} {
		cmd := &cmdregistry.Command{Canon: canon.CanonPlugin, Runnable: true, JSON: canon.JSONEnvelope, Output: canon.OutputDocument, SideEffect: canon.SideEffectRead,
			Flags: []cmdregistry.Flag{{Name: "raw", JSON: &kind}}}
		if ok, why := Exposure(cmd, TransportMCP, map[string]bool{"raw": true}); ok {
			t.Fatalf("override %s exposed: %s", kind, why)
		}
		if ok, why := Exposure(cmd, TransportMCP, nil); !ok {
			t.Fatalf("unset override refused: %s", why)
		}
	}
}
