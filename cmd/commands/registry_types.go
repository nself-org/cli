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
// P7-REG-07 registers `help` (the first envelope command); P7-REG-09 adds the
// pilots: status and doctor (pre-contract bare JSON in v1.4 mode, envelope in
// v1.5), and config show/get/list (v1.5 only, their --json was accepted and
// ignored before).
//
// P7-CANON-02 splits the registrations by domain: this file keeps the map and
// `help`; registry_types_<domain>.go files register the rest from init() with
// registerJSONType (observe: status, doctor; config: show, get, list). A later
// Ticket adds its own registry_types_<domain>.go and never edits this map.

package commands

import "github.com/nself-org/cli/internal/cmdregistry"

// jsonDataTypes maps a command path to the Go type of its envelope data.
var jsonDataTypes = map[string]any{
	// `help --json` answers with the registry document itself.
	"help": cmdregistry.Registry{},
}

// registerJSONType adds one envelope data type. It is called from init() in the
// registry_types_<domain>.go files; registering a path twice is a programming
// error and panics at start-up of the test binary (TestRegistryTypesSplit).
func registerJSONType(path string, zero any) {
	if _, dup := jsonDataTypes[path]; dup {
		panic("commands: JSON data type registered twice for " + path)
	}
	jsonDataTypes[path] = zero
}

// jsonV15OnlyEnvelope lists the paths in jsonDataTypes whose envelope is
// visible only when compat.V15() is true. Each domain file adds its own paths
// from init() with registerV15OnlyEnvelope.
var jsonV15OnlyEnvelope = map[string]bool{}

// registerV15OnlyEnvelope marks a registered path as envelope in v1.5 mode only.
func registerV15OnlyEnvelope(path string) { jsonV15OnlyEnvelope[path] = true }

// BuildRegistry builds the registry of the live RootCmd tree for the given
// compat mode, uncached. It is the entry point for tools/cmdinventory, which
// documents the v1.5 contract (BuildRegistry(true)) in
// .github/command-registry.json, so the generated file, `help --json` and the
// decorator all come from the same preparation and the same data-type maps.
func BuildRegistry(v15 bool) (*cmdregistry.Registry, error) { return buildRegistry(v15) }

// JSONDataTypes returns a copy of the registered envelope data types, keyed by
// command path without the leading "nself ". tools/schemagen reads it to emit
// one schema per envelope command (P7-REG-08); the copy keeps callers from
// mutating the registration.
func JSONDataTypes() map[string]any {
	out := make(map[string]any, len(jsonDataTypes))
	for k, v := range jsonDataTypes {
		out[k] = v
	}
	return out
}
