package promote

// Purpose: Persist the local authority for a promotion and roll it back.
// Inputs: Promotion outcome plus caller-opened target remotes and remote paths.
// Outputs: Atomic promote.json state and file/database restoration from that state.
// Constraints: Host data never widens the saved changed/absent lists; the verified
// database backup is restored once, on its recorded host, and persisted immediately.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	remotesdk "github.com/nself-org/cli/sdk/go/v2/remote"
)

// RecordFile is the local promotion rollback authority.
const RecordFile = ".nself/state/promote.json"

// Record describes one completed promotion and everything its rollback may touch.
type Record struct {
	SchemaVersion  int                 `json:"schema_version"`
	ID             string              `json:"id"`
	Source         string              `json:"source"`
	Target         string              `json:"target"`
	StartedAt      time.Time           `json:"started_at"`
	CompletedAt    time.Time           `json:"completed_at"`
	Status         string              `json:"status"`
	SnapshotTag    string              `json:"snapshot_tag,omitempty"`
	BackupID       string              `json:"backup_id,omitempty"`
	BackupHost     string              `json:"backup_host,omitempty"`
	PreDeployTar   map[string]string   `json:"pre_deploy_tar,omitempty"`
	Changed        map[string][]string `json:"changed,omitempty"`
	Absent         map[string][]string `json:"absent,omitempty"`
	Digests        []DigestRow         `json:"digests,omitempty"`
	SavedManifest  SavedManifest       `json:"saved_manifest"`
	DBRestoredAt   string              `json:"db_restored_at,omitempty"`
	MigrateStarted bool                `json:"migrate_started"`
	DeployIdentity *DeployIdentity     `json:"deploy_identity,omitempty"`
}

// LoadRecord reads promote.json; a missing record returns nil, nil.
func LoadRecord(projectRoot string) (*Record, error) {
	raw, err := os.ReadFile(filepath.Join(projectRoot, RecordFile)) //nolint:gosec // project-local state path
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading promote state: %w", err)
	}
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, fmt.Errorf("parsing promote state: %w", err)
	}
	return &rec, nil
}

// SaveRecord atomically writes promote.json with private permissions.
func SaveRecord(projectRoot string, rec Record) error {
	if err := captureDeployIdentity(projectRoot, &rec); err != nil {
		return err
	}
	rec.SchemaVersion = 1
	file := filepath.Join(projectRoot, RecordFile)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return fmt.Errorf("creating promote state directory: %w", err)
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling promote state: %w", err)
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("writing promote state: %w", err)
	}
	if err := os.Rename(tmp, file); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("committing promote state: %w", err)
	}
	return nil
}

// LoadLatestRecord returns a target record only when no different deploy supersedes it.
func LoadLatestRecord(projectRoot, target string) (*Record, error) {
	rec, err := LoadRecord(projectRoot)
	if err != nil || rec == nil {
		if err != nil {
			return nil, errs.Wrap("E485", "promotion record "+RecordFile+" is unusable; move it aside, then deploy rollback uses deploy.json; nothing was changed", err)
		}
		return rec, nil
	}
	if rec.Target != target {
		return nil, nil
	}
	state, err := deploy.LoadDeployState(projectRoot)
	if err != nil {
		return nil, err
	}
	if state.Target == target && !recordMatchesDeploy(rec, state) {
		return nil, nil
	}
	if rec.SchemaVersion != 1 || rec.ID == "" || rec.CompletedAt.IsZero() || (rec.Status != "ok" && rec.Status != "rollback_failed" && rec.Status != "rolled_back") {
		return nil, errs.New("E485", "promotion record is unusable; nothing was changed")
	}
	if rec.Status != "rolled_back" && !recordHasRestoreLists(rec) {
		return nil, errs.New("E485", "promotion record has missing tar or changed lists; nothing was changed")
	}
	return rec, nil
}

