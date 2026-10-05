// Package canon holds the declared command canon: the embedded side table that
// records what cobra cannot know about a command (canon status, side-effect
// class, output kind, JSON support, state exit codes and flag escalations).
//
// Purpose:     contract:cli.command-registry v1 input and producer of
//
//	contract:cli.canon-fragments v1. canon.yaml keeps the schema version
//	and the core verbs; every command entry lives in
//	domains/<domain>.yaml (one fragment per domain, ADR 0016).
//
// Inputs:      canon.yaml and domains/*.yaml compiled into the binary
//
//	(go:embed), an fs.FS (LoadFS, fixtures) or bytes (Parse).
//
// Outputs:     Load: the registry view for the running compat mode; LoadRaw: the
//
//	merged raw data (canonical paths plus the move rows); both validated.
//	Tree-dependent rules (completeness, flag existence, targets) are
//	checked by internal/cmdregistry.Build.
//
// Constraints: stdlib + gopkg.in/yaml.v3 + internal/compat only (Constitution
//
//	2.1, L0). Decoding is strict: an unknown key is an error.
package canon

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/nself-org/cli/internal/compat"
	"gopkg.in/yaml.v3"
)

//go:embed canon.yaml domains/*.yaml
var embeddedFS embed.FS

// File is the decoded canon.yaml document.
type File struct {
	// SchemaVersion is the canon.yaml schema version (1).
	SchemaVersion int `yaml:"schema_version"`
	// Verbs lists the ADR 0016 core verbs in ADR order (at most MaxVerbs).
	Verbs []string `yaml:"verbs"`
	// Commands maps a command path without the leading "nself " to its entry.
	Commands map[string]Entry `yaml:"commands"`
	// Rows are the move rows of contract:cli.canon-fragments v1.
	Rows `yaml:",inline"`

	// origin maps "<kind>:<key>" to the fragment file that declared it (set by
	// LoadFS; empty for a document parsed with Parse).
	origin map[string]string
}

// Rows are the lists of contract:cli.canon-fragments v1 that describe how the
// v1.4 surface maps to the v1.5 surface. A fragment and the merged File both
// carry them. Paths are command paths without the leading "nself ".
type Rows struct {
	// Hubs are non-runnable hubs the engine creates in v1.5 mode.
	Hubs []Hub `yaml:"hubs"`
	// Moves relocate a command with its subtree (From old path, To canonical).
	Moves []Row `yaml:"moves"`
	// Shims replace an old command with a forward to an equivalent command.
	Shims []Row `yaml:"shims"`
	// RetiredHubs turn an emptied old hub into a stub forwarding to To.
	RetiredHubs []Row `yaml:"retired_hubs"`
	// Builtins are hidden framework commands (canon builtin in v1.5).
	Builtins []string `yaml:"builtins"`
	// Breakouts leave the core tree for a plugin in v1.5 mode.
	Breakouts []Breakout `yaml:"breakouts"`
	// Removed lists commands removed in v1.5 with the message to print.
	Removed []Removed `yaml:"removed"`
}

// Hub is a non-runnable command group created in v1.5 mode.
type Hub struct {
	Path    string `yaml:"path"`
	Summary string `yaml:"summary"`
}

// Row is one move, shim or retired-hub row.
type Row struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
	// Since is the release that introduces the new spelling (vX.Y.Z).
	Since string `yaml:"since"`
	// RemovalAt is the release that removes the old spelling (vX.Y.Z, after Since).
	RemovalAt string `yaml:"removal_at"`
	// Equivalence names the Go test proving a shim equals its target (shims only).
	Equivalence string `yaml:"equivalence"`
}

// Breakout is a core command that moves to a plugin.
type Breakout struct {
	From   string `yaml:"from"`
	Plugin string `yaml:"plugin"`
	To     string `yaml:"to"`
	Since  string `yaml:"since"`
}

// Removed is a command removed without replacement.
type Removed struct {
	From    string `yaml:"from"`
	Since   string `yaml:"since"`
	Message string `yaml:"message"`
}

// Entry is the declared data for one command.
type Entry struct {
	// Canon is the status (D4). Empty means subcommand at depth >= 2.
	Canon string `yaml:"canon"`
	// Target is the canonical path (without "nself ") a deprecated-shim runs.
	Target string `yaml:"target"`
	// SideEffect is the class of running the command with default flags (D5).
	SideEffect string `yaml:"side_effect"`
	// Output is the output kind (D6); empty means document.
	Output string `yaml:"output"`
	// JSON is the pre-contract JSON support, legacy or none (D7). "envelope"
	// is never declared: it is derived from the registered data types.
	JSON string `yaml:"json"`
	// ExitCodes documents state exit codes in v1.4 mode (keys "0".."125").
	ExitCodes map[string]string `yaml:"exit_codes"`
	// ExitCodesV15 documents state exit codes in v1.5 mode (D10).
	ExitCodesV15 map[string]string `yaml:"exit_codes_v15"`
	// Flags maps a flag name to the escalation it causes when set.
	Flags map[string]FlagOverride `yaml:"flags"`
	// Mode is empty (the entry exists in both modes) or "v1.4" (it exists only
	// in v1.4 mode: break-outs, removed commands, shim and retired-hub sources).
	Mode string `yaml:"mode"`
}

// FlagOverride is what setting a flag changes about its command.
type FlagOverride struct {
	// SideEffect escalates the command's class; it must outrank the command's.
	SideEffect string `yaml:"side_effect"`
	// JSON overrides the command's JSON support: legacy or none.
	JSON string `yaml:"json"`
	// Output is empty or "stream" (D6, invalidation 2026-09-30): the only
	// flag-level output value, allowed on a command whose output is document.
	Output string `yaml:"output"`
}

var (
	rawOnce sync.Once
	raw     *File
	rawErr  error

	viewOnce [2]sync.Once
	views    [2]*File
	viewErr  [2]error
)

// LoadRaw parses and validates the embedded canon.yaml and domains/*.yaml and
// returns the merged raw data: keys are canonical (v1.5) paths and the move
// rows are present. The result is memoised and read-only. tools/canongen and
// tests use it; the registry uses Load.
func LoadRaw() (*File, error) {
	rawOnce.Do(func() {
		if missing := checkRequiredFragments(embeddedFS); len(missing) > 0 {
			rawErr = NewValidationError(missing)
			return
		}
		raw, rawErr = LoadFS(embeddedFS)
	})
	return raw, rawErr
}

// Effective returns the registry view for a compat mode (see (*File).View),
// built from LoadRaw and memoised per mode. The result is read-only.
func Effective(v15 bool) (*File, error) {
	i := 0
	if v15 {
		i = 1
	}
	viewOnce[i].Do(func() {
		r, err := LoadRaw()
		if err != nil {
			viewErr[i] = err
			return
		}
		views[i], viewErr[i] = r.View(v15)
	})
	return views[i], viewErr[i]
}

// Load returns the registry view of the running compat mode, so every registry
// caller sees the right surface without knowing about modes. Callers must treat
// the returned *File as read-only.
func Load() (*File, error) {
	// compat.V15(P7-CANON-02): registry view with moved entries at their v1.4 paths -> view at canonical paths plus deprecated-shim entries
	return Effective(compat.V15())
}

// Parse strictly decodes canon.yaml bytes (unknown keys are errors) and
// validates the result. On a validation failure the error is a
// *ValidationError naming every offending path.
func Parse(data []byte) (*File, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var f File
	if err := dec.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("canon: empty document")
		}
		return nil, fmt.Errorf("canon: decode: %w", err)
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return &f, nil
}
