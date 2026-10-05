package commands

// JSON data-type registration for the command registry.
//
// Purpose: the one fact the declarative canon.yaml cannot hold is the Go type
// behind a command's v1 JSON envelope. A command is `json: envelope` exactly
// when its path is a key of jsonDataTypes; envelope support is never declared
// in YAML (EPIC D1).
//
// Rules (EPIC D1, D8):
//   - Keys are command paths without the leading "nself ", e.g. "config get".
//   - Values are zero values of the Go type placed in the envelope `data`.
//   - A path registered here must not be declared `json: legacy` in canon.yaml:
//     a command is envelope or legacy, never both.
//   - A path also listed in jsonV15OnlyEnvelope reports `envelope` only in v1.5
//     mode; in v1.4 mode it keeps its pre-contract behaviour (D8: `config get`,
//     `config list`, `config show` accepted and ignored --json before P7-REG).
//
// Both maps are empty in P7-REG-05; P7-REG-07 and P7-REG-09 add entries.

// jsonDataTypes maps a command path to the Go type of its envelope data.
var jsonDataTypes = map[string]any{}

// jsonV15OnlyEnvelope lists the paths in jsonDataTypes whose envelope is
// visible only when compat.V15() is true.
var jsonV15OnlyEnvelope = map[string]bool{}
