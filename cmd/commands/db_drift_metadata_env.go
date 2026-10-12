package commands

// Purpose: resolve what `--env` means for `nself db drift --metadata` and
// `nself db reconcile` (D-0165). Across the db group `--env` names an
// environment (resolveDBRemoteTarget); these two commands used to read it as a
// filesystem path, so `--env prod` silently scanned ./prod or failed to find it.
// Inputs:  the cobra command (its --env value) and the remote argv to run on
//          the named environment's host.
// Outputs: the project directory to read hasura/metadata/** from, or
//          handled=true when the command already ran on the remote host.
// Constraints: an existing directory still works as the project dir (the old
//              documented meaning, kept for back-compat). Any other value is an
//              environment name: unknown names are refused by resolveTarget
//              (E483) and never fall back to a directory.

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

// dbEnvDispatchFn runs remoteArgs on the environment named by --env. A package
// variable so tests can observe the dispatch without ssh.
var dbEnvDispatchFn = dispatchRemoteIfNeeded

// resolveDBMetadataDir turns the --env value into a project directory.
//
//   - empty                  -> the current working directory
//   - an existing directory  -> that directory (back-compat)
//   - an environment name    -> remoteArgs run on that env's host (handled=true,
//     the returned error is the remote result), or, when the name resolves to
//     local, the current working directory
//   - an unknown name        -> handled=true with the E483 error
//   - a remote name with the JSON envelope on -> handled=true with a usage
//     error, nothing dispatched (the remote's data is not parsed here)
func resolveDBMetadataDir(cmd *cobra.Command, remoteArgs ...string) (dir string, handled bool, err error) {
	envVal, _ := cmd.Flags().GetString(dbRemoteEnvFlag)
	if envVal != "" {
		if info, statErr := os.Stat(envVal); statErr == nil && info.IsDir() {
			// D-0525: a local ./staging directory wins over the environment
			// of the same name. Say so on stderr, so `reconcile --apply --env
			// staging` is never silently a local run.
			if _, nameErr := resolveTarget(envVal); nameErr == nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
					"note: --env %q is a directory here and also an environment name; using the directory and not contacting the environment (rename or remove ./%[1]s to target the environment)\n", envVal)
			}
			return envVal, false, nil
		}
		// A remote run streams the host's text output and its data is never
		// parsed here, so a JSON envelope for it would be invented. Refuse
		// before dispatching anything (unknown names still surface E483).
		if jsonEnvelopeOn(cmd) {
			target, resolveErr := resolveDBRemoteTarget(cmd)
			if resolveErr != nil {
				return "", true, resolveErr
			}
			if !target.Local {
				return "", true, errs.ExitWithf(errs.ExitUserError,
					"--json is not supported with a remote --env (%s) for 'nself %s': run it on the host", target.EnvName, strings.Join(remoteArgs, " "))
			}
		}
		if handled, err := dbEnvDispatchFn(cmd, remoteArgs...); handled {
			return "", true, err
		}
	}
	dir, err = os.Getwd()
	if err != nil {
		return "", false, fmt.Errorf("getting working directory: %w", err)
	}
	return dir, false, nil
}
