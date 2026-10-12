//go:build integration

package stack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/runner"
)

// TestStackIntegration exercises durable execution with real Linux containers,
// distinct images, a killed executor, and a real PostgreSQL dump/restore.
func TestStackIntegration(t *testing.T) {
	for _, scenario := range []string{"resume", "rollback", "production"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			b := newIntegrationBackend(t, ctx)
			prod := scenario == "production"
			if scenario == "resume" {
				killCtx, kill := context.WithTimeout(ctx, time.Minute)
				defer kill()
				result := make(chan integrationChildResult, 1)
				binary, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				go func() {
					out, err := (runner.LocalExecutor{}).Run(killCtx, "NSELF_STACK_CHILD="+shellQuote(b.dir)+" exec "+shellQuote(binary)+" -test.run='^TestStackIntegrationChild$' -test.v")
					result <- integrationChildResult{output: out, err: err}
				}()
				waitIntegrationChildStart(t, killCtx, filepath.Join(b.dir, "start-entered"), result)
				kill()
				select {
				case r := <-result:
					if r.err == nil {
						t.Fatalf("killed executor succeeded: %s", r.output)
					}
				case <-time.After(10 * time.Second):
					t.Fatal("killed executor was not reaped within ten seconds")
				}
				j, err := loadJournal(b.dir)
				if err != nil {
					t.Fatal(err)
				}
				if j == nil || j.Steps[3].Name != "start" || j.Steps[3].Result != "running" {
					t.Fatalf("kill journal: %+v", j)
				}
				before, err := os.ReadFile(filepath.Join(b.dir, "operations"))
				if err != nil {
					t.Fatal(err)
				}
				j, err = Execute(ctx, Options{ProjectDir: b.dir, Yes: true}, b)
				if err != nil {
					t.Fatal(err)
				}
				after, err := os.ReadFile(filepath.Join(b.dir, "operations"))
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(after[len(before):]), "backup\n") || strings.Contains(string(after[len(before):]), "pull\n") || strings.Contains(string(after[len(before):]), "build\n") {
					t.Fatal("resume replayed completed operations")
				}
				if j.Status != "complete" {
					t.Fatalf("status %q", j.Status)
				}
				b.assertImage(t, ctx, "nplus1")
				return
			}
			b.failMigrate = true
			j, err := Execute(ctx, Options{ProjectDir: b.dir, Prod: prod, Yes: true}, b)
			var coded *errs.CLIError
			if !errors.As(err, &coded) || coded.Code != "E139" {
				t.Fatalf("expected E139: %v", err)
			}
			rows, countErr := b.CountRows(ctx)
			if countErr != nil {
				t.Fatal(countErr)
			}
			if prod {
				if j.Status != "awaiting-operator" || b.restores != 0 || rows["np_fixture"] != 0 {
					t.Fatalf("production restored: %+v rows=%v restores=%d", j, rows, b.restores)
				}
				if !strings.Contains(coded.Fix, RestoreCommand(j.BackupID)) {
					t.Fatalf("missing exact restore command: %v", err)
				}
				b.assertImage(t, ctx, "nplus1")
			} else {
				if b.restores != 1 || !reflect.DeepEqual(rows, j.BeforeRows) || !reflect.DeepEqual(j.AfterRows, j.BeforeRows) {
					t.Fatalf("rollback counts mismatch before=%v after=%v actual=%v", j.BeforeRows, j.AfterRows, rows)
				}
				b.assertImage(t, ctx, "n")
				// M2: the new-image worker was stopped before the restore ran.
				if b.workerUpAtRestore {
					t.Fatal("restore ran while the new-image worker was still running")
				}
			}
		})
	}
}

