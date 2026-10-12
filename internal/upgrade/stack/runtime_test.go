package stack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

func TestRuntimeImmutableSnapshot(t *testing.T) {
	original := []byte(`{"name":"fixture","services":{"postgres":{"image":"postgres:mutable","build":{"context":"."},"environment":{"PRIVATE":"kept"}}},"volumes":{"data":{}}}`)
	out, err := pinSnapshot(original, map[string]string{"postgres": "sha256:old"})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	service := doc["services"].(map[string]any)["postgres"].(map[string]any)
	if service["image"] != "sha256:old" || service["pull_policy"] != "never" || service["build"] != nil {
		t.Fatalf("snapshot retained mutable image/build: %s", out)
	}
	if doc["volumes"] == nil || service["environment"] == nil {
		t.Fatal("recovery snapshot lost environment or volume configuration")
	}
	if _, err := pinSnapshot(original, map[string]string{}); err == nil {
		t.Fatal("accepted missing immutable image")
	}
	for _, bad := range []string{`{`, `{"services":{}}`, `{"services":{"pg":3}}`} {
		if _, err := pinSnapshot([]byte(bad), nil); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}

func TestRuntimeConstructionAndProductionRestore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	cfg := &config.Config{Env: "staging"}
	r := NewRuntime(dir, cfg, nil)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("construction wrote project")
	}
	if cfg.Backup.Dir != "" {
		t.Fatal("construction mutated config")
	}
	if err := r.Restore(context.Background(), &Journal{}); err == nil {
		t.Fatal("restored staging automatically")
	}
	if err := r.RestartPrevious(context.Background(), &Journal{}); err == nil {
		t.Fatal("restarted staging recovery automatically")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("prod refusal wrote project")
	}
	if err := r.Run(context.Background(), "unknown", &Journal{}); err == nil {
		t.Fatal("accepted unknown step")
	}
}

func TestRuntimeBackupDigestBoundToRun(t *testing.T) {
	dir := t.TempDir()
	r := NewRuntime(dir, &config.Config{Backup: config.BackupConfig{Dir: dir}}, nil)
	path := filepath.Join(dir, "backup.dump")
	if err := os.WriteFile(path, []byte("PGDMP-evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := r.backupDigest("backup.dump")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "latest", "../backup.dump", path} {
		if _, err := r.backupDigest(id); err == nil {
			t.Fatalf("accepted identity %s", id)
		}
	}
	if err := os.Symlink(path, filepath.Join(dir, "link.dump")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.backupDigest("link.dump"); err == nil {
		t.Fatal("accepted symlink backup")
	}
	j := &Journal{StartedAt: time.Now().Add(-time.Second), BackupID: "backup.dump", BackupCreatedAt: time.Now(), BackupSHA256: digest}
	if err := r.VerifyBackup(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	if j.BackupVerifiedAt.IsZero() {
		t.Fatal("missing verification proof")
	}
	if err := os.WriteFile(path, []byte("PGDMP-mutated"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.VerifyBackup(context.Background(), j); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("accepted changed backup: %v", err)
	}
	j.BackupCreatedAt = j.StartedAt.Add(-time.Second)
	if err := r.VerifyBackup(context.Background(), j); err == nil {
		t.Fatal("accepted prior-run backup")
	}
}

func TestRuntimeSnapshotRejectsSymlinkState(t *testing.T) {
	dir := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(dir, ".nself")); err != nil {
		t.Fatal(err)
	}
	r := NewRuntime(dir, &config.Config{}, nil)
	if _, err := r.privateState(); err == nil {
		t.Fatal("accepted symlink recovery directory")
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("wrote through symlink")
	}
}

func TestRuntimePluginPortSelectsDeclaredPort(t *testing.T) {
	port := 8080
	m := &manifestv2.Manifest{Name: "fixture", Service: &manifestv2.Service{Port: &port}}
	got, err := pluginHostPort([]string{"0.0.0.0:9999->9090/tcp", "[::]:1234->8080/tcp"}, m)
	if err != nil || got != 1234 {
		t.Fatalf("port=%d err=%v", got, err)
	}
	for _, ports := range [][]string{nil, {"8080/tcp"}, {"0.0.0.0:no->8080/tcp"}, {"0.0.0.0:0->8080/tcp"}} {
		if _, err := pluginHostPort(ports, m); err == nil {
			t.Fatalf("accepted unmapped port %v", ports)
		}
	}
	if _, err := pluginHostPort(nil, &manifestv2.Manifest{Name: "missing"}); err == nil {
		t.Fatal("accepted missing manifest port")
	}
}

func TestRuntimeRecoverySnapshotTampering(t *testing.T) {
	dir := t.TempDir()
	r := NewRuntime(dir, &config.Config{}, nil)
	state, err := r.privateState()
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"services":{"postgres":{"image":"sha256:old","pull_policy":"never"}}}`)
	p := filepath.Join(state, "update-stack-previous-proof.json")
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	j := &Journal{PreviousCompose: p, PreviousComposeSHA256: hex.EncodeToString(sum[:])}
	if err := r.verifySnapshot(j); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"services":{"postgres":{"image":"tag:new"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.verifySnapshot(j); err == nil {
		t.Fatal("accepted altered recovery bytes")
	}
	mutated, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	sum = sha256.Sum256(mutated)
	j.PreviousComposeSHA256 = hex.EncodeToString(sum[:])
	if err := r.verifySnapshot(j); err == nil {
		t.Fatal("accepted mutable recovery image")
	}
}
