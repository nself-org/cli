package commands

// Purpose: shared flag handling for the `nself server` subcommands —
// turning --token/--token-env into a server.Client. Split out so the
// provision/list/resize/destroy handlers stay pure cobra wiring, mirroring
// access_transport.go's newAccessTransport pattern.
// Inputs: a *cobra.Command carrying --token/--token-env flags.
// Outputs: a server.Client (always a real Hetzner client in the live CLI —
// newServerClient is swapped for a fake in server_test.go so command tests
// never reach the network) or a resolution error.
// Constraints: never logs or echoes the resolved token.

import (
	"fmt"

	"github.com/nself-org/cli/internal/server"

	"github.com/spf13/cobra"
)

// newServerClient is a package-level indirection so server_test.go can
// substitute a fake Client without any handler knowing the difference. The
// real CLI never reassigns it — every live invocation resolves through
// buildServerClient to a real Hetzner-backed client.
var newServerClient = buildServerClient

// buildServerClient resolves --token/--token-env into a token and builds a
// live Hetzner Client from it. It never makes a network call itself; that
// only happens when a handler invokes an operation on the returned Client.
func buildServerClient(cmd *cobra.Command) (server.Client, error) {
	explicit, _ := cmd.Flags().GetString("token")
	envVar, _ := cmd.Flags().GetString("token-env")

	token, err := server.ResolveToken(explicit, envVar)
	if err != nil {
		return nil, fmt.Errorf("nself server: %w", err)
	}
	return server.NewHetznerClient(token), nil
}
