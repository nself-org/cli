package commands

// P7-WIRE-15: promotion runs migrate between image up and health (D-0698), and its
// rollback restores files, then the database, then brings the stack up with
// StrictHasura and checks health (D-0699). Both are driven through the real
// `deploy promote` cobra entry against a host double whose services are healthy
// only while the release files and the database schema agree.

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/spf13/cobra"
)

type wire15Host struct {
	*v15Remote
	proof      *promoteSafetyRemote
	mu         sync.Mutex
	newFiles   bool // the synced release's files are on the host
	migrated   bool // the database carries the new release's schema
	failDoctor int  // the next N doctor runs fail regardless of state
}

func (h *wire15Host) consistent() bool { return h.newFiles == h.migrated }

func (h *wire15Host) Rsync(ctx context.Context, flags []string, a, b, dest string) error {
	h.mu.Lock()
	h.newFiles = true
	h.mu.Unlock()
	return h.v15Remote.Rsync(ctx, flags, a, b, dest)
}

func (h *wire15Host) Run(ctx context.Context, command string) ([]byte, error) {
	h.mu.Lock()
	switch {
	case strings.Contains(command, "db migrate up"):
		h.migrated = true
	case strings.Contains(command, "nself backup restore"):
		h.migrated = false
	case strings.Contains(command, "tar -xzf"):
		h.newFiles = false
	}
	ok := h.consistent()
	fail := ""
	switch {
	case strings.Contains(command, "hasura metadata apply") && !ok:
		fail = "inconsistent metadata: column missing"
	case strings.Contains(command, "nself doctor") && !ok:
		fail = "schema does not match the release"
	case strings.Contains(command, "nself doctor") && h.failDoctor > 0:
		h.failDoctor--
		fail = "injected doctor failure"
	}
	h.mu.Unlock()
	if fail != "" {
		_, _ = h.v15Remote.Run(ctx, command)
		return []byte(fail), errors.New("exit status 1: " + fail)
	}
	if strings.Contains(command, "ps -a --format json") {
		_, _ = h.v15Remote.Run(ctx, command)
		if !ok {
			return []byte(`[{"ID":"id-web","Name":"demo-web-1","State":"exited","ExitCode":1}]`), nil
		}
		return []byte(`[{"ID":"id-web","Name":"demo-web-1","State":"running"}]`), nil
	}
	if strings.Contains(command, "{{.Id}}") {
		return []byte("sha256:" + strings.Repeat("a", 64) + " web:1\n"), nil
	}
	raw, err := h.v15Remote.Run(ctx, command)
	if err == nil && (strings.Contains(command, "images --format json") || strings.Contains(command, " ps --all --format json") || strings.Contains(command, "image inspect") || strings.Contains(command, "config --format json --no-env-resolution") || strings.Contains(command, "__NSELF_ABSENT__")) {
		return h.proof.Run(ctx, command)
	}
	return raw, err
}

func wire15Setup(t *testing.T) *wire15Host {
	t.Helper()
	compattest.Set(t, true)
	skipPromoteShellDoubles(t)
	locked := compose.LockedImages()[0]
	source, proof := setupPromoteSafety(t, locked.LegacyRef, locked.String(), false)
	root := mustProjectRoot(t)
	wf(t, filepath.Join(root, "docker-compose.yml"), "services:\n  api:\n    image: "+locked.LegacyRef+"\n    build: .\n")
	key := filepath.Join(t.TempDir(), "key")
	wf(t, key, "fixture")
	t.Setenv("TEST_KEY", key)
	target := &wire15Host{v15Remote: &v15Remote{label: "prod-app"}, proof: proof}
	oldClock := deployGateClock
	t.Cleanup(func() { deployGateClock = oldClock })
	deployGateClock = &noClock{now: time.Unix(0, 0)}
	deployRemoteOpen = func(_ context.Context, srv controlplane.Server, _, _ string) (deploy.Remote, error) {
		if srv.Name == "ls-app" {
			return source, nil
		}
		return target, nil
	}
	return target
}

// runDeployArgv mounts the real deploy command under a fresh root and executes argv.
func runDeployArgv(t *testing.T, args ...string) (string, error) {
	t.Helper()
	tree := &cobra.Command{Use: "nself", SilenceUsage: true, SilenceErrors: true}
	if deployCmd.GroupID != "" {
		tree.AddGroup(&cobra.Group{ID: deployCmd.GroupID, Title: "Fixture commands"})
	}
	parent := deployCmd.Parent()
	if parent != nil {
		parent.RemoveCommand(deployCmd)
	}
	tree.AddCommand(deployCmd)
	t.Cleanup(func() {
		tree.RemoveCommand(deployCmd)
		if parent != nil {
			parent.AddCommand(deployCmd)
		}
	})
	var out bytes.Buffer
	tree.SetOut(&out)
	tree.SetErr(&out)
	tree.SetArgs(args)
	err := tree.Execute()
	return out.String(), err
}

func wire15Order(t *testing.T, log []string, events ...string) {
	t.Helper()
	prev := -1
	for _, event := range events {
		at := -1
		for i := prev + 1; i < len(log); i++ {
			if strings.Contains(log[i], event) {
				at = i
				break
			}
		}
		if at < 0 {
			t.Fatalf("event %q not found after index %d:\n%s", event, prev, strings.Join(log, "\n"))
		}
		prev = at
	}
}

// TestWire15MigrateBeforeHealth: a release whose services need the new schema
// promotes, because the migration runs after `up` and before any health check.
func TestWire15MigrateBeforeHealth(t *testing.T) {
	host := wire15Setup(t)
	out, err := runDeployArgv(t, "deploy", "promote", "ls", "prodsim", "--yes")
	log := host.commands()
	if err != nil {
		t.Fatalf("promotion failed: %v\n%s\ncommands:\n%s", err, out, strings.Join(log, "\n"))
	}
	wire15Order(t, log, "up -d --no-deps", "db migrate up", "ps -a --format json", "hasura metadata apply", "nself doctor")
}

// TestWire15RollbackRestoresDBFirst: a promotion whose final health check fails
// rolls back as files, database, then up with StrictHasura plus health, and ends
// E488 (rolled back), never E489.
func TestWire15RollbackRestoresDBFirst(t *testing.T) {
	host := wire15Setup(t)
	host.failDoctor = 1
	out, err := runDeployArgv(t, "deploy", "promote", "ls", "prodsim", "--yes")
	log := host.commands()
	if err == nil || !strings.Contains(err.Error(), "E488") || strings.Contains(err.Error(), "E489") {
		t.Fatalf("err = %v, want E488 without E489\n%s\ncommands:\n%s", err, out, strings.Join(log, "\n"))
	}
	wire15Order(t, log, "db migrate up", "nself doctor", "tar -xzf", "nself backup restore", "up -d --no-deps", "hasura metadata apply", "nself doctor")
}
