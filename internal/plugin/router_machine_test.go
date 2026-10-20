package plugin

import (
	"errors"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestMachinePluginExecutionRefused(t *testing.T) {
	for _, by := range []string{"mcp", "automation"} {
		t.Run(by, func(t *testing.T) {
			t.Setenv("NSELF_INVOKED_BY", by)
			// An invalid executable ensures the machine boundary is checked before exec.
			err := execPluginBinary("missing-plugin-binary", "demo", []string{"--token=fixture"})
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != "E421" {
				t.Fatalf("expected E421 before plugin execution, got %v", err)
			}
		})
	}
}
