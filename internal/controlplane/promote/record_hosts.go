package promote

// Purpose: Verify recorded host recovery authority before changing any host.
// Inputs: Recorded paths, applied manifest hashes, and current inventory/remotes.
// Outputs: E485 refusals or validated hosts. Constraints: Identity reads are plain
// file reads; all host checks complete before file or database restoration.

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path"
	"strings"

	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	remotesdk "github.com/nself-org/cli/sdk/go/v2/remote"
)

// ManifestHash binds recovery to the exact shipped compose manifest bytes.
func ManifestHash(raw []byte) string { return fmt.Sprintf("%x", sha256.Sum256(raw)) }

// ManifestIdentity authorizes a prior manifest, including a first deploy's absence.
type ManifestIdentity struct {
	Hash   string `json:"hash,omitempty"`
	Absent bool   `json:"absent"`
}

// Purpose: Read prior artifact identity without conflating absent and empty files.
// Inputs: Validated remote path. Outputs: Manifest presence and exact byte hash.
// Constraints: File reads only; command failures never authorize manifest absence.
func ReadManifestIdentity(ctx context.Context, r deploy.Remote, rp string) (ManifestIdentity, error) {
	if err := deploy.ValidateRemotePath(rp); err != nil {
		return ManifestIdentity{}, err
	}
	file := remotesdk.ShellQuote(path.Join(rp, ".nself/compose-files.txt"))
	raw, err := r.Run(ctx, "if [ -f "+file+" ]; then cat "+file+"; elif [ ! -e "+file+" ]; then printf 'nself-promote-manifest-absent\\n'; else exit 1; fi")
	if err != nil {
		return ManifestIdentity{}, err
	}
	if string(raw) == "nself-promote-manifest-absent\n" {
		return ManifestIdentity{Absent: true}, nil
	}
	return ManifestIdentity{Hash: ManifestHash(raw)}, nil
}

func validateRecordPath(rec *Record, host, current string) error {
	rp := rec.RemotePaths[host]
	if rp == "" || rp != current {
		return errs.New("E485", "promotion rollback remote_path differs from the recorded path for "+host+"; nothing was changed")
	}
	if err := deploy.ValidateRemotePath(rp); err != nil {
		return errs.Wrap("E485", "promotion rollback path is invalid; nothing was changed", err)
	}
	return nil
}

func validateRecordHosts(ctx context.Context, rec *Record, hosts []string, remotes map[string]deploy.Remote, current map[string]string) error {
	for _, host := range hosts {
		if remotes[host] == nil {
			return errs.New("E485", "promotion rollback has no remote for "+host+"; nothing was changed")
		}
		if err := validateRecordPath(rec, host, current[host]); err != nil {
			return err
		}
		tar := rec.PreDeployTar[host]
		prefix := path.Join(rec.RemotePaths[host], deploy.BackupDir) + "/pre-deploy-"
		if !strings.HasPrefix(tar, prefix) || strings.Contains(tar, "..") || !strings.HasSuffix(tar, ".tar.gz") {
			return errs.New("E485", "promotion rollback tar is outside the recorded backup path for "+host+"; nothing was changed")
		}
		if rec.AppliedManifestHash[host] == "" {
			return errs.New("E485", "promotion record has no applied artifact identity for "+host+"; nothing was changed")
		}
	}
	for _, host := range hosts {
		if rec.FilesRestored[host] {
			prior, ok := rec.PriorManifestIdentity[host]
			current, err := ReadManifestIdentity(ctx, remotes[host], rec.RemotePaths[host])
			if !ok || err != nil || current != prior {
				return errs.New("E485", "restored target identity differs from the authorized prior identity for "+host+"; restore backup "+rec.BackupID+" by hand; nothing was changed")
			}
			continue
		}
		raw, err := remotes[host].Run(ctx, "cat "+remotesdk.ShellQuote(path.Join(rec.RemotePaths[host], ".nself/compose-files.txt")))
		if err != nil || ManifestHash(raw) != rec.AppliedManifestHash[host] {
			if rec.Status == "in_progress" && rec.ApplyStarted {
				prior, ok := rec.PriorManifestIdentity[host]
				current, readErr := ReadManifestIdentity(ctx, remotes[host], rec.RemotePaths[host])
				if ok && readErr == nil && current == prior {
					continue
				}
			}
			return errs.New("E485", "target changed since the promote "+rec.ID+"; use `deploy rollback` after a new deploy, or restore backup "+rec.BackupID+" by hand; nothing was changed")
		}
	}
	return nil
}
