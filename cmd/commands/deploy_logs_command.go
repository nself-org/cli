package commands

import "github.com/nself-org/cli/sdk/go/v2/remote"

// remoteProjectLogsCommand preserves the same interpolation-file order as
// docker.ProjectEnvFiles, checking paths on the remote filesystem.
func remoteProjectLogsCommand(root string) string {
	return "cd " + remote.ShellQuote(root) + ` || exit; set -- docker compose; ` +
		`if [ -f .nself/compose.env ]; then ` +
		`if [ -f .env ]; then set -- "$@" --env-file "$PWD/.env"; fi; ` +
		`set -- "$@" --env-file "$PWD/.nself/compose.env"; fi; ` +
		`"$@" logs --tail=200 -f`
}
