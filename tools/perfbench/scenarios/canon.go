package scenarios

// Canon scenarios (EPIC P7-CANON D15): what the canon engine, the plugin-command
// mount and plugin discovery cost at startup, where the cost lands.
//
//	cold-start-plugins      the three G8 probes with a HOME holding 31 CLI plugins
//	cold-start-plugins-v15  the same, with NSELF_V15=1 (relocation + mount + hubs)
//
// The HOME holds 31 generated CLI-plugin manifests (manifest v2 `commands` block
// plus the v1 compatibility keys) and stub binaries, the real v1 CLI-plugin count.
// Metric names are cold_start.version|help|status, as in the cold-start scenario,
// so `perfbench check -budget .github/perf-budget.json` applies unchanged.
//
// Not a Prober: the probes need the generated HOME, which `perfbench ab` does not
// provide. scripts/ci/perf-ab-scenario.sh compares base and head through
// `perfbench run -scenario ... -json` instead.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PluginCount is the number of CLI plugins in the generated HOME (the v1 count).
const PluginCount = 31

type coldStartPlugins struct{ v15 bool }

func init() {
	Register(coldStartPlugins{v15: false})
	Register(coldStartPlugins{v15: true})
}

func (s coldStartPlugins) Name() string {
	if s.v15 {
		return "cold-start-plugins-v15"
	}
	return "cold-start-plugins"
}

// Probes are the EPIC G8 probes: `status` runs in an empty directory and is
// expected to exit 1.
func (coldStartPlugins) Probes() []Probe {
	return []Probe{
		{Metric: "cold_start.version", Args: []string{"version"}, Exit: 0},
		{Metric: "cold_start.help", Args: []string{"--help"}, Exit: 0},
		{Metric: "cold_start.status", Args: []string{"status"}, Exit: 1},
	}
}

// Run builds the plugin HOME, points every probe at it and measures.
func (s coldStartPlugins) Run(ctx context.Context, cfg Config) ([]Sample, error) {
	home, err := os.MkdirTemp("", "perfbench-plugins-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(home) }()
	if err := WritePluginHome(home, PluginCount); err != nil {
		return nil, err
	}
	cfg.Env = append(append([]string{}, cfg.Env...), "HOME="+home) // the later HOME wins over the probe's empty one
	if s.v15 {
		cfg.Env = append(cfg.Env, "NSELF_V15=1")
	}
	return RunProbes(ctx, cfg, s.Probes())
}

// WritePluginHome creates n CLI plugins under home/.nself/plugins: one
// <slug>/plugin.json each and one executable stub per binary in plugins/bin.
func WritePluginHome(home string, n int) error {
	root := filepath.Join(home, ".nself", "plugins")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		slug, binary := PluginName(i)
		dir := filepath.Join(root, slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		doc, err := json.MarshalIndent(pluginManifest(slug, binary), "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), append(doc, '\n'), 0o644); err != nil {
			return err
		}
		stub := []byte("#!/bin/sh\nexit 0\n")
		if err := os.WriteFile(filepath.Join(bin, binary), stub, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// PluginName returns the slug and binary of generated plugin i. The command name
// is never an ADR 0016 verb: names are tool-0 ... tool-30.
func PluginName(i int) (slug, binary string) {
	return fmt.Sprintf("tool%d-cli", i), fmt.Sprintf("nself-tool%d", i)
}

// pluginManifest is a plugin.json with a v2 `commands` block and the v1 keys a
// 1.4.x CLI reads (binaryName, pluginType cli, cliCommands).
func pluginManifest(slug, binary string) map[string]any {
	command := binary[len("nself-"):]
	return map[string]any{
		"manifest_version": 2,
		"name":             slug,
		"version":          "1.0.0",
		"description":      "Generated CLI plugin for the cold-start scenarios",
		"license":          "MIT",
		"licenseType":      "free",
		"minNselfVersion":  "1.5.0",
		"pluginType":       "cli",
		"binaryName":       binary,
		"category":         "infrastructure",
		"cliCommands": []map[string]string{
			{"name": command, "binary": binary, "description": "Generated command"},
		},
		"commands": map[string]any{
			"command":     command,
			"binary":      binary,
			"summary":     "Generated command " + command,
			"side_effect": "read",
			"output":      "document",
			"json":        "none",
			"subcommands": []map[string]any{
				{"name": "list", "summary": "List things", "side_effect": "read", "flags": []map[string]any{{"name": "limit", "type": "int"}}},
				{"name": "show", "summary": "Show one thing", "side_effect": "read", "args": []map[string]any{{"name": "id", "required": true}}},
				{"name": "item add", "summary": "Add an item", "side_effect": "write"},
			},
		},
	}
}
