//go:build !windows

package stack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/docker"
)

// recordingShim logs every docker argv to a file and answers config and ps.
func recordingShim(t *testing.T, dir string) string {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "argv.log")
	bin := filepath.Join(t.TempDir(), "docker")
	script := "#!/bin/sh\necho \"$*\" >> \"$STACK_ARGV_LOG\"\ncase \"$*\" in\n*config*) printf '%s' \"$STACK_TEST_CONFIG\";;\n*ps*) printf '%s' \"$STACK_TEST_PS\";;\n*) exit 0;;\nesac\n"
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("STACK_ARGV_LOG", logPath)
	t.Setenv("STACK_TEST_CONFIG", `{"services":{"postgres":{"image":"postgres:x","healthcheck":{"test":["CMD","true"]}},"hasura":{"image":"h:x","healthcheck":{"test":["CMD","true"]}},"newsvc":{"image":"n:x","healthcheck":{"test":["CMD","true"]}}}}`)
	t.Setenv("STACK_TEST_PS", `[]`)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return logPath
}

// pinnedJournal writes a valid pinned snapshot and returns a journal pointing at it.
func pinnedJournal(t *testing.T, r *Runtime, images map[string]string) *Journal {
	t.Helper()
	state, err := r.privateState()
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for name, image := range images {
		parts = append(parts, fmt.Sprintf(`%q:{"image":%q,"pull_policy":"never"}`, name, image))
	}
	data := []byte(`{"services":{` + strings.Join(parts, ",") + `}}`)
	path := filepath.Join(state, "update-stack-previous-r4.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return &Journal{PreviousCompose: path, PreviousComposeSHA256: hex.EncodeToString(sum[:])}
}

// TestUpdateStackRuntimeCheckPreviousImages (M2 step 1) proves a missing pinned
// image fails before anything runs, and an unreadable check is not "present".
func TestUpdateStackRuntimeCheckPreviousImages(t *testing.T) {
	dir := t.TempDir()
	r := NewRuntime(dir, &config.Config{}, nil)
	j := pinnedJournal(t, r, map[string]string{"postgres": "sha256:old1", "hasura": "sha256:old2"})
	old := runtimeImageDigests
	t.Cleanup(func() { runtimeImageDigests = old })
	var seen []string
	runtimeImageDigests = func(_ context.Context, ref string) ([]string, error) {
		seen = append(seen, ref)
		return nil, nil
	}
	if err := r.CheckPreviousImages(context.Background(), j); err != nil || len(seen) != 2 {
		t.Fatalf("present images rejected: %v %v", err, seen)
	}
	runtimeImageDigests = func(_ context.Context, ref string) ([]string, error) {
		if ref == "sha256:old2" {
			return nil, fmt.Errorf("%s: %w", ref, docker.ErrImageNotFound)
		}
		return nil, nil
	}
	if err := r.CheckPreviousImages(context.Background(), j); !errors.Is(err, docker.ErrImageNotFound) || !strings.Contains(err.Error(), "hasura") {
		t.Fatalf("missing image not named: %v", err)
	}
	runtimeImageDigests = func(context.Context, string) ([]string, error) { return nil, errors.New("daemon down") }
	if err := r.CheckPreviousImages(context.Background(), j); err == nil || !strings.Contains(err.Error(), "cannot check") {
		t.Fatalf("unknown state treated as present: %v", err)
	}
	if err := NewRuntime(dir, &config.Config{Env: "prod"}, nil).CheckPreviousImages(context.Background(), j); err == nil {
		t.Fatal("production check accepted")
	}
}

// TestUpdateStackRuntimeStopServicesKeepsPostgresUp (M2 step 2) proves a plain
// `compose stop` of every service except postgres, with no down, -v or rm.
func TestUpdateStackRuntimeStopServicesKeepsPostgresUp(t *testing.T) {
	dir := t.TempDir()
	logPath := recordingShim(t, dir)
	r := NewRuntime(dir, &config.Config{}, nil)
	if err := r.StopServices(context.Background(), &Journal{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var stop string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, " stop") {
			stop = line
		}
	}
	if stop == "" || !strings.Contains(stop, "hasura") || !strings.Contains(stop, "newsvc") || strings.Contains(stop, "postgres") {
		t.Fatalf("wrong stop set: %q in %s", stop, raw)
	}
	for _, banned := range []string{" down", " -v", " rm ", "volume"} {
		if strings.Contains(string(raw), banned) {
			t.Fatalf("destructive docker call %q: %s", banned, raw)
		}
	}
	if err := NewRuntime(dir, &config.Config{Env: "production"}, nil).StopServices(context.Background(), &Journal{}); err == nil {
		t.Fatal("production stop accepted")
	}
}

// TestUpdateStackRuntimeRestartRemovesOrphans (M2 step 4) proves the snapshot
// start uses `up -d --remove-orphans` from the pinned file and never `-v`.
func TestUpdateStackRuntimeRestartRemovesOrphans(t *testing.T) {
	dir := t.TempDir()
	logPath := recordingShim(t, dir)
	r := NewRuntime(dir, &config.Config{}, nil)
	j := pinnedJournal(t, r, map[string]string{"postgres": "sha256:old"})
	t.Setenv("STACK_TEST_CONFIG", `{"services":{"postgres":{"image":"sha256:old","healthcheck":{"test":["CMD","true"]}}}}`)
	t.Setenv("STACK_TEST_PS", `[{"ID":"pg","Name":"pg","Service":"postgres","State":"running","Health":"healthy"}]`)
	if err := r.RestartPrevious(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(logPath)
	if !strings.Contains(string(raw), "-f "+j.PreviousCompose+" up -d --remove-orphans") || strings.Contains(string(raw), " -v") || strings.Contains(string(raw), " down") {
		t.Fatalf("wrong restart command: %s", raw)
	}
}

// TestUpdateStackRuntimeRestoreIsStrict (S6) proves Runtime.Restore asks the
// backup layer to fail on any pg_restore error line.
func TestUpdateStackRuntimeRestoreIsStrict(t *testing.T) {
	dir := t.TempDir()
	r := NewRuntime(dir, &config.Config{}, nil)
	j := pinnedJournal(t, r, map[string]string{"postgres": "sha256:old"})
	if err := os.MkdirAll(r.cfg.Backup.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	dump := []byte("PGDMP-proof")
	if err := os.WriteFile(filepath.Join(r.cfg.Backup.Dir, "b.dump"), dump, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(dump)
	j.BackupID, j.BackupSHA256 = "b.dump", hex.EncodeToString(sum[:])
	j.StartedAt, j.BackupCreatedAt = timeAgo(), time.Now()
	oldVerify, oldRestore := runtimeBackupVerify, runtimeBackupRestore
	t.Cleanup(func() { runtimeBackupVerify, runtimeBackupRestore = oldVerify, oldRestore })
	runtimeBackupVerify = func(context.Context, *config.Config, backup.VerifyOptions) (*backup.VerifyResult, error) {
		return &backup.VerifyResult{Verified: true}, nil
	}
	var got backup.RestoreOptions
	runtimeBackupRestore = func(_ context.Context, _ *config.Config, o backup.RestoreOptions) error {
		got = o
		return nil
	}
	if err := r.Restore(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	if !got.Strict || got.BackupID != "b.dump" || len(got.Only) != 1 || got.Only[0] != "pg" {
		t.Fatalf("restore is not strict: %+v", got)
	}
}

// TestUpdateStackRuntimeBackupTagHoldsFromPrune (S2) proves the run backup tag
// carries the keep- prefix that internal/backup prune never deletes.
func TestUpdateStackRuntimeBackupTagHoldsFromPrune(t *testing.T) {
	dir := t.TempDir()
	r := NewRuntime(dir, &config.Config{}, nil)
	old := runtimeBackupCreate
	t.Cleanup(func() { runtimeBackupCreate = old })
	var tag string
	runtimeBackupCreate = func(_ context.Context, cfg *config.Config, o backup.CreateOptions) error {
		tag = o.Tag
		if err := os.MkdirAll(cfg.Backup.Dir, 0700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(cfg.Backup.Dir, "p_full_1_"+o.Tag+".dump"), []byte("PGDMP-x"), 0600)
	}
	j := &Journal{RunID: "r4", StartedAt: timeAgo()}
	if err := r.createRunBackup(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tag, "keep-stack-r4-") || !strings.Contains(j.BackupID, "_keep-stack-r4-") {
		t.Fatalf("run backup is not held from prune: tag=%q id=%q", tag, j.BackupID)
	}
}

// TestUpdateStackRuntimeTrackTables (S4) proves tracking covers only active
// boot-migration plugins with tables, and a Hasura failure names the plugins.
func TestUpdateStackRuntimeTrackTables(t *testing.T) {
	dir := t.TempDir()
	recordingShim(t, dir)
	plugins := filepath.Join(dir, "plugins")
	for name, apply := range map[string]string{"fixture-full": "boot", "lazy": "none"} {
		p := filepath.Join(plugins, name)
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join("..", "..", "plugin", "manifestv2", "testdata", "v2", "full.json"))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		m["name"], m["schema"] = name, "np_"+strings.ReplaceAll(name, "-", "_")
		if apply != "boot" {
			delete(m, "migrations") // declares tables but does not migrate at boot
		}
		if data, err = json.Marshal(m); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "plugin.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("STACK_TEST_CONFIG", `{"services":{"plugin_fixture-full":{"image":"x"},"plugin_lazy":{"image":"x"}}}`)
	r := NewRuntime(dir, &config.Config{PluginSystem: config.PluginSystemConfig{Dir: plugins}}, nil)
	old := runtimeTrackTables
	t.Cleanup(func() { runtimeTrackTables = old })
	var calls [][]database.PluginTables
	runtimeTrackTables = func(_ context.Context, _ *config.Config, p []database.PluginTables) (database.TrackResult, error) {
		calls = append(calls, p)
		return database.TrackResult{}, nil
	}
	if err := r.trackTables(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || len(calls[0]) != 1 || calls[0][0].Plugin != "fixture-full" {
		t.Fatalf("wrong plugins tracked: %+v", calls)
	}
	runtimeTrackTables = func(context.Context, *config.Config, []database.PluginTables) (database.TrackResult, error) {
		return database.TrackResult{}, errors.New("hasura rejected")
	}
	if err := r.trackTables(context.Background()); err == nil || !strings.Contains(err.Error(), "fixture-full") || !strings.Contains(err.Error(), "hasura rejected") {
		t.Fatalf("tracking failure not reported with the plugin: %v", err)
	}
}

// timeAgo is a run start safely before any artifact the test creates.
func timeAgo() time.Time { return time.Now().Add(-time.Minute) }
