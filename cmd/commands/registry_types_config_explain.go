package commands

// registry_types_config_explain.go — the envelope data of `nself config explain`.
//
// Purpose: `config explain` answers with the v1 envelope in both compat modes
//          (a new command, so there is no pre-contract JSON to keep). This file
//          holds the data types, their registration and the one function that
//          assembles them from a config.Explanation, the bindings and the
//          registry, so the human and the JSON output show the same facts.
// Inputs:  config.ExplainKey result, bindings.Bindings, *cmdregistry.Registry.
// Outputs: configExplainData placed in the envelope `data`.
// Constraints: values are redacted unless reveal is set: a redacted value is
//          JSON null and `revealed` is false. Nothing here reads the disk.

import (
	"sort"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/config/bindings"
)

func init() {
	registerJSONType("config explain", configExplainData{})
}

// nselfYAMLNote is the nself.yaml row of the explanation: the file holds no
// configuration keys (EPIC D6).
const nselfYAMLNote = "not a source; nself.yaml carries app, bundle, bundles, plugins"

// configExplainFlag is one flag that supplies the key, with the machine
// surfaces that carry it. The surface fields are empty for a cli-only command
// or flag.
type configExplainFlag struct {
	Command       string `json:"command"`        // "nself init"
	Flag          string `json:"flag"`           // "domain"
	MCPTool       string `json:"mcp_tool"`       // "nself_init"
	MCPParameter  string `json:"mcp_parameter"`  // "flags.domain"
	HTTPRoute     string `json:"http_route"`     // "/v1/commands/init"
	HTTPParameter string `json:"http_parameter"` // "flags.domain"
}

// configExplainSource is one place that sets the key, lowest precedence first.
type configExplainSource struct {
	Source string  `json:"source"` // cascade file name or "process environment"
	Kind   string  `json:"kind"`   // file | process
	Value  *string `json:"value"`  // null unless --reveal
	Winner bool    `json:"winner"`
}

// configExplainEffective is the source the loader uses and its value.
type configExplainEffective struct {
	Source string  `json:"source"` // file name, "process environment", "default" or "unset"
	Value  *string `json:"value"`  // null unless --reveal (and for "unset")
}

// configExplainData is the data of `config explain --json`.
type configExplainData struct {
	Key          string                 `json:"key"`
	EnvName      string                 `json:"env_name"`
	Known        bool                   `json:"known"`
	Default      string                 `json:"default"`
	Environment  string                 `json:"environment"`
	CascadeMode  string                 `json:"cascade_mode"` // canonical | legacy
	Flags        []configExplainFlag    `json:"flags"`
	NselfYAML    string                 `json:"nself_yaml"`
	Sources      []configExplainSource  `json:"sources"`
	Effective    configExplainEffective `json:"effective"`
	Revealed     bool                   `json:"revealed"`
	ResolvedFrom *string                `json:"resolved_from"` // "init --domain" when asked by flag
}

// buildConfigExplain assembles the explanation of exp's key. v15 selects the
// command spelling reported for bound flags (the registry's own paths).
func buildConfigExplain(exp config.Explanation, b bindings.Bindings, reg *cmdregistry.Registry, v15, reveal bool, resolvedFrom string) configExplainData {
	show := func(v string) *string {
		if !reveal {
			return nil
		}
		return &v
	}
	mode := "canonical"
	if exp.Legacy {
		mode = "legacy"
	}
	data := configExplainData{
		Key: exp.Key, EnvName: exp.Key, Known: exp.Known, Default: exp.Default,
		Environment: exp.Env, CascadeMode: mode, NselfYAML: nselfYAMLNote,
		Flags: []configExplainFlag{}, Sources: []configExplainSource{}, Revealed: reveal,
	}
	if resolvedFrom != "" {
		data.ResolvedFrom = &resolvedFrom
	}
	for _, r := range b.ForKey(exp.Key) {
		data.Flags = append(data.Flags, explainFlag(reg, treePath(r.Command, v15), r.Flag))
	}
	sort.Slice(data.Flags, func(i, j int) bool {
		if data.Flags[i].Command != data.Flags[j].Command {
			return data.Flags[i].Command < data.Flags[j].Command
		}
		return data.Flags[i].Flag < data.Flags[j].Flag
	})

	effSource, effValue := exp.Effective()
	// The loader applies every cascade file over the process environment, so the
	// process environment is the lowest source (and wins only when no file sets it).
	if exp.ProcessSet {
		data.Sources = append(data.Sources, configExplainSource{
			Source: "process environment", Kind: "process", Value: show(exp.ProcessValue),
			Winner: effSource == "process environment",
		})
	}
	for _, s := range exp.Setters {
		data.Sources = append(data.Sources, configExplainSource{
			Source: s.File, Kind: "file", Value: show(s.Value), Winner: s.File == effSource,
		})
	}
	data.Effective = configExplainEffective{Source: effSource, Value: show(effValue)}
	if effSource == "unset" {
		data.Effective.Value = nil
	}
	return data
}

// explainFlag reports one bound flag with its machine surface parameters.
func explainFlag(reg *cmdregistry.Registry, path, flag string) configExplainFlag {
	out := configExplainFlag{Command: "nself " + path, Flag: flag}
	if path == bindings.Root || reg == nil {
		return out
	}
	cmd, ok := reg.Lookup(path)
	if !ok || cmd.Surface == "cli-only" {
		return out
	}
	for _, f := range cmd.Flags {
		if f.Name == flag && !f.CLIOnly {
			out.MCPTool = cmdregistry.ToolName(cmd.Path)
			out.MCPParameter = "flags." + flag
			out.HTTPRoute = cmdregistry.RoutePath(cmd.Path)
			out.HTTPParameter = "flags." + flag
		}
	}
	return out
}
