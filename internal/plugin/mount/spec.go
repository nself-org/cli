// Package mount discovers the CLI command surface an installed plugin
// declares (contract:cli.plugin-command-mount v1).
//
// Purpose:     turn installed plugin manifests into mount Specs plus Problems,
//
//	without touching cobra, canon YAML or the network.
//
// Inputs:      a plugins directory (the same one internal/plugin installs
//
//	into) and the core verbs, passed in by the caller from the
//	generated canon table.
//
// Outputs:     []Spec (sorted by slug) and []Problem (deterministic order),
//
//	carrying the loader's error code for rejected manifests.
//
// Constraints: disk only, no network; manifests are read only through
//
//	manifestv2 (the one manifest reader); a Spec is emitted only when
//	its command can be mounted and its binary is executable and inside
//	the plugins directory after symlink resolution.
package mount

import "encoding/json"

// Annotation keys the mount writes on every cobra node it creates. The
// registry (internal/cmdregistry) reads them as plain strings; it must not
// import this package.
const (
	// AnnPlugin is the slug of the plugin that mounted the node.
	AnnPlugin = "nself.plugin"
	// AnnSource is "installed" or "builtin".
	AnnSource = "nself.mount.source"
	// AnnSideEffect, AnnOutput and AnnJSON carry the surface attributes.
	AnnSideEffect = "nself.side_effect"
	AnnOutput     = "nself.output"
	AnnJSON       = "nself.json"
	// AnnArgs and AnnFlags carry the declared args/flags as JSON.
	AnnArgs  = "nself.args"
	AnnFlags = "nself.flags"
	// AnnConfirm carries the confirm declaration as compact JSON (absent
	// when the manifest declares none).
	AnnConfirm = "nself.confirm"
	// AnnSurface carries the surface declaration (absent when none).
	AnnSurface = "nself.surface"
)

// Spec is one plugin's mountable command surface.
type Spec struct {
	Slug        string // plugin slug (the manifest directory name)
	Command     string // the mounted root command name
	Binary      string // the binary name declared by the manifest
	BinaryPath  string // the resolved, validated binary path
	Summary     string
	SideEffect  string
	Output      string
	JSON        string
	Args        json.RawMessage
	Flags       json.RawMessage
	Confirm     json.RawMessage
	Surface     string
	Subcommands []Sub
}

// Sub is one nested command path below the Spec root.
type Sub struct {
	Path       []string // segments below the root command
	Summary    string
	SideEffect string
	Output     string
	JSON       string
	Args       json.RawMessage
	Flags      json.RawMessage
	Confirm    json.RawMessage
	Surface    string
}

// Problem is why a plugin was not (fully) mounted. Code is an errs code
// (E405 collision, E406 binary, or the manifest loader's code such as E113).
type Problem struct {
	Code    string
	Slug    string
	Message string
}

// Defaults for absent surface attributes (contract v1): an undeclared
// side_effect is destructive, output is a document, json is none.
const (
	DefaultSideEffect = "destructive"
	DefaultOutput     = "document"
	DefaultJSON       = "none"
)
