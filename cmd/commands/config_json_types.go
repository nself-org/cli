package commands

// Purpose: the v1 JSON envelope data types of the read-only pilots (P7-REG-09)
// and the helpers they share: the output writer seam, the status/doctor state
// vocabulary with its exit codes, and the one emit helper for pilots that had a
// pre-contract bare JSON shape (status, doctor).
//
// Inputs: command results (config pairs, health report, doctor report).
//
// Outputs: values placed in the envelope `data`; registered in registry_types.go
// so tools/schemagen emits schemas/commands/<cmd>.v1.schema.json.
//
// Constraints:
//   - The status and doctor payload types (statusJSONOutput, doctorReport) are
//     embedded unchanged; the v1.5 data adds only `state`.
//   - config values in JSON follow the human output exactly: a secret key is
//     masked as "***" unless --reveal is given (maskValue); `config list`
//     never reveals, as the table does not.

import (
	"os"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/output"
)

// pilotWriter is the output seam of the pilots; tests substitute buffers.
var pilotWriter = output.Default

// configGetData is the data of `config get --json`.
type configGetData struct {
	Key    string `json:"key"`
	Value  string `json:"value"`  // masked unless --reveal
	Masked bool   `json:"masked"` // true when masking changed the value
	File   string `json:"file"`   // env file basename
}

// Source values of configListKey.source. (doctorCheckResult.status and the
// status service `status` are not closed: checks emit pass/warn/fail plus ok,
// skip, skipped and unknown, health emits more, so neither is enumerated.)
const (
	configSourceFile    = "file"
	configSourceDefault = "default"
	configSourceUnset   = "unset"
)

// PilotEnums holds the closed string sets of the pilot data types. tools/schemagen
// turns them into enum constraints, so the schemas and the code share one list.
type PilotEnums struct {
	StatusState      []string // statusData.state
	DoctorState      []string // doctorData.state
	ConfigListSource []string // configListKey.source
}

// PilotJSONEnums returns the closed string sets of the pilot data types.
func PilotJSONEnums() PilotEnums {
	return PilotEnums{
		StatusState:      []string{stateOK, stateTransitional, stateUnhealthy},
		DoctorState:      []string{stateOK, stateWarnings, stateUnhealthy},
		ConfigListSource: []string{configSourceFile, configSourceDefault, configSourceUnset},
	}
}

// configListKey is one row of `config list --json`.
type configListKey struct {
	Key    string `json:"key"`
	Value  string `json:"value"`  // file: masked like the table; default: the default; unset: ""
	Source string `json:"source"` // configSourceFile | configSourceDefault | configSourceUnset
}

// configListData is the data of `config list --json`.
type configListData struct {
	File string          `json:"file"` // env file basename
	Keys []configListKey `json:"keys"`
}

// State values of statusData.state and doctorData.state (EPIC D10).
const (
	stateOK           = "ok"
	stateTransitional = "transitional"
	stateUnhealthy    = "unhealthy"
	stateWarnings     = "warnings"
)

// stateExitCode is the v1.5 exit status of a state (contract:cli.exit-codes v1,
// reserved range 10-12): 10 unhealthy or failed, 11 transitional, 12 warnings
// only, 0 for ok.
func stateExitCode(state string) int {
	switch state {
	case stateUnhealthy:
		return 10
	case stateTransitional:
		return 11
	case stateWarnings:
		return 12
	}
	return 0
}

// emitStateJSON writes the result of a command that printed a pre-contract bare
// JSON shape. bare is that shape; enveloped is the same payload plus `state`.
// v1.4 mode and NSELF_JSON_LEGACY=1 in v1.5 mode print bare (through
// output.EmitLegacyCompatible, which owns the deprecation warning); v1.5 mode
// prints the envelope around enveloped.
func emitStateJSON(command string, bare, enveloped any) error {
	w := pilotWriter()
	// compat.V15(P7-REG-09): bare pre-contract JSON -> v1 envelope whose data adds `state`
	if compat.V15() && !legacyJSONRequested() {
		return output.EmitData(w, command, enveloped)
	}
	return output.EmitLegacyCompatible(w, command, bare)
}

// legacyJSONRequested mirrors output.EmitLegacyCompatible's reading of
// NSELF_JSON_LEGACY, so the legacy output omits the v1.5-only `state`.
func legacyJSONRequested() bool {
	v := os.Getenv(output.LegacyEnvVar)
	return v == "1" || strings.EqualFold(v, "true")
}
