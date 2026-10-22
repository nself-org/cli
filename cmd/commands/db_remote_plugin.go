package commands

// Purpose: forward a --plugin value to the remote host on a db remote dispatch (D-0232).
// Inputs: the cobra command (may carry a --plugin flag) and the remote argv a
// db subcommand built.
// Outputs: the argv with "--plugin <name>" appended, or the E401 error from
// validatePluginFlag for an invalid name.
// Constraints: no I/O. The argv is copied, never mutated in place. A caller
// that already forwards --plugin is left alone; the dedup matches the exact
// "--plugin" flag, consuming other flags' values before deduplication. Quoting
// happens later in shellQuoteArgs. An invalid name fails before any SSH call.
// SPORT: cli/cmd/commands — see D-0232, db_remote.go, plugin_flag.go.

import (
	"strings"

	"github.com/spf13/cobra"
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
	if err := validatePluginFlag(name); err != nil {
		return nil, err
	}
	for i := 0; i < len(remoteArgs); i++ {
		a := remoteArgs[i]
		if a == "--" {
			break
		}
		if a == "--plugin" || strings.HasPrefix(a, "--plugin=") {
			return remoteArgs, nil
		}
		// A value may itself spell --plugin. Consume values using the local
		// command's flag definitions, just as the remote parser will.
		if strings.HasPrefix(a, "--") && !strings.Contains(a, "=") {
			if flag := cmd.Flags().Lookup(strings.TrimPrefix(a, "--")); flag != nil && flag.NoOptDefVal == "" {
				i++
			}
		}
	}
	out := make([]string, 0, len(remoteArgs)+2)
	out = append(out, remoteArgs...)
	return append(out, "--plugin", name), nil
}