// TestStackIntegrationChild is a subprocess executor killed after entering start.
func TestStackIntegrationChild(t *testing.T) {
	dir := os.Getenv("NSELF_STACK_CHILD")
	if dir == "" {
		return
	}
	b := &integrationBackend{dir: dir, compose: docker.NewCompose(filepath.Join(dir, "compose.yaml")), blockStart: true}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := Execute(ctx, Options{ProjectDir: dir, Yes: true}, b); err != nil {
		t.Fatalf("child execution: %v; cause: %v", err, errors.Unwrap(err))
	}
}

// shellQuote preserves exact test-owned paths through the supported host runner.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

// integrationChildResult retains early subprocess failures rather than waiting
// for a marker that a failed executor can never create.
type integrationChildResult struct {
	output string
	err    error
}

// waitIntegrationChildStart detects child exit while waiting for start's marker.
// Inputs: bounded child context and result stream. Outputs: reached-start proof.
// Constraints: a child failure reports captured diagnostics immediately.
func waitIntegrationChildStart(t *testing.T, ctx context.Context, marker string, result <-chan integrationChildResult) {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		select {
		case r := <-result:
			t.Fatalf("executor exited before entering start: %v\n%s", r.err, r.output)
		case <-ctx.Done():
			select {
			case r := <-result:
				t.Fatalf("executor never entered start: %v\n%s", r.err, r.output)
			case <-time.After(10 * time.Second):
				t.Fatalf("executor start timed out: %v", ctx.Err())
			}
		case <-ticker.C:
		}
	}
}

// waitIntegration bounds polling of subprocess/container readiness.
func waitIntegration(t *testing.T, ctx context.Context, ready func() bool) {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for !ready() {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}

// integrationBackend runs fixture operations through the Docker funnel.
// The fixture intentionally isolates execution semantics from project generation.
type integrationBackend struct {
	dir                     string
	compose                 *docker.Compose
	blockStart, failMigrate bool
	restores                int
	// workerUpAtRestore is set when Restore ran while the new-image worker was
	// still running (the M2 hazard).
	workerUpAtRestore bool
}

// newIntegrationBackend creates only test-owned Docker resources and refuses skips.
func newIntegrationBackend(t *testing.T, ctx context.Context) *integrationBackend {
	t.Helper()
	dir := t.TempDir()
	b := &integrationBackend{dir: dir, compose: docker.NewCompose(filepath.Join(dir, "compose.yaml"))}
	name := fmt.Sprintf("p31-%d", time.Now().UnixNano())
	if err := os.WriteFile(filepath.Join(dir, "name"), []byte(name), 0600); err != nil {
		t.Fatal(err)
	}
	var builtImages []string
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		if _, err := os.Stat(filepath.Join(dir, "compose.yaml")); err == nil {
			if err := b.compose.ComposeDown(cleanup, dir, docker.DownOptions{RemoveOrphans: true}); err != nil {
				t.Errorf("fixture container cleanup: %v", err)
			}
		} else if !os.IsNotExist(err) {
			t.Errorf("inspect fixture cleanup manifest: %v", err)
		}
		if len(builtImages) > 0 {
			if err := b.compose.Run(cleanup, dir, append([]string{"image", "rm"}, builtImages...)...); err != nil {
				t.Errorf("fixture image cleanup: %v", err)
			}
		}
	})
	for _, version := range []string{"n", "nplus1"} {
		if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM postgres:16\nLABEL p31.version="+version+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := b.compose.Run(ctx, dir, "build", "--network=none", "-t", name+":"+version, dir); err != nil {
			t.Fatalf("Linux Docker and postgres:16 are required: %v", err)
		}
		builtImages = append(builtImages, name+":"+version)
	}
	if err := b.writeCompose("n"); err != nil {
		t.Fatal(err)
	}
	if err := b.compose.ComposeUp(ctx, dir); err != nil {
		t.Fatal(err)
	}
	waitIntegration(t, ctx, func() bool { _, err := b.sql(ctx, "SELECT 1;"); return err == nil })
	if _, err := b.sql(ctx, "CREATE TABLE np_fixture(id integer primary key); INSERT INTO np_fixture SELECT generate_series(1,7); CREATE TABLE np_second(id integer primary key); INSERT INTO np_second SELECT generate_series(1,3);"); err != nil {
		t.Fatal(err)
	}
	return b
}

