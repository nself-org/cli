// Package cmdregistry derives the deterministic command registry document
// (contract:cli.command-registry v1) from a cobra tree, the declared canon data
// and the JSON data-type map.
//
// Purpose:     one machine-readable description of the CLI command surface.
// Inputs:      a cobra root (read only), *canon.File, a map of registered JSON
//
//	data types keyed by command path without "nself ", BuildOptions.
//
// Outputs:     *Registry, marshalled with Marshal (2-space indent, no HTML
//
//	escaping, trailing newline). Field names and order are a
//	cross-repo contract; a rename after merge is a v2.
//
// Constraints: stdlib + cobra + pflag + internal/canon only. Build never
//
//	mutates the tree and does not import compat (the caller passes
//	compat.V15() as BuildOptions.V15).
package cmdregistry

// SchemaVersion is the registry document version.
const SchemaVersion = "1"

// Registry is the whole document.
type Registry struct {
	SchemaVersion string    `json:"schema_version"`
	Verbs         []string  `json:"verbs"`
	Root          Root      `json:"root"`
	Commands      []Command `json:"commands"`
	Counts        Counts    `json:"counts"`

	rootPath string
	index    map[string]int
}

// Root describes the root command.
type Root struct {
	Summary string `json:"summary"`
	Flags   []Flag `json:"flags"`
}

// Command is one non-root cobra command.
type Command struct {
	Path       string            `json:"path"`
	Name       string            `json:"name"`
	Parent     string            `json:"parent"`
	Summary    string            `json:"summary"`
	Hidden     bool              `json:"hidden"`
	Deprecated *string           `json:"deprecated"`
	Aliases    []string          `json:"aliases"`
	Group      *string           `json:"group"`
	Runnable   bool              `json:"runnable"`
	Args       []Arg             `json:"args"`
	Flags      []Flag            `json:"flags"`
	Canon      string            `json:"canon"`
	Plugin     *string           `json:"plugin,omitempty"`
	Target     *string           `json:"target"`
	SideEffect string            `json:"side_effect"`
	Output     string            `json:"output"`
	JSON       string            `json:"json"`
	DataSchema *string           `json:"data_schema"`
	ExitCodes  map[string]string `json:"exit_codes"`
}

// Arg is one positional argument parsed from the command's Use string.
type Arg struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Variadic bool   `json:"variadic"`
}

// Flag is one flag declared on a command (or on the root).
type Flag struct {
	Name       string  `json:"name"`
	Shorthand  *string `json:"shorthand"`
	Type       string  `json:"type"`
	Default    string  `json:"default"`
	Usage      string  `json:"usage"`
	Hidden     bool    `json:"hidden"`
	Deprecated *string `json:"deprecated"`
	Required   bool    `json:"required"`
	Persistent bool    `json:"persistent"`
	Env        *string `json:"env"`
	SideEffect *string `json:"side_effect"`
	JSON       *string `json:"json"`
	Output     *string `json:"output"`
}

// Counts are the derived totals.
type Counts struct {
	Commands        int      `json:"commands"`
	TopLevel        int      `json:"top_level"`
	Core            int      `json:"core"`
	Pending         int      `json:"pending"`
	DeprecatedShims int      `json:"deprecated_shims"`
	Plugin          int      `json:"plugin,omitempty"`
	CoreMissing     []string `json:"core_missing"`
	JSONEnvelope    int      `json:"json_envelope"`
	JSONLegacy      int      `json:"json_legacy"`
}

// BuildOptions selects the compat mode and the v1.5-only envelope paths.
type BuildOptions struct {
	// V15 selects ExitCodesV15 and enables V15OnlyEnvelope paths. The caller
	// passes compat.V15().
	V15 bool
	// V15OnlyEnvelope lists paths (without "nself ") whose registered data
	// type reports json envelope only when V15 is true (EPIC D8).
	V15OnlyEnvelope map[string]bool
}
