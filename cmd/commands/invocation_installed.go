package commands

// Pre-run entry of the invocation decorator. Installed plugin commands take a
// separate path: the mount parses no flags, so the invocation record is built
// from the forwarded args.

import (
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

// enterInstalledInvocation records the invocation of an installed plugin
// command. The mount parses no flags (DisableFlagParsing), so the --json the
// plugin receives is only in the forwarded args: the intent is read from them,
// otherwise a proxy that fails to start (binary missing, not executable) would
// be reported as text although the caller asked for JSON (D-0614). Nothing is
// refused here: the plugin owns its output.
func enterInstalledInvocation(cmd *cobra.Command, args []string) error {
	output.SetInvocation(invokedKey(cmd), output.JSONRequestedFromArgs(args))
	return imageSwitchGuard(cmd)
}

// guardedPre checks before hooks can fetch, cache, or change directories.
func guardedPre(orig func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) (err error) {
		defer func() {
			if err != nil && cmd == secretsResolveCmd {
				err = auditResolvePreRunError(cmd, args, err)
			}
		}()
		if cmd.Annotations["nself.mount.source"] == "installed" {
			if err := enterInstalledInvocation(cmd, args); err != nil {
				return err
			}
			return orig(cmd, args)
		}
		if err := enterInvocation(cmd); err != nil {
			return err
		}
		return orig(cmd, args)
	}
}
