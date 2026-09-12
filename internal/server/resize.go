package server

// Purpose: `nself server resize` business logic. Design requirement 3:
// Hetzner refuses to shrink a server's disk — the only path is
// snapshot-old -> create-new -> restore-from-snapshot -- so this must be
// detected and explained here, before ever calling the provider, rather
// than surfacing Hetzner's raw "invalid_input" error to the operator.
// Inputs: a Client, the target server ID, and a ResizeRequest.
// Outputs: the Action Hetzner started, or ErrDiskShrink with a plain-English
// explanation of the snapshot-based workaround.
// Constraints: the disk-shrink check requires one extra API call
// (ListServerTypes) to compare current vs. target disk size; this is
// deliberate — Hetzner's own error message for this case does not explain
// the workaround, so we do the comparison ourselves.

import (
	"context"
	"errors"
	"fmt"
)

// ErrDiskShrink is returned (wrapped, via errors.Is-compatible %w) when a
// resize would shrink the server's disk. Hetzner Cloud has no API to do
// this at all — it is a hard provider limitation, not a permission or quota
// issue — so this is never retried internally.
var ErrDiskShrink = errors.New("disk shrink is not supported by Hetzner Cloud")

// Resize changes req.ServerID to req.TargetType. Before calling the
// provider, it fetches the server's current type and req.TargetType's specs
// and refuses (with ErrDiskShrink) if the target has a smaller disk than the
// server currently has.
func Resize(ctx context.Context, client Client, req ResizeRequest) (*Action, error) {
	if req.ServerID == 0 {
		return nil, fmt.Errorf("resize: server ID is required")
	}
	if req.TargetType == "" {
		return nil, fmt.Errorf("resize: --type is required (target server type)")
	}

	srv, err := client.GetServer(ctx, req.ServerID)
	if err != nil {
		return nil, fmt.Errorf("resize: %w", err)
	}

	types, err := client.ListServerTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("resize: list server types: %w", err)
	}

	var current, target *ServerType
	for i := range types {
		switch types[i].Name {
		case srv.ServerType:
			current = &types[i]
		case req.TargetType:
			target = &types[i]
		}
	}
	if target == nil {
		return nil, fmt.Errorf("resize: unknown target server type %q", req.TargetType)
	}
	if current != nil && target.Disk < current.Disk {
		return nil, fmt.Errorf(
			"resize %s (%s, %dGB disk) to %s (%dGB disk): %w — "+
				"the only supported path is: take a snapshot of the current server, "+
				"provision a new server of the smaller type from that snapshot, "+
				"verify it, then destroy the original",
			srv.Name, srv.ServerType, current.Disk, req.TargetType, target.Disk, ErrDiskShrink)
	}

	return client.ChangeServerType(ctx, req.ServerID, req.TargetType, req.UpgradeDisk)
}
