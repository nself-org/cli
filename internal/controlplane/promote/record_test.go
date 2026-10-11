package promote

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/deploy"
)

type recordRemote struct {
	label string
	calls []string
}

func (r *recordRemote) Label() string { return r.label }
func (r *recordRemote) Run(_ context.Context, cmd string) ([]byte, error) {
	r.calls = append(r.calls, cmd)
	if strings.Contains(cmd, "echo OK") {
		return []byte("OK\n"), nil
	}
	return nil, nil
}
func (r *recordRemote) Rsync(context.Context, []string, string, string, string) error { return nil }

func TestPromoteRecordLatest(t *testing.T) {
	root := t.TempDir()
	when := time.Unix(100, 0).UTC()
	rec := Record{ID: "p1", Target: "prodsim", CompletedAt: when}
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

func TestPromoteRecordRollbackOnce(t *testing.T) {
	root := t.TempDir()
	r := &recordRemote{label: "prod-primary"}
	rec := &Record{Target: "prodsim", BackupID: "backup-1", BackupHost: r.label,
		PreDeployTar: map[string]string{"primary": "/srv/app/.nself/backups/pre-deploy-1.tar.gz"},
		Changed:      map[string][]string{"primary": nil}, Absent: map[string][]string{"primary": nil}}
	remotes := map[string]deploy.Remote{"primary": r}
	paths := map[string]string{"primary": "/srv/app"}
	if err := RollbackRecord(context.Background(), root, rec, remotes, paths); err != nil {
		t.Fatal(err)
	}
	if rec.DBRestoredAt == "" || rec.Status != "rolled_back" {
		t.Fatalf("record was not persisted after restore: %#v", rec)
	}
	if err := RollbackRecord(context.Background(), root, rec, remotes, paths); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(r.calls, "\n")
	if got := strings.Count(joined, "nself backup restore 'backup-1' --yes"); got != 1 {
		t.Fatalf("backup restore count = %d\n%s", got, joined)
	}
}

func TestPromoteRecordValidatesBeforeChange(t *testing.T) {
	good := &recordRemote{label: "one"}
	rec := &Record{PreDeployTar: map[string]string{"one": "/tmp/one.tar.gz", "two": "/tmp/two.tar.gz"}}
	err := RollbackRecord(context.Background(), t.TempDir(), rec,
		map[string]deploy.Remote{"one": good}, map[string]string{"one": "/srv/one", "two": "/srv/two"})
	if err == nil || len(good.calls) != 0 {
		t.Fatalf("validation error = %v, calls = %v", err, good.calls)
	}
}
