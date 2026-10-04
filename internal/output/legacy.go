package output

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/compat"
)

// LegacyEnvVar restores the pre-contract bare JSON in v1.5 mode for commands
// that printed one (EmitLegacyCompatible only). Removal at v1.6.0.
const LegacyEnvVar = "NSELF_JSON_LEGACY"

// legacyWarning is printed once per process on the first bare write forced by
// LegacyEnvVar in v1.5 mode.
const legacyWarning = "warning: NSELF_JSON_LEGACY is deprecated and will be removed in v1.6.0; parse the v1 envelope instead (see JSON-Output wiki page)"

// EmitLegacyCompatible writes the result of a command that printed a
// pre-contract JSON shape (status, doctor, config show --format json).
//
// v1.4 mode: data bare, byte-identical to ui.PrintJSON, nothing on w.Err.
// v1.5 mode: the v1 envelope (same as EmitData). v1.5 mode with
// NSELF_JSON_LEGACY=1 (or "true"): data bare as above, plus the deprecation
// warning once per process on w.Err. In v1.4 mode NSELF_JSON_LEGACY has no
// effect. EmitData and EmitError never consult it.
func EmitLegacyCompatible(w Writer, command string, data any) error {
	// compat.V15(P7-REG-02): bare pre-contract JSON -> v1 envelope (NSELF_JSON_LEGACY=1 or true keeps bare JSON for one minor)
	if !compat.V15() {
		return writeBare(w, data)
	}
	if !legacyRequested() {
		return EmitData(w, command, data)
	}
	if err := writeBare(w, data); err != nil {
		return err
	}
	if w.Err != nil && claimLegacyWarning() {
		_, _ = fmt.Fprintln(w.Err, legacyWarning) // diagnostics stream: a write failure has nowhere to go
	}
	return nil
}

// legacyRequested reports whether NSELF_JSON_LEGACY selects bare output.
func legacyRequested() bool {
	v := os.Getenv(LegacyEnvVar)
	return v == "1" || strings.EqualFold(v, "true")
}
