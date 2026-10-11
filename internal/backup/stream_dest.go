package backup

// stream_dest.go — destination resolution for backup stream, resume and the heartbeat.
//
// Purpose: parse --to and the heartbeat remote ONCE, through the same destinations grammar create and restore use, so path:// and host:// work and option-shaped values, connection strings and inline credentials are refused before any process starts (D-0257).
// Inputs: the destination URIs from StreamOptions / config and the S3 environment for the backup destination.
// Outputs: a destinations.Streamer (and Remover for the backup destination) per URI; an error that never echoes the URI.
// Constraints: the heartbeat remote is separate from the backup remote and gets no S3 environment. Callers resolve before the dump, dry-run included.

import (
	"fmt"

	"github.com/nself-org/cli/internal/backup/destinations"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// streamTargets are the resolved destinations of one Stream run.
type streamTargets struct {
	put      destinations.Streamer // backup object
	del      destinations.Remover  // removes a failed backup object
	hbRemote string                // heartbeat remote URI; "" means no heartbeat
	hb       destinations.Streamer // nil when hbRemote is ""
}

// streamerFor parses uri and requires the Streamer (and Remover) interfaces.
func streamerFor(uri string, env ...string) (destinations.Streamer, destinations.Remover, error) {
	dest, err := destinationFor(uri, env...)
	if err != nil {
		return nil, nil, err
	}
	st, ok := dest.(destinations.Streamer)
	rm, ok2 := dest.(destinations.Remover)
	if !ok || !ok2 {
		return nil, nil, fmt.Errorf("a %s destination cannot be streamed to", dest.Kind())
	}
	return st, rm, nil
}

// resolveStreamTargets resolves opts.To (already settled) and the heartbeat
// remote. Errors carry ErrBackupRemoteFailed and name the flag, never the value.
func resolveStreamTargets(cfg *config.Config, opts StreamOptions) (*streamTargets, error) {
	t := &streamTargets{hbRemote: opts.HeartbeatTo}
	var err error
	if t.put, t.del, err = streamerFor(opts.To, rcloneEnvFor(cfg)...); err != nil {
		return nil, fmt.Errorf("%w: --to: %v", errs.ErrBackupRemoteFailed, err)
	}
	if t.hbRemote == "" {
		t.hbRemote = cfg.Backup.HeartbeatRemote()
	}
	if t.hbRemote != "" {
		if t.hb, _, err = streamerFor(t.hbRemote); err != nil {
			return nil, fmt.Errorf("%w: heartbeat remote: %v", errs.ErrBackupRemoteFailed, err)
		}
	}
	return t, nil
}