// name reads the persisted fixture identity shared by parent and child processes.
func (b *integrationBackend) name() (string, error) {
	data, err := os.ReadFile(filepath.Join(b.dir, "name"))
	return string(data), err
}

// writeCompose selects a distinct local image without changing the database service.
func (b *integrationBackend) writeCompose(version string) error {
	name, err := b.name()
	if err != nil {
		return err
	}
	data := fmt.Sprintf("name: %s\nservices:\n  db:\n    image: postgres:16\n    container_name: %s-db\n    environment:\n      POSTGRES_HOST_AUTH_METHOD: trust\n    tmpfs:\n      - /var/lib/postgresql/data\n  worker:\n    image: %s:%s\n    pull_policy: never\n    container_name: %s-worker\n    entrypoint: [sleep, infinity]\n", name, name, name, version, name)
	return os.WriteFile(filepath.Join(b.dir, "compose.yaml"), []byte(data), 0600)
}

// sql runs actual database statements through the Docker stdin funnel.
func (b *integrationBackend) sql(ctx context.Context, sql string) (string, error) {
	name, err := b.name()
	if err != nil {
		return "", err
	}
	out, stderr, err := docker.ExecStdin(ctx, name+"-db", []string{"psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-At"}, strings.NewReader(sql))
	if err != nil {
		return "", fmt.Errorf("fixture SQL: %w: %s", err, stderr)
	}
	return strings.TrimSpace(out), nil
}

