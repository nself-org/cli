package manifestv2

import (
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

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

// CheckedCLITargets is CLITargets followed by CheckCLITargets: the binaries an
// installer may publish for the plugin, or the reason it must refuse it.
func CheckedCLITargets(name, pluginType, binaryName string, commands []string) ([]CLITarget, error) {
	t := CLITargets(name, pluginType, binaryName, commands)
	if err := CheckCLITargets(name, t); err != nil {
		return nil, err
	}
	return t, nil
}

// CheckCLITargets refuses command targets the proxy cannot serve (D-0259).
//
// A command outside NamePattern (a space, an upper-case letter, a leading
// digit) or a binary outside BinaryPattern is never reached by
// `nself <command>`; a core verb never reaches the proxy because the built-in
// command wins, so its binary would sit unused; a command declared twice
// publishes one binary over itself. plugin names the plugin in the message.
//
// Outputs: nil, E106 (name or binary pattern, duplicate) or E113 (core verb),
// for the first bad target. CoreVerbs is the canon list
// (TestCoreVerbsMatchCanon keeps it equal to internal/canon.Verbs).
func CheckCLITargets(plugin string, targets []CLITarget) error {
	seen := map[string]bool{}
	for _, t := range targets {
		if !segmentRe.MatchString(t.Command) {
			return invalidf("cliCommands", "command %q of plugin %q must match %s", t.Command, plugin, NamePattern)
		}
		if !binaryRe.MatchString(t.Binary) {
			return invalidf("binaryName", "binary %q of plugin %q must match %s", t.Binary, plugin, BinaryPattern)
		}
		if in(CoreVerbs, t.Command) {
			return errs.Newf("E113", "cliCommands %q is a core command verb and cannot be published by plugin %q", t.Command, plugin)
		}
		if seen[t.Binary] {
			return invalidf("cliCommands", "command %q of plugin %q is declared twice", t.Command, plugin)
		}
		seen[t.Binary] = true
	}
	return nil
}

// CLITargetsOf is CLITargets over a parsed manifest through its compatibility
// projection, which is what a released CLI reads from a v2 plugin.json.
// Without a commands block, carried cliCommands do not declare binaries.
func CLITargetsOf(m *Manifest) []CLITarget {
	if m.Commands == nil {
		return nil
	}
	p := Projection(m)
	names := make([]string, 0, len(p.CLICommands))
	for _, c := range p.CLICommands {
		names = append(names, c.Name)
	}
	return CLITargets(m.Name, p.PluginType, p.BinaryName, names)
}
