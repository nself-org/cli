package manifestv2

import "strings"

// CLITarget is one command binary a CLI plugin publishes: the binary file name
// the command proxy execs and the command that names it.
type CLITarget struct {
	Binary  string
	Command string
}

// CLITargets returns every command binary a plugin installs, by the installer's
// rule (P7-PLUG-55). It is the one definition: internal/plugin cliBinaryNames
// calls it, and manifestv2migrate -targets prints it, so the release pipeline
// builds exactly what the installer will look for.
//
// Inputs are the v1 view of a manifest (a v2 manifest through Projection):
// pluginType, binaryName and the cliCommands names. A plugin whose pluginType is
// set and is not "cli" has none. Each cliCommands entry with a name publishes
// nself-<name>. With no usable entry, an explicit binaryName is the single
// binary, and pluginType "cli" without one falls back to nself-<name>. The
// command of a single binary is its name without the nself- prefix.
func CLITargets(name, pluginType, binaryName string, commands []string) []CLITarget {
	if pluginType != "" && pluginType != "cli" {
		return nil
	}
	var out []CLITarget
	for _, c := range commands {
		if c != "" {
			out = append(out, CLITarget{Binary: "nself-" + c, Command: c})
		}
	}
	if len(out) > 0 {
		return out
	}
	bin := binaryName
	if bin == "" && pluginType == "cli" {
		bin = "nself-" + name
	}
	if bin == "" {
		return nil
	}
	return []CLITarget{{Binary: bin, Command: strings.TrimPrefix(bin, "nself-")}}
}

// CLITargetsOf is CLITargets over a parsed manifest through its compatibility
// projection, which is what a released CLI reads from a v2 plugin.json.
func CLITargetsOf(m *Manifest) []CLITarget {
	p := Projection(m)
	names := make([]string, 0, len(p.CLICommands))
	for _, c := range p.CLICommands {
		names = append(names, c.Name)
	}
	return CLITargets(m.Name, p.PluginType, p.BinaryName, names)
}
