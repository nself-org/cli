package commands

// Tree preparation: everything that decides WHICH command runs, before cobra.
//
// Purpose: Execute's pre-dispatch steps live here so dispatch.go stays small and
// the doc generators can prepare the same tree the binary runs (EPIC P7-CANON
// D2, D6).
//
// Inputs: os.Args, the cobra tree, the compat mode, the generated canon table.
//
// Outputs: a rewritten os.Args, a prepared tree and an undo func.
//
// Constraints: order is fixed (EPIC D2/D6): normalizeInvokedBinary,
// rewriteLegacyInvocation, rewriteCanonArgs, prepareTree (builtin mount hook,
// relocation, groups, installed mount hook), then Execute's unknown-command
// intercept, the invocation decorator and RootCmd.Execute. A rewrite whose
// destination did not materialise in the tree is undone: an argv never lands on
// a command the engine did not put there.

import (
	"os"

	"github.com/nself-org/cli/internal/compat"
	"github.com/spf13/cobra"
)

// mountBuiltinFamilies and mountInstalledPlugins are the plugin-command mount
// hooks P7-CANON-01 fills; they do nothing until then.
var (
	mountBuiltinFamilies  = func(root *cobra.Command) {}
	mountInstalledPlugins = func(root *cobra.Command) {}
)

// prepareTree applies the canon to root for the mode: builtin mount hook,
// relocation (applyCanon), command groups, then the installed mount hook when
// installed is true. The returned func undoes the canon relocation.
func prepareTree(root *cobra.Command, v15, installed bool) func() {
	return prepareTreeWith(&canonTable, root, v15, installed)
}

// prepareTreeWith is prepareTree over an explicit table.
func prepareTreeWith(t *canonTableT, root *cobra.Command, v15, installed bool) func() {
	mountBuiltinFamilies(root)
	undo := applyCanonWith(t, root, v15)
	applyGroups(root)
	if installed {
		mountInstalledPlugins(root)
	}
	return undo
}

// PrepareTreeForGeneration prepares RootCmd as the v1.5 surface for the tools
// that document it (registry, inventory, wiki, parity). Installed plugin
// commands are never mounted: generated output is hermetic.
func PrepareTreeForGeneration() func() {
	return prepareTree(RootCmd, true, false)
}

// prepareInvocation runs the pre-dispatch steps for the current process.
func prepareInvocation() error { return prepareInvocationWith(&canonTable, RootCmd) }

// prepareInvocationWith is prepareInvocation over an explicit table and root.
func prepareInvocationWith(t *canonTableT, root *cobra.Command) error {
	normalizeInvokedBinary()
	// CLI-R09: retired spellings first; the plugin proxy would otherwise try to
	// resolve a name that is no longer registered.
	legacy := rewriteLegacyInvocation()
	// compat.V15(P7-CANON-21): the argv names commands at their v1.4 paths -> argv rewritten between old and canonical spellings, tree relocated
	v15 := compat.V15()
	orig := append([]string{}, os.Args...)
	args, notes, err := rewriteCanonArgsWith(t, root, processArgs(), v15)
	if err != nil {
		return err
	}
	if len(orig) > 0 {
		os.Args = append([]string{orig[0]}, args...)
	}
	prepareTreeWith(t, root, v15, true)
	if kept := confirmRewrites(root, notes); len(kept) != len(notes) {
		os.Args, notes = orig, nil
	}
	warnLegacyChain(legacy, notes)
	if v15 {
		return hubPreflight(root, processArgs())
	}
	return nil
}

// confirmRewrites returns the notes whose destination exists in the prepared
// tree: a move needs the relocated node at its new path, a shim or retired hub
// needs its stub and a live target, a break-out needs the plugin mounted at the
// new path. A rewrite that fails the check is dropped (the caller restores the
// original argv), so an unknown or half-applied row never runs another command.
func confirmRewrites(root *cobra.Command, notes []canonNote) []canonNote {
	var kept []canonNote
	for _, n := range notes {
		dest := walkNames(root, n.To)
		ok := dest != nil && dest.Annotations[annStub] == ""
		switch n.Kind {
		case "move":
			ok = ok && dest.Annotations[annMovedFrom] == n.Old
		case "breakout":
			ok = ok && mountedBy(dest, n.Plugin)
		default:
			stub := walkNames(root, n.From)
			ok = ok && stub != nil && stub.Annotations[annStub] != ""
		}
		if ok {
			kept = append(kept, n)
		}
	}
	return kept
}

// mountedBy reports whether c, or the command it hangs from, is mounted from the
// plugin (annotation nself.plugin, set by the plugin-command mount).
func mountedBy(c *cobra.Command, plugin string) bool {
	for ; c != nil; c = c.Parent() {
		if c.Annotations["nself.plugin"] == plugin {
			return true
		}
	}
	return false
}

// hubPreflight is the v1.5 unknown-subcommand check for a hub that has no body
// (cobra shows its help and exits 0 before any Args validator runs): extra
// command words below it are an unknown subcommand (E401, D-0062).
func hubPreflight(root *cobra.Command, args []string) error {
	start := commandStart(root, args)
	if start < 0 {
		return nil
	}
	end := start
	for end < len(args) && len(args[end]) > 0 && args[end][0] != '-' {
		end++
	}
	cmd, rest, err := root.Find(args[start:end])
	if err != nil || cmd == nil || cmd == root || cmd.Runnable() || !cmd.HasSubCommands() || len(rest) == 0 {
		return nil
	}
	return hubUnknownSubcommand(cmd, rest)
}
