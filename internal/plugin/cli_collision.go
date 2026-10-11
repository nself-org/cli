package plugin

import (
	"path/filepath"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

// checkCLIBinaries refuses a CLI plugin whose command binaries are unsafe to
// publish (D-0259), before anything is written to the proxy's bin directory.
//
// Purpose: linkCLIBinary copies every binary cliCommands names into
// ~/.nself/plugins/bin and replaces an existing file without asking whose it
// is. Five free plugins declared their subcommands (init, server, status, add)
// as cliCommands, so one install could publish nself-init, which the built-in
// `nself init` shadows, or replace another plugin's nself-server.
//
// Inputs: the extracted plugin directory (its parent is the plugins directory
// whose installed manifests are read), the plugin name and its manifest.
//
// Outputs: nil, or the first refusal: E106 or E113 from
// manifestv2.CheckCLITargets for a bad name or a core verb, E141 for a binary
// another installed plugin already provides.
//
// Constraints: the plugin itself is skipped, so an upgrade or a reinstall is
// never a collision. A bin file no installed manifest claims is an orphan and
// is replaced as before. Lives apart from cli_binary.go so the collision rule
// stays one unit (cli#643 rewrites that file).
func checkCLIBinaries(destDir, name string, m *PluginManifest) error {
	if m == nil {
		return nil
	}
	cmds := make([]string, 0, len(m.CLICommands))
	for _, c := range m.CLICommands {
		cmds = append(cmds, c.Name)
	}
	targets, err := manifestv2.CheckedCLITargets(name, m.PluginType, m.BinaryName, cmds)
	if err != nil || len(targets) == 0 {
		return err
	}
	owners, err := installedCLIBinaryOwners(filepath.Dir(destDir), name)
	if err != nil {
		return err
	}
	for _, t := range targets {
		if other, ok := owners[t.Binary]; ok {
			return errs.Newf("E141", "plugin %q provides the %q command (%s), which installed plugin %q already provides", name, t.Command, t.Binary, other)
		}
	}
	return nil
}

// installedCLIBinaryOwners maps each command binary an installed plugin other
// than self publishes to that plugin's name. Directories without a readable
// manifest are skipped, as LoadManifestsFromDir does.
func installedCLIBinaryOwners(pluginDir, self string) (map[string]string, error) {
	manifests, err := LoadManifestsFromDir(pluginDir)
	if err != nil {
		return nil, err
	}
	owners := map[string]string{}
	for _, om := range manifests {
		if om.Name == self {
			continue
		}
		for _, bin := range cliBinaryNames(om.Name, om) {
			if _, taken := owners[bin]; !taken {
				owners[bin] = om.Name
			}
		}
	}
	return owners, nil
}