// Purpose: Require local authority for every host before opening any remote.
// Inputs: Current record. Outputs: Whether tar and changed lists are present.
// Constraints: An empty per-host change list remains a valid recorded list.
func recordHasRestoreLists(rec *Record) bool {
	if len(rec.PreDeployTar) == 0 {
		return false
	}
	for host, tar := range rec.PreDeployTar {
		if _, ok := rec.Changed[host]; tar == "" || !ok {
			return false
		}
	}
	return true
}

// RollbackRecord re-applies every saved stack before restoring the migrated DB.
func RollbackRecord(ctx context.Context, projectRoot string, rec *Record, remotes map[string]deploy.Remote, remotePaths map[string]string) error {
	if rec == nil {
		return fmt.Errorf("no promotion record")
	}
	ctx, release, err := AcquireRecordLock(ctx, projectRoot)
	if err != nil {
		return err
	}
	defer release()
	// Reload under the same project lock as deploy apply: a caller's stale record
	// must never authorize another database restore after a completed rollback.
	latest, err := LoadLatestRecord(projectRoot, rec.Target)
	if err != nil {
		return errs.Wrap("E485", "promotion record is unusable; nothing was changed", err)
	}
	if latest == nil || latest.ID != rec.ID {
		return errs.New("E485", "promotion record changed or is no longer applicable; nothing was changed")
	}
	*rec = *latest
	if rec.Status == "rolled_back" {
		return nil
	}
	keys := make([]string, 0, len(rec.PreDeployTar))
	for key := range rec.PreDeployTar {
		keys = append(keys, key)
	}
	backupKey := ""
	for _, key := range keys {
		r, ok := remotes[key]
		if !ok || r == nil {
			return fmt.Errorf("promotion rollback has no remote for %s", key)
		}
		if rec.PreDeployTar[key] == "" {
			return fmt.Errorf("promotion rollback has no pre-deploy tar for %s", key)
		}
		if err := deploy.ValidateRemotePath(remotePaths[key]); err != nil {
			return fmt.Errorf("promotion rollback path for %s: %w", key, err)
		}
		if recordHostMatches(r, rec.BackupHost) {
			backupKey = key
		}
	}
	restoreDB := rec.MigrateStarted && rec.DBRestoredAt == ""
	if restoreDB && (rec.BackupID == "" || backupKey == "") {
		return fmt.Errorf("promotion backup host %q is absent from the local record", rec.BackupHost)
	}
	sort.Strings(keys)
	recoveryCtx := context.WithoutCancel(ctx)
	for _, key := range keys {
		r := remotes[key]
		rp := remotePaths[key]
		err := deploy.RollbackRemote(recoveryCtx, deploy.RollbackOptions{
			Remote: r, RemotePath: rp, Tar: rec.PreDeployTar[key], BackupID: "",
			Changed: rec.Changed[key], Absent: rec.Absent[key], StrictHasura: true,
		})
		if err != nil {
			return fmt.Errorf("rolling back promotion on %s: %w", key, err)
		}
	}
	if restoreDB {
		_, err := remotes[backupKey].Run(recoveryCtx, "cd "+remotesdk.ShellQuote(remotePaths[backupKey])+" && nself backup restore "+remotesdk.ShellQuote(rec.BackupID)+" --yes")
		if err != nil {
			return fmt.Errorf("restoring promotion backup %s: %w", rec.BackupID, err)
		}
		rec.DBRestoredAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := SaveRecord(projectRoot, *rec); err != nil {
			return fmt.Errorf("database restored but promotion record could not be saved: %w", err)
		}
	}
	for _, key := range keys {
		if err := HealthStep(remotes[key], remotePaths[key]).Run(recoveryCtx, &State{}); err != nil {
			return fmt.Errorf("checking target health after promotion rollback: %w", err)
		}
	}
	rec.Status = "rolled_back"
	rec.CompletedAt = time.Now().UTC()
	return SaveRecord(projectRoot, *rec)
}

func recordHostMatches(r deploy.Remote, backupHost string) bool {
	return r != nil && r.Label() == backupHost
}
