package commands

// Purpose: forward a --plugin value to the remote host on a db remote dispatch (D-0232).
// Inputs: the cobra command (may carry a --plugin flag) and the remote argv a
// db subcommand built.
// Outputs: the argv with "--plugin <name>" appended, or the E401 error from
// plugin.ValidateName for an invalid name.
// Constraints: no I/O. The argv is copied, never mutated in place. A caller
// that already forwards --plugin is left alone; flag metadata distinguishes
// flags from their values during deduplication. Quoting
// happens later in shellQuoteArgs. An invalid name fails before any SSH call.
// SPORT: cli/cmd/commands — see D-0232, db_remote.go, internal/plugin/name_validate.go.

import (
	"github.com/spf13/cobra"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/plugin"
)

// withRemotePluginFlag appends "--plugin <name>" to remoteArgs when cmd carries
// a non-empty --plugin value and the caller did not already forward it (D-0232).
// --plugin is a persistent flag on `db migrate`, but the per-command builders
// forward only the flags they know, so without this a remote
// `db migrate up --plugin X --env staging` ran the whole migration set on the
// remote host. The name is validated first (an invalid one fails with zero SSH
// calls) and quoted later by shellQuoteArgs. remoteArgs is copied, never
// mutated in place.
func withRemotePluginFlag(cmd *cobra.Command, remoteArgs []string) ([]string, error) {
	f := cmd.Flags().Lookup("plugin")
	if f == nil || f.Value.String() == "" {
		return remoteArgs, nil
	}
	name := f.Value.String()
	if err := plugin.ValidateName(name); err != nil {
		return nil, err
	}
	insertAt, forwarded := cmdregistry.FlagInsertionIndex(remoteArgs, "plugin", cmd.Flags(), cmd.InheritedFlags())
	if forwarded {
		return remoteArgs, nil
	}
	out := make([]string, 0, len(remoteArgs)+2)
	out = append(out, remoteArgs[:insertAt]...)
	out = append(out, "--plugin", name)
	return append(out, remoteArgs[insertAt:]...), nil
}
