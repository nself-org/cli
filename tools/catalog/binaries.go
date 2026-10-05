package main

// binaries.go: which command binaries a registry entry makes a released CLI require.
//
// Purpose: v1.4.12 turns an entry's cliCommands names, binaryName and pluginType
// into required binaries (internal/plugin/cli_binary.go:58 cliBinaryNames) and
// fails the install, rolling it back, when the archive lacks one. A manifest alone
// cannot say whether the published archive ships the binary (encryption, mail and
// pentest-kit read like the plugins that do, yet are source-only today), so the
// linking keys are emitted only for a release whose releases.json row lists its
// binaries, and only when that list covers every binary the keys require.
// Inputs: a manifest and its release row. Outputs: the keys to emit, or a problem.
// Constraints: requiredBinaries is a copy of cliBinaryNames and cliBinaryName at
// v1.4.12; released_cli_test.go proves the copy against the real committed shape.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
	"github.com/nself-org/cli/tools/catalog/model"
)

// linkKeys are the entry keys that make a released CLI require binaries.
type linkKeys struct {
	pluginType, binaryName string
	cliCommands            []model.CLICommand
}

// requiredBinaries is v1.4.12 cliBinaryNames over the keys an entry would carry.
func requiredBinaries(slug string, k linkKeys) []string {
	if k.pluginType != "" && k.pluginType != "cli" {
		return nil
	}
	var out []string
	for _, c := range k.cliCommands {
		if c.Name != "" {
			out = append(out, "nself-"+c.Name)
		}
	}
	if len(out) > 0 {
		return out
	}
	switch {
	case k.binaryName != "":
		return []string{k.binaryName}
	case k.pluginType == "cli":
		return []string{"nself-" + slug}
	}
	return nil
}

// linking decides the linking keys of one entry. A single cliCommands entry
// repeats binaryName, so the list is carried only when there are several.
func linking(m *manifestv2.Manifest, r model.Release) (linkKeys, string) {
	if len(r.Binaries) == 0 {
		return linkKeys{}, ""
	}
	k := linkKeys{pluginType: m.PluginType, binaryName: m.BinaryName}
	if len(m.CLICommands) > 1 {
		for _, c := range m.CLICommands {
			k.cliCommands = append(k.cliCommands, model.CLICommand{Name: c.Name, Description: c.Description})
		}
	}
	req := requiredBinaries(m.Name, k)
	shipped := map[string]bool{}
	for _, b := range r.Binaries {
		shipped[b] = true
	}
	var missing []string
	for _, b := range req {
		if !shipped[b] {
			missing = append(missing, b)
		}
	}
	if len(missing) > 0 {
		return linkKeys{}, fmt.Sprintf("%s: the manifest requires binaries %s but the release lists %s", m.Name,
			strings.Join(missing, ", "), strings.Join(r.Binaries, ", "))
	}
	if len(req) == 0 {
		return linkKeys{}, ""
	}
	return k, ""
}
