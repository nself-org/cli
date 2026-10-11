package promote

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/deploy"
)

func TestPromoteRecordRecoverySafeguards(t *testing.T) {
	for _, refusal := range []string{"path", "tar", "identity"} {
		t.Run(refusal, func(t *testing.T) {
			root := t.TempDir()
			r := &recordRemote{label: "primary"}
			rec := &Record{BackupID: "backup-1", BackupHost: "primary", MigrateStarted: true,
				PreDeployTar: map[string]string{"primary": "/srv/app/.nself/backups/pre-deploy-1.tar.gz"}}
			hash := fmt.Sprintf("%x", sha256.Sum256(nil))
			path := "/srv/app"
			if refusal == "path" {
				path = "/srv/other"
			}
			if refusal == "tar" {
				rec.PreDeployTar["primary"] = "/tmp/pre-deploy-1.tar.gz"
			}
			if refusal == "identity" {
				hash = "changed"
			}
			raw := fmt.Sprintf(`{"remote_paths":{"primary":"/srv/app"},"applied_manifest_hash":{"primary":%q}}`, hash)
			if err := json.Unmarshal([]byte(raw), rec); err != nil {
				t.Fatal(err)
			}
			persistRollbackFixture(t, root, rec)
			err := RollbackRecord(context.Background(), root, rec, map[string]deploy.Remote{"primary": r}, map[string]string{"primary": path})
			if err == nil || !strings.Contains(err.Error(), "E485") {
				t.Fatalf("%s refusal: %v calls=%v", refusal, err, r.calls)
			}
			for _, call := range r.calls {
				if !strings.HasPrefix(call, "cat ") {
					t.Fatalf("changed host during refusal: %s", call)
				}
			}
			if refusal == "identity" && !strings.Contains(err.Error(), "target changed since the promote fixture") {
				t.Fatalf("identity recovery hint: %v", err)
			}
		})
	}
}

func TestPromoteRecordInProgressRecovery(t *testing.T) {
	for _, migrated := range []bool{false, true} {
		t.Run(fmt.Sprint(migrated), func(t *testing.T) {
			root := t.TempDir()
			r := &recordRemote{label: "primary"}
			rec := &Record{ID: "interrupted", Target: "prodsim", Status: "in_progress", BackupID: "backup-1", BackupHost: "primary", MigrateStarted: migrated, RemotePaths: map[string]string{"primary": "/srv/app"}}
			if err := SaveRecord(root, *rec); err != nil {
				t.Fatal(err)
			}
			got, err := LoadLatestRecord(root, rec.Target)
			if err != nil || got == nil {
				t.Fatalf("provisional record unavailable: %#v %v", got, err)
			}
			if err := RollbackRecord(context.Background(), root, rec, map[string]deploy.Remote{"primary": r}, map[string]string{"primary": "/srv/app"}); err != nil {
				t.Fatal(err)
			}
			restored := strings.Contains(strings.Join(r.calls, "\n"), "backup restore")
			if restored != migrated {
				t.Fatalf("DB restore=%v migrate started=%v calls=%v", restored, migrated, r.calls)
			}
		})
	}
}

func TestPromoteRecordChecksEveryHostBeforeRestore(t *testing.T) {
	root := t.TempDir()
	one, two := &recordRemote{label: "one"}, &recordRemote{label: "two"}
	rec := &Record{PreDeployTar: map[string]string{
		"one": "/srv/app/.nself/backups/pre-deploy-1.tar.gz", "two": "/srv/app/.nself/backups/pre-deploy-2.tar.gz"},
		AppliedManifestHash: map[string]string{"one": ManifestHash(nil), "two": ManifestHash([]byte("changed"))}}
	persistRollbackFixture(t, root, rec)
	err := RollbackRecord(context.Background(), root, rec,
		map[string]deploy.Remote{"one": one, "two": two}, map[string]string{"one": "/srv/app", "two": "/srv/app"})
	if err == nil || !strings.Contains(err.Error(), "E485") {
		t.Fatalf("identity mismatch: %v", err)
	}
	for _, r := range []*recordRemote{one, two} {
		for _, call := range r.calls {
			if !strings.HasPrefix(call, "cat ") {
				t.Fatalf("host changed before all identities verified: %s", call)
			}
		}
	}
}

func TestPromoteRecordInProgressAppliedRecovery(t *testing.T) {
	for _, migrated := range []bool{false, true} {
		t.Run(fmt.Sprint(migrated), func(t *testing.T) {
			root := t.TempDir()
			r := &recordRemote{label: "primary"}
			rec := &Record{Status: "in_progress", BackupID: "backup-1", BackupHost: "primary", MigrateStarted: migrated,
				PreDeployTar: map[string]string{"primary": "/srv/app/.nself/backups/pre-deploy-1.tar.gz"}}
			persistRollbackFixture(t, root, rec)
			if err := RollbackRecord(context.Background(), root, rec, map[string]deploy.Remote{"primary": r}, map[string]string{"primary": "/srv/app"}); err != nil {
				t.Fatal(err)
			}
			if restored := strings.Contains(strings.Join(r.calls, "\n"), "backup restore"); restored != migrated {
				t.Fatalf("restore=%v migrated=%v", restored, migrated)
			}
			if rec.Status != "rolled_back" {
				t.Fatalf("recovery status=%s", rec.Status)
			}
		})
	}
}
