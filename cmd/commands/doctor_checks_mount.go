package commands

// Doctor check for the plugin-command mount (contract:cli.plugin-command-mount
// v1).
//
// Purpose: surface every mount problem — rejected manifests, unusable
// binaries, command collisions — as doctor warn rows.
//
// Inputs: the prepared cobra root and the verbose flag.
//
// Outputs: []doctorCheckResult (warn rows only).
//
// Constraints: a machine with no mount problems must get identical doctor
// output to before this contract, so a clean install emits no row at all; the
// tree-collision half is derived with the same planMount the mount uses, so
// doctor and runtime can never disagree.

import (
	"fmt"

	"github.com/nself-org/cli/internal/plugin/mount"
	"github.com/spf13/cobra"
)

// checkPluginMount reports one warn row per mount Problem: the Discover half
// (manifest rejected, binary missing or outside the plugins dir, two plugins
// claiming one command) plus the tree half (a command a cobra-native command
// already owns). Emitted only when a problem exists.
func checkPluginMount(root *cobra.Command, verbose bool) []doctorCheckResult {
	specs, problems := mount.Discover(resolvePluginDir(), canonTable.Verbs)
	// planMount against the prepared tree: specs that did mount are annotated
	// nodes and invisible to nativeClash, so only real collisions surface.
	_, treeProblems := planMount(root, specs)
	problems = append(problems, treeProblems...)
	var results []doctorCheckResult
	for _, p := range problems {
		name := fmt.Sprintf("Plugin mount: %s", safeMountText(p.Slug))
		message := safeMountText(p.Message)
		printCheck("warn", name, message, verbose)
		results = append(results, doctorCheckResult{Name: name, Status: "warn", Message: message})
	}
	return results
}
