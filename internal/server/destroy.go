package server

// Purpose: `nself server destroy` — the safety-critical orchestration this
// gap (G-011) exists to close. Tonight's manual procedure took a provider
// snapshot AND a local dump AND verified checksums before deleting a
// server; this encodes the provider-snapshot half of that as a hard gate
// (design requirement 1) and closes the primary-IP footgun found on all 8
// production IPs (design requirement 2).
// Inputs: a Client and a DestroyRequest.
// Outputs: a DestroyResult describing what was actually done (snapshot ID,
// which IPs were retained/released), or an error if any safety
// precondition failed — in which case the server is NEVER deleted.
// Constraints: order matters and is not configurable — snapshot (if
// requested) completes and verifies BEFORE IP handling, and IP handling
// completes BEFORE the DELETE call. A failure at any step aborts before the
// next one runs.

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoBackup is returned when destroy is invoked with neither --snapshot
// nor --force-no-backup. This is the design-requirement-1 gate: a destroy
// call may never reach the provider's DELETE endpoint without one of these
// being explicit.
var ErrNoBackup = errors.New("destroy refused: no verified backup — pass --snapshot to take one first, or --force-no-backup to proceed without one")

// Destroy validates req, optionally takes and verifies a snapshot,
// protects (or releases) the server's primary IPs, and only then deletes
// the server.
func Destroy(ctx context.Context, client Client, req DestroyRequest) (*DestroyResult, error) {
	if req.ServerID == 0 {
		return nil, fmt.Errorf("destroy: server ID is required")
	}
	if !req.TakeSnapshot && !req.ForceNoBackup {
		return nil, ErrNoBackup
	}

	result := &DestroyResult{}

	if req.TakeSnapshot {
		srv, err := client.GetServer(ctx, req.ServerID)
		if err != nil {
			return nil, fmt.Errorf("destroy: %w", err)
		}
		img, err := TakeVerifiedSnapshot(ctx, client, req.ServerID,
			fmt.Sprintf("pre-destroy snapshot of %s", srv.Name), req.SnapshotWait)
		if err != nil {
			return nil, fmt.Errorf("destroy: pre-destroy snapshot failed, server NOT deleted: %w", err)
		}
		result.SnapshotID = img.ID
	}

	retained, released, err := ProtectOrReleaseIPs(ctx, client, req.ServerID, req.ReleaseIP)
	if err != nil {
		return nil, fmt.Errorf("destroy: %w, server NOT deleted", err)
	}
	result.RetainedIPs = retained
	result.ReleasedIPs = released

	if err := client.DeleteServer(ctx, req.ServerID); err != nil {
		return nil, fmt.Errorf("destroy: %w", err)
	}

	return result, nil
}
