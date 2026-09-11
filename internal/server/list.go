package server

// Purpose: `nself server list` business logic — a thin pass-through to
// Client.ListServers. Exists as its own function (rather than the command
// calling the client directly) so tests and any future caller share one
// entry point, matching how List/Grant/Revoke work in internal/access.
// Inputs: a Client and ListOptions.
// Outputs: the servers Hetzner reports, unmodified.

import "context"

// List returns every server visible to client, optionally filtered by
// opts.LabelSelector.
func List(ctx context.Context, client Client, opts ListOptions) ([]Server, error) {
	return client.ListServers(ctx, opts)
}