// record persists operation calls so resume replay is independently observable.
func (b *integrationBackend) record(step string) error {
	f, err := os.OpenFile(filepath.Join(b.dir, "operations"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(step + "\n")
	closeErr := f.Close()
	return errors.Join(err, closeErr)
}

// Backup retains a real pg_dump and the old image configuration for recovery.
func (b *integrationBackend) Backup(ctx context.Context, j *Journal) error {
	if err := b.record("backup"); err != nil {
		return err
	}
	name, err := b.name()
	if err != nil {
		return err
	}
	out, _, err := docker.ExecCapture(ctx, name+"-db", []string{"pg_dump", "-U", "postgres", "--clean", "--if-exists", "postgres"})
	if err != nil {
		return err
	}
	j.BackupID = "fixture-" + j.RunID
	j.BackupCreatedAt = time.Now().UTC()
	if err := os.WriteFile(filepath.Join(b.dir, j.BackupID), []byte(out), 0600); err != nil {
		return err
	}
	previous, err := os.ReadFile(filepath.Join(b.dir, "compose.yaml"))
	j.PreviousCompose = string(previous)
	return err
}

// VerifyBackup restores the retained dump into an isolated database and counts rows.
func (b *integrationBackend) VerifyBackup(ctx context.Context, j *Journal) error {
	name, err := b.name()
	if err != nil {
		return err
	}
	dump, err := os.ReadFile(filepath.Join(b.dir, j.BackupID))
	if err != nil {
		return err
	}
	exists, err := b.sql(ctx, "SELECT count(*) FROM pg_database WHERE datname = 'backup_verify';")
	if err != nil {
		return err
	}
	if exists == "0" {
		if _, err := b.sql(ctx, "CREATE DATABASE backup_verify;"); err != nil {
			return err
		}
	}
	_, stderr, err := docker.ExecStdin(ctx, name+"-db", []string{"psql", "-U", "postgres", "-d", "backup_verify", "-v", "ON_ERROR_STOP=1"}, strings.NewReader(string(dump)))
	if err != nil {
		return fmt.Errorf("verify restore: %w: %s", err, stderr)
	}
	out, _, err := docker.ExecCapture(ctx, name+"-db", []string{"psql", "-U", "postgres", "-d", "backup_verify", "-Atc", "SELECT (SELECT count(*) FROM np_fixture),(SELECT count(*) FROM np_second)"})
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "7|3" {
		return fmt.Errorf("backup verification count %q", out)
	}
	j.BackupVerifiedAt = time.Now().UTC()
	return nil
}

// Run recreates the worker and injects real data loss at the migration boundary.
func (b *integrationBackend) Run(ctx context.Context, step string, j *Journal) error {
	if err := b.record(step); err != nil {
		return err
	}
	switch step {
	case "pull":
		return nil // both new images were built locally before execution
	case "build":
		return b.writeCompose("nplus1")
	case "start":
		if b.blockStart {
			if err := os.WriteFile(filepath.Join(b.dir, "start-entered"), []byte("start"), 0600); err != nil {
				return err
			}
			<-ctx.Done()
			return ctx.Err()
		}
		return b.compose.ComposeUpNoDeps(ctx, b.dir, "worker")
	case "migrate":
		if b.failMigrate {
			if _, err := b.sql(ctx, "DELETE FROM np_fixture; DELETE FROM np_second;"); err != nil {
				return err
			}
			return errors.New("injected migration failure after real row changes")
		}
		_, err := b.sql(ctx, "CREATE TABLE IF NOT EXISTS np_new_schema(id integer);")
		return err
	case "readiness", "health":
		_, err := b.sql(ctx, "SELECT count(*) FROM np_fixture;")
		return err
	default:
		return fmt.Errorf("unknown fixture step %s", step)
	}
}

// CheckPreviousImages proves the old worker image still exists before recovery.
func (b *integrationBackend) CheckPreviousImages(ctx context.Context, _ *Journal) error {
	name, err := b.name()
	if err != nil {
		return err
	}
	return b.compose.Run(ctx, b.dir, "image", "inspect", name+":n")
}

// StopServices stops the new-image worker; the database service stays up.
func (b *integrationBackend) StopServices(ctx context.Context, _ *Journal) error {
	return b.compose.ComposeStop(ctx, b.dir, "worker")
}

// Restore reapplies the retained real dump without removing backup artifacts.
func (b *integrationBackend) Restore(ctx context.Context, j *Journal) error {
	b.restores++
	if name, err := b.name(); err == nil {
		if info, err := docker.InspectContainer(ctx, name+"-worker"); err == nil && info.State == "running" {
			b.workerUpAtRestore = true
		}
	}
	dump, err := os.ReadFile(filepath.Join(b.dir, j.BackupID))
	if err != nil {
		return err
	}
	_, err = b.sql(ctx, string(dump))
	return err
}

// RestartPrevious recreates only the worker using the recorded old image.
func (b *integrationBackend) RestartPrevious(ctx context.Context, j *Journal) error {
	if err := os.WriteFile(filepath.Join(b.dir, "compose.yaml"), []byte(j.PreviousCompose), 0600); err != nil {
		return err
	}
	return b.compose.ComposeUpNoDeps(ctx, b.dir, "worker")
}

// CountRows proves both application tables retain their pre-update row counts.
func (b *integrationBackend) CountRows(ctx context.Context) (map[string]int64, error) {
	out, err := b.sql(ctx, "SELECT (SELECT count(*) FROM np_fixture),(SELECT count(*) FROM np_second);")
	if err != nil {
		return nil, err
	}
	var n, second int64
	if _, err := fmt.Sscanf(out, "%d|%d", &n, &second); err != nil {
		return nil, err
	}
	return map[string]int64{"np_fixture": n, "np_second": second}, nil
}

// assertImage checks the image of the running container, not just generated YAML.
func (b *integrationBackend) assertImage(t *testing.T, ctx context.Context, version string) {
	t.Helper()
	name, err := b.name()
	if err != nil {
		t.Fatal(err)
	}
	info, err := docker.InspectContainer(ctx, name+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(info.Image, "sha256:") || info.Labels["p31.version"] != version {
		t.Fatalf("running image=%q version=%q, want %s", info.Image, info.Labels["p31.version"], version)
	}
}
