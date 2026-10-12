package promote

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/oplock"
)

type recordRemote struct {
	label   string
	calls   []string
	compose bool
}

func (r *recordRemote) Label() string { return r.label }
func (r *recordRemote) Run(_ context.Context, cmd string) ([]byte, error) {
	r.calls = append(r.calls, cmd)
	if r.compose && len(cmd) < 250 && strings.Contains(cmd, "compose-files.txt") {
		return []byte("/srv/app/docker-compose.yml\n"), nil
	}
	if strings.Contains(cmd, "config --format json") {
		return []byte(`{"services":{}}`), nil
	}
	if strings.Contains(cmd, "echo OK") {
		return []byte("OK\n"), nil
	}
	return nil, nil
}
func (r *recordRemote) Rsync(context.Context, []string, string, string, string) error { return nil }

func persistRollbackFixture(t *testing.T, root string, rec *Record) {
	t.Helper()
	rec.ID, rec.Target, rec.CompletedAt = "fixture", "prodsim", time.Now()
	if rec.Status == "" {
		rec.Status = "ok"
	}
	if rec.Changed == nil {
		rec.Changed = map[string][]string{}
		for host := range rec.PreDeployTar {
			rec.Changed[host] = nil
		}
	}
	if rec.RemotePaths == nil {
		rec.RemotePaths = map[string]string{}
	}
	if rec.AppliedManifestHash == nil {
		rec.AppliedManifestHash = map[string]string{}
	}
	for host := range rec.PreDeployTar {
		if rec.RemotePaths[host] == "" {
			rec.RemotePaths[host] = "/srv/app"
		}
		if rec.AppliedManifestHash[host] == "" {
			rec.AppliedManifestHash[host] = ManifestHash(nil)
		}
	}
	if err := SaveRecord(root, *rec); err != nil {
		t.Fatal(err)
	}
}

