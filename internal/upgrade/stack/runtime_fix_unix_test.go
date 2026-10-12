//go:build !windows

package stack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

// runtimeDockerShim exercises the Docker funnel without a daemon or Docker binary.
func runtimeDockerShim(t *testing.T, dir string) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "docker")
	script := `#!/bin/sh
case "$*" in
*config*)
 output=''
 while [ "$#" -gt 0 ]; do
  if [ "$1" = '--output' ]; then shift; output="$1"; fi
  shift
 done
 if [ -n "$output" ]; then printf '%s' "$STACK_TEST_CONFIG" > "$output"; else printf '%s' "$STACK_TEST_CONFIG"; fi;;
*inspect*) printf '%s' "$STACK_TEST_INSPECT";;
*ps*) printf '%s' "$STACK_TEST_PS";;
*pull*|*up*|*stop*) exit 0;;
*) exit 1;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("STACK_TEST_CONFIG", `{"services":{"postgres":{"image":"postgres:mutable","healthcheck":{"test":["CMD","true"]}}}}`)
	t.Setenv("STACK_TEST_PS", `[{"ID":"pg","Name":"pg","Service":"postgres","State":"running","Health":"healthy"}]`)
	t.Setenv("STACK_TEST_INSPECT", `{"Id":"pg","Image":"sha256:old","Config":{"Labels":{"com.docker.compose.service":"postgres"}}}`)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateStackRuntimeCountsAfterDump(t *testing.T) {
	dir := t.TempDir()
	runtimeDockerShim(t, dir)
	r := NewRuntime(dir, &config.Config{}, nil)
	oldCreate, oldCount := runtimeBackupCreate, runtimeBackupCountRows
	t.Cleanup(func() { runtimeBackupCreate, runtimeBackupCountRows = oldCreate, oldCount })
	dumped := false
	runtimeBackupCreate = func(_ context.Context, cfg *config.Config, opt backup.CreateOptions) error {
		if err := os.MkdirAll(cfg.Backup.Dir, 0700); err != nil {
			return err
		}
		err := os.WriteFile(filepath.Join(cfg.Backup.Dir, "fixture_"+opt.Tag+".dump"), []byte("PGDMP-proof"), 0600)
		dumped = err == nil
		return err
	}
	runtimeBackupCountRows = func(context.Context, *backup.Container) (map[string]int64, error) {
		if !dumped {
			t.Fatal("counted rows before dump creation")
		}
		return map[string]int64{"public.items": 7}, nil
	}
	j := &Journal{RunID: "row-order", StartedAt: time.Now().Add(-time.Second)}
	if err := r.Backup(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	if j.BeforeRows["public.items"] != 7 || j.BackupSHA256 == "" || j.PreviousComposeSHA256 == "" {
		t.Fatalf("missing backup evidence: %+v", j)
	}
	if err := r.VerifyBackup(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	if err := r.Backup(context.Background(), j); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateStackRuntimePreviousImagesRejectsNonImage(t *testing.T) {
	dir := t.TempDir()
	runtimeDockerShim(t, dir)
	r := NewRuntime(dir, &config.Config{}, nil)
	c, err := r.compose()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, inspect, ps, want string }{
		{"non-image", `{"Image":"postgres:tag","Config":{"Labels":{"com.docker.compose.service":"postgres"}}}`, "", "not immutable"},
		{"no service", `{"Image":"sha256:old","Config":{"Labels":{}}}`, "", "lacks service"},
		{"missing container", "", `[]`, "no container evidence"},
		{"bad inspect", `{`, "", "inspect previous image"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.inspect != "" {
				t.Setenv("STACK_TEST_INSPECT", tc.inspect)
			}
			if tc.ps != "" {
				t.Setenv("STACK_TEST_PS", tc.ps)
			}
			if _, err := r.previousImages(context.Background(), c); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want=%s", err, tc.want)
			}
		})
	}
}

func TestUpdateStackRuntimeHealthPaths(t *testing.T) {
	dir := t.TempDir()
	runtimeDockerShim(t, dir)
	r := NewRuntime(dir, &config.Config{}, nil)
	if err := r.Run(context.Background(), "health", &Journal{}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, spec, state, want string }{
		{"crashed", "", `[{"ID":"pg","State":"exited","ExitCode":2}]`, "service postgres health"},
		{"no services", `{"services":{}}`, "", "no declared health services"},
		{"invalid spec", `{`, "", "read health specifications"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.spec != "" {
				t.Setenv("STACK_TEST_CONFIG", tc.spec)
			}
			if tc.state != "" {
				t.Setenv("STACK_TEST_PS", tc.state)
			}
			if err := r.health(context.Background()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want=%s", err, tc.want)
			}
		})
	}
	port := 8080
	m := &manifestv2.Manifest{Name: "fixture", Service: &manifestv2.Service{Port: &port}, Migrations: &manifestv2.Migrations{Apply: "boot"}}
	for _, containers := range [][]docker.ContainerInfo{nil, {{Service: "fixture", Ports: []string{"8080/tcp"}}}} {
		if err := waitPluginReadiness(context.Background(), m, containers, time.Second); err == nil || !strings.Contains(err.Error(), "published") {
			t.Fatalf("missing port accepted: %v", err)
		}
	}
}

func TestUpdateStackRuntimeBackupPostDumpCountFailure(t *testing.T) {
	dir := t.TempDir()
	runtimeDockerShim(t, dir)
	r := NewRuntime(dir, &config.Config{}, nil)
	oldCreate, oldCount := runtimeBackupCreate, runtimeBackupCountRows
	t.Cleanup(func() { runtimeBackupCreate, runtimeBackupCountRows = oldCreate, oldCount })
	want := errors.New("count failed")
	runtimeBackupCreate = func(_ context.Context, cfg *config.Config, opt backup.CreateOptions) error {
		if err := os.MkdirAll(cfg.Backup.Dir, 0700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(cfg.Backup.Dir, "fixture_"+opt.Tag+".dump"), []byte("PGDMP-proof"), 0600)
	}
	runtimeBackupCountRows = func(context.Context, *backup.Container) (map[string]int64, error) { return nil, want }
	j := &Journal{RunID: "count-failure", StartedAt: time.Now().Add(-time.Second)}
	if err := r.Backup(context.Background(), j); !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
	if j.BackupID == "" || j.BackupSHA256 == "" || j.BeforeRows != nil {
		t.Fatal("count failure lost backup identity or recorded false baseline")
	}
}
