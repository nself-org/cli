// Package canon holds the declared command canon: the one embedded side table
// (canon.yaml) that records what cobra cannot know about a command (canon
// status, side-effect class, output kind, JSON support, state exit codes and
// flag escalations).
//
// Purpose:     contract:cli.command-registry v1 input; ADR 0016 names
//
//	internal/canon/canon.yaml as the machine form of the command canon.
//
// Inputs:      canon.yaml compiled into the binary (go:embed), or bytes passed
//
//	to Parse.
//
// Outputs:     a validated *File. Tree-dependent rules (completeness, flag
//
//	existence, targets) are checked by internal/cmdregistry.Build.
//
// Constraints: stdlib + gopkg.in/yaml.v3 only (Constitution 2.1, L0). Decoding
//
//	is strict: an unknown key is an error.
package canon

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed canon.yaml
var embedded []byte

// File is the decoded canon.yaml document.
type File struct {
	// SchemaVersion is the canon.yaml schema version (1).
	SchemaVersion int `yaml:"schema_version"`
	// Verbs lists the ADR 0016 core verbs in ADR order (at most MaxVerbs).
	Verbs []string `yaml:"verbs"`
	// Commands maps a command path without the leading "nself " to its entry.
	Commands map[string]Entry `yaml:"commands"`
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
	loadOnce sync.Once
	loaded   *File
	loadErr  error
)

// Load parses and validates the embedded canon.yaml. The result is memoised;
// callers must treat the returned *File as read-only.
func Load() (*File, error) {
	loadOnce.Do(func() {
		loaded, loadErr = Parse(embedded)
	})
	return loaded, loadErr
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
