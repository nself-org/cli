//go:build integration

package stack

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
)

// TestStackIntegrationRuntime proves the shipped Runtime adapters create, verify,
// restore and retain a real backup, and restart the immutable previous images.
func TestStackIntegrationRuntime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	b := newIntegrationBackend(t, ctx)
	name, err := b.name()
	if err != nil {
		t.Fatal(err)
	}
	dbInfo, err := docker.InspectContainer(ctx, name+"-db")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.compose.ComposeDown(ctx, b.dir, docker.DownOptions{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(b.dir, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// Pin the database before the initial run so the recovery specification keeps
	// this tmpfs fixture's database container. Production persists data in volumes.
	doc := strings.ReplaceAll(string(data), name+"-db", name+"_postgres")
	doc = strings.Replace(doc, "image: postgres:16", "image: "+dbInfo.Image+"\n    pull_policy: never", 1)
	for _, file := range []string{"compose.yaml", "docker-compose.yml"} {
		if err := os.WriteFile(filepath.Join(b.dir, file), []byte(doc), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.compose.ComposeUp(ctx, b.dir); err != nil {
		t.Fatal(err)
	}
	sql := func(query string) (string, error) {
		out, stderr, err := docker.ExecStdin(ctx, name+"_postgres", []string{"psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "postgres", "-At", "-v", "ON_ERROR_STOP=1"}, strings.NewReader(query))
		if err != nil {
			return "", fmt.Errorf("runtime fixture SQL: %w: %s", err, stderr)
		}
		return strings.TrimSpace(out), nil
	}
	waitIntegration(t, ctx, func() bool { _, err := sql("SELECT 1;"); return err == nil })
	if _, err := sql("CREATE TABLE np_fixture(id integer primary key); INSERT INTO np_fixture SELECT generate_series(1,7); CREATE TABLE np_second(id integer primary key); INSERT INTO np_second SELECT generate_series(1,3);"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROJECT_NAME", name)
	t.Setenv("POSTGRES_DB", "postgres")
	cfg := &config.Config{ProjectName: name, Env: "dev", Postgres: config.PostgresConfig{User: "postgres", DB: "postgres"}, Backup: config.BackupConfig{Dir: filepath.Join(b.dir, "backups")}}
	r := NewRuntime(b.dir, cfg, io.Discard)
	j := newJournal(false)
	if err := r.Backup(ctx, j); err != nil {
		t.Fatal(err)
	}
	if err := r.VerifyBackup(ctx, j); err != nil {
		t.Fatal(err)
	}
	if j.BackupID == "" || j.BackupSHA256 == "" || j.PreviousCompose == "" || j.BackupVerifiedAt.Before(j.StartedAt) {
		t.Fatalf("missing real recovery evidence: %+v", j)
	}
	before, err := r.CountRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 2 {
		t.Fatalf("expected both application tables: %v", before)
	}
	if err := b.writeCompose("nplus1"); err != nil {
		t.Fatal(err)
	}
	// Recreate only the worker to model the N -> N+1 upgrade. The database
	// remains available for backup restoration, as on a normal persistent stack.
	if err := b.compose.ComposeUpNoDeps(ctx, b.dir, "worker"); err != nil {
		t.Fatal(err)
	}
	b.assertImage(t, ctx, "nplus1")
	if _, err := sql("DELETE FROM np_fixture; DELETE FROM np_second;"); err != nil {
		t.Fatal(err)
	}
	changed, err := r.CountRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(before, changed) {
		t.Fatal("fault did not change real row counts")
	}
	if err := r.Restore(ctx, j); err != nil {
		t.Fatal(err)
	}
	if err := r.RestartPrevious(ctx, j); err != nil {
		t.Fatal(err)
	}
	after, err := r.CountRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("real runtime rollback rows before=%v after=%v", before, after)
	}
	b.assertImage(t, ctx, "n")
	if _, err := os.Stat(filepath.Join(cfg.Backup.Dir, j.BackupID)); err != nil {
		t.Fatalf("backup was not retained: %v", err)
	}
	prodCfg := *cfg
	prodCfg.Env = "prod"
	if err := NewRuntime(b.dir, &prodCfg, io.Discard).Restore(ctx, j); err == nil {
		t.Fatal("production runtime accepted automatic restore")
	}
	unchanged, err := r.CountRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, unchanged) {
		t.Fatal("production refusal changed database")
	}
}