func TestPromoteRecordLatest(t *testing.T) {
	root := t.TempDir()
	when := time.Unix(100, 0).UTC()
	rec := Record{ID: "p1", Target: "prodsim", Status: "ok", CompletedAt: when, PreDeployTar: map[string]string{"primary": "/srv/app/.nself/backups/pre-deploy-1.tar.gz"}, Changed: map[string][]string{"primary": {}}}
	if err := SaveRecord(root, rec); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadLatestRecord(root, "other"); err != nil || got != nil {
		t.Fatalf("mismatched target = %#v, %v", got, err)
	}
	if err := deploy.SaveDeployState(root, deploy.StrategyState{Target: "prodsim", CompletedAt: when.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadLatestRecord(root, "prodsim"); err != nil || got != nil {
		t.Fatalf("superseded record = %#v, %v", got, err)
	}
	if err := deploy.SaveDeployState(root, deploy.StrategyState{Target: "other", CompletedAt: when.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadLatestRecord(root, "prodsim"); err != nil || got == nil {
		t.Fatalf("unrelated deploy hid record = %#v, %v", got, err)
	}
}

type overlappingRecordRemote struct {
	mu                        sync.Mutex
	base                      recordRemote
	restores                  atomic.Int32
	entered, overlap, release chan struct{}
}

func (r *overlappingRecordRemote) Label() string { return r.base.label }
func (r *overlappingRecordRemote) Rsync(context.Context, []string, string, string, string) error {
	return nil
}
func (r *overlappingRecordRemote) Run(ctx context.Context, command string) ([]byte, error) {
	if strings.Contains(command, "tar -tzf") {
		return []byte("docker-compose.yml\n"), nil
	}
	if strings.Contains(command, "nself backup restore") {
		if r.restores.Add(1) == 1 {
			close(r.entered)
		} else {
			close(r.overlap)
		}
		<-r.release
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.base.Run(ctx, command)
}

func TestPromoteRecordRollbackConcurrent(t *testing.T) {
	root := t.TempDir()
	r := &overlappingRecordRemote{base: recordRemote{label: "primary"}, entered: make(chan struct{}), overlap: make(chan struct{}), release: make(chan struct{})}
	var released sync.Once
	cleanup := func() { released.Do(func() { close(r.release) }) }
	t.Cleanup(cleanup)
	rec := Record{ID: "fixture", Target: "prodsim", CompletedAt: time.Now(), Status: "ok", MigrateStarted: true,
		BackupID: "backup-1", BackupHost: "primary", PreDeployTar: map[string]string{"primary": "/srv/app/.nself/backups/pre-deploy-1.tar.gz"}, Changed: map[string][]string{"primary": {"docker-compose.yml"}}, RemotePaths: map[string]string{"primary": "/srv/app"}, AppliedManifestHash: map[string]string{"primary": ManifestHash(nil)}}
	if err := SaveRecord(root, rec); err != nil {
		t.Fatal(err)
	}
	remotes, paths := map[string]deploy.Remote{"primary": r}, map[string]string{"primary": "/srv/app"}
	first, second := rec, rec
	one, two := make(chan error, 1), make(chan error, 1)
	go func() { one <- RollbackRecord(context.Background(), root, &first, remotes, paths) }()
	select {
	case <-r.entered:
	case err := <-one:
		t.Fatalf("first rollback before restore: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("first rollback never reached restore")
	}
	go func() { two <- RollbackRecord(context.Background(), root, &second, remotes, paths) }()
	var secondErr error
	secondDone := false
	select {
	case secondErr = <-two:
		secondDone = true
	case <-r.overlap:
	case <-time.After(5 * time.Second):
	}
	cleanup()
	if err := <-one; err != nil {
		t.Fatal(err)
	}
	if !secondDone {
		secondErr = <-two
	}
	if got := r.restores.Load(); got != 1 {
		t.Fatalf("overlapping rollbacks restored backup %d times", got)
	}
	// With the project oplock the overlap is refused (E460). Where the platform has
	// no oplock (Windows) the restore-started marker still fails closed (E489).
	if secondErr != nil && !strings.Contains(secondErr.Error(), "E460") && (oplockSupported(t) || !strings.Contains(secondErr.Error(), "E489")) {
		t.Fatalf("second rollback = %v", secondErr)
	}
	// A stale caller must reload the completed record before deciding to restore.
	if err := RollbackRecord(context.Background(), root, &rec, remotes, paths); err != nil {
		t.Fatal(err)
	}
	if got := r.restores.Load(); got != 1 {
		t.Fatalf("stale rollback restored backup %d times", got)
	}
}

func TestPromoteRecordRollbackOnce(t *testing.T) {
	root := t.TempDir()
	r := &recordRemote{label: "prod-primary"}
	rec := &Record{Target: "prodsim", BackupID: "backup-1", BackupHost: r.label,
		PreDeployTar: map[string]string{"primary": "/srv/app/.nself/backups/pre-deploy-1.tar.gz"},
		Changed:      map[string][]string{"primary": nil}, Absent: map[string][]string{"primary": nil}}
	if err := json.Unmarshal([]byte(`{"migrate_started":true}`), rec); err != nil {
		t.Fatal(err)
	}
	remotes := map[string]deploy.Remote{"primary": r}
	paths := map[string]string{"primary": "/srv/app"}
	persistRollbackFixture(t, root, rec)
	if err := RollbackRecord(context.Background(), root, rec, remotes, paths); err != nil {
		t.Fatal(err)
	}
	if rec.DBRestoredAt == "" || rec.Status != "rolled_back" {
		t.Fatalf("record was not persisted after restore: %#v", rec)
	}
	saved, err := LoadRecord(root)
	if err != nil || saved == nil || !saved.MigrateStarted || saved.DBRestoredAt != rec.DBRestoredAt {
		t.Fatalf("migration/restore evidence was not persisted: %#v, %v", saved, err)
	}
	if err := RollbackRecord(context.Background(), root, rec, remotes, paths); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(r.calls, "\n")
	if got := strings.Count(joined, "nself backup restore 'backup-1' --yes"); got != 1 {
		t.Fatalf("backup restore count = %d\n%s", got, joined)
	}
}

func TestPromoteRecordMigrationEvidence(t *testing.T) {
	for _, status := range []string{"ok", "rolled_back"} {
		t.Run(status, func(t *testing.T) {
			r := &recordRemote{label: "primary"}
			rec := &Record{Status: status, BackupID: "backup-1", BackupHost: r.label,
				PreDeployTar: map[string]string{"primary": "/srv/app/.nself/backups/pre-deploy-1.tar.gz"}}
			root := t.TempDir()
			persistRollbackFixture(t, root, rec)
			if err := RollbackRecord(context.Background(), root, rec, map[string]deploy.Remote{"primary": r}, map[string]string{"primary": "/srv/app"}); err != nil {
				t.Fatal(err)
			}
			for _, call := range r.calls {
				if strings.Contains(call, "backup restore") {
					t.Fatalf("restore before migration: %s", call)
				}
			}
			if status == "rolled_back" && len(r.calls) != 0 {
				t.Fatalf("already rolled back wrote remotely: %v", r.calls)
			}
		})
	}
}

func TestPromoteRecordRestoreOrder(t *testing.T) {
	r := &recordRemote{label: "primary", compose: true}
	rec := &Record{BackupID: "backup-1", BackupHost: r.label, PreDeployTar: map[string]string{"primary": "/srv/app/.nself/backups/pre-deploy-1.tar.gz"}, AppliedManifestHash: map[string]string{"primary": ManifestHash([]byte("/srv/app/docker-compose.yml\n"))}}
	if err := json.Unmarshal([]byte(`{"migrate_started":true}`), rec); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	persistRollbackFixture(t, root, rec)
	if err := RollbackRecord(context.Background(), root, rec, map[string]deploy.Remote{"primary": r}, map[string]string{"primary": "/srv/app"}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(r.calls, "\n")
	// The restored artifact set is read by RollbackRemote before its re-apply.
	manifest := strings.Index(joined, " pull")
	restore := strings.Index(joined, "backup restore")
	health := strings.LastIndex(joined, "nself doctor")
	if manifest < 0 || restore <= manifest || health <= restore {
		t.Fatalf("not files/reapply, DB, health: %s", joined)
	}
}

func TestPromoteRecordValidatesBeforeChange(t *testing.T) {
	good := &recordRemote{label: "one"}
	rec := &Record{PreDeployTar: map[string]string{"one": "/tmp/one.tar.gz", "two": "/tmp/two.tar.gz"}}
	root := t.TempDir()
	persistRollbackFixture(t, root, rec)
	err := RollbackRecord(context.Background(), root, rec,
		map[string]deploy.Remote{"one": good}, map[string]string{"one": "/srv/one", "two": "/srv/two"})
	if err == nil || len(good.calls) != 0 {
		t.Fatalf("validation error = %v, calls = %v", err, good.calls)
	}
}

func TestPromoteRecordRollbackReloadAuthority(t *testing.T) {
	for _, mutation := range []string{"missing", "superseded", "corrupt"} {
		t.Run(mutation, func(t *testing.T) {
			root := t.TempDir()
			r := &recordRemote{label: "primary"}
			rec := &Record{MigrateStarted: true, BackupID: "backup-1", BackupHost: "primary", PreDeployTar: map[string]string{"primary": "/srv/app/.nself/backups/pre-deploy-1.tar.gz"}}
			persistRollbackFixture(t, root, rec)
			switch mutation {
			case "missing":
				if err := os.Remove(filepath.Join(root, RecordFile)); err != nil {
					t.Fatal(err)
				}
			case "superseded":
				if err := deploy.SaveDeployState(root, deploy.StrategyState{Target: rec.Target, CompletedAt: rec.CompletedAt.Add(time.Second)}); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				changed := *rec
				changed.Status = "unknown"
				if err := SaveRecord(root, changed); err != nil {
					t.Fatal(err)
				}
			}
			err := RollbackRecord(context.Background(), root, rec, map[string]deploy.Remote{"primary": r}, map[string]string{"primary": "/srv/app"})
			if err == nil || !strings.Contains(err.Error(), "E485") || len(r.calls) != 0 {
				t.Fatalf("stale authority err=%v remote=%v", err, r.calls)
			}
		})
	}
}

// oplockSupported reports whether this platform takes the project oplock.
func oplockSupported(t *testing.T) bool {
	t.Helper()
	lock, err := oplock.Acquire(context.Background(), t.TempDir(), oplock.Opts{Getenv: func(string) string { return "" }})
	if errors.Is(err, oplock.ErrUnsupported) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	lock.Release()
	return true
}
