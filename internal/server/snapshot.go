package server

// Purpose: take a Hetzner snapshot and block until it is verifiably
// available, for `nself server destroy --snapshot`. Design requirement 1:
// tonight's manual procedure took a snapshot AND waited for it to reach
// status=available before deleting anything — a snapshot request that is
// still "creating" is not a backup yet, so destroy must never proceed past
// a snapshot that hasn't finished.
// Inputs: a Client, the server ID to snapshot, a description, and a wait
// budget (destroy.go passes DestroyRequest.SnapshotWait, defaulting to 10m
// in the command layer if unset).
// Outputs: the Image ID once status=available, or an error if the action
// failed or the wait budget was exceeded.
// Constraints: polls at a fixed interval rather than exponential backoff —
// snapshot creation is a single, already-slow operation (minutes), so
// backoff would only add latency with no benefit here.

import (
	"context"
	"fmt"
	"time"
)

// snapshotPollInterval is a var (not const) so tests can shrink it.
var snapshotPollInterval = 3 * time.Second

// DefaultSnapshotWait is used by the command layer when --snapshot is passed
// without an explicit timeout.
const DefaultSnapshotWait = 10 * time.Minute

// TakeVerifiedSnapshot creates a snapshot of serverID and blocks until
// Hetzner reports the resulting image as status=available. It returns as
// soon as either condition is met: success (image available), the
// underlying action reports status=error, or wait elapses.
func TakeVerifiedSnapshot(ctx context.Context, client Client, serverID int64, description string, wait time.Duration) (*Image, error) {
	if wait <= 0 {
		wait = DefaultSnapshotWait
	}

	img, act, err := client.CreateSnapshot(ctx, serverID, description)
	if err != nil {
		return nil, fmt.Errorf("start snapshot: %w", err)
	}

	return waitForSnapshotAvailable(ctx, client, img.ID, act, time.Now().Add(wait), wait)
}

// waitForSnapshotAvailable polls act and the resulting image until the image
// reaches status=available, act reports status=error, deadline passes, or
// ctx is canceled — whichever comes first.
func waitForSnapshotAvailable(ctx context.Context, client Client, imageID int64, act *Action, deadline time.Time, wait time.Duration) (*Image, error) {
	for {
		if act.Status == "error" {
			msg := "unknown error"
			if act.Error != nil {
				msg = fmt.Sprintf("%s: %s", act.Error.Code, act.Error.Message)
			}
			return nil, fmt.Errorf("snapshot action failed: %s", msg)
		}

		img, err := client.GetImage(ctx, imageID)
		if err != nil {
			return nil, fmt.Errorf("poll snapshot image %d: %w", imageID, err)
		}
		if img.Status == "available" {
			return img, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf(
				"snapshot image %d did not reach status=available within %s (last status: %q) — "+
					"destroy refused; re-run once the snapshot finishes, or check the Hetzner console",
				imageID, wait, img.Status)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(snapshotPollInterval):
		}

		act, err = client.GetAction(ctx, act.ID)
		if err != nil {
			return nil, fmt.Errorf("poll snapshot action %d: %w", act.ID, err)
		}
	}
}
