package promote

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/cli/internal/deploy"
)

// wire15SchemaRemote models a host whose release files and database schema must
// agree: the restored (old) files with the migrated schema make the strict hasura
// metadata apply fail, the way a dropped column does on a real host.
type wire15SchemaRemote struct {
	mu       sync.Mutex
	calls    []string
	oldFiles bool // tar -xzf put the previous release's files back
	migrated bool // the database carries the new release's schema
}

func (r *wire15SchemaRemote) Label() string { return "primary" }
func (r *wire15SchemaRemote) Rsync(context.Context, []string, string, string, string) error {
	return nil
}
func (r *wire15SchemaRemote) Run(_ context.Context, cmd string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, cmd)
	consistent := r.oldFiles != r.migrated
	switch {
	case strings.Contains(cmd, "tar -xzf"):
		r.oldFiles = true
	case strings.Contains(cmd, "nself backup restore"):
		r.migrated = false
	case strings.Contains(cmd, "hasura metadata apply"):
		if !consistent {
			return []byte("inconsistent metadata: column missing"), errors.New("exit status 1")
		}
	case strings.Contains(cmd, "nself doctor") && !consistent:
		return nil, errors.New("exit status 1: schema does not match the release")
	case len(cmd) < 250 && strings.Contains(cmd, "compose-files.txt"):
		return []byte("/srv/app/docker-compose.yml\n"), nil
	case strings.Contains(cmd, "config --format json"):
		return []byte(`{"services":{}}`), nil
	case strings.Contains(cmd, "echo OK"):
		return []byte("OK\n"), nil
	}
	return nil, nil
}

// TestWire15RollbackRestoresDBFirst: an operator rollback of a migrated
// promotion restores the files, then the database, and only then re-applies the
// restored stack with StrictHasura and checks health. The old order (re-apply
// before the DB restore) fails the strict metadata apply and stops at E489.
func TestWire15RollbackRestoresDBFirst(t *testing.T) {
	r := &wire15SchemaRemote{migrated: true}
	rec := &Record{BackupID: "backup-1", BackupHost: r.Label(), PreDeployTar: map[string]string{"primary": "/srv/app/.nself/backups/pre-deploy-1.tar.gz"}, AppliedManifestHash: map[string]string{"primary": ManifestHash([]byte("/srv/app/docker-compose.yml\n"))}}
	if err := json.Unmarshal([]byte(`{"migrate_started":true}`), rec); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	persistRollbackFixture(t, root, rec)
	err := RollbackRecord(context.Background(), root, "demo", rec, map[string]deploy.Remote{"primary": r}, map[string]string{"primary": "/srv/app"})
	joined := strings.Join(r.calls, "\n")
	if err != nil {
		t.Fatalf("rollback failed (re-apply ran against the migrated schema?): %v\n%s", err, joined)
	}
	files := strings.Index(joined, "tar -xzf")
	restore := strings.Index(joined, "nself backup restore")
	metadata := strings.Index(joined, "hasura metadata apply")
	health := strings.LastIndex(joined, "nself doctor")
	if files < 0 || restore <= files || metadata <= restore || health <= metadata {
		t.Fatalf("order is not files, DB restore, up with strict hasura, health:\n%s", joined)
	}
	if rec.Status != "rolled_back" {
		t.Fatalf("status = %q, want rolled_back", rec.Status)
	}
}
