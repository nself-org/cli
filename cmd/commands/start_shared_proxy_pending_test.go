package commands

// Tests for D-0931 in start_shared_proxy.go and restart_strategies.go: a
// targeted `restart nginx` reattaches, the owner's start pays a reload a
// failed uninstall left owed, and an uninstall retry pays it too.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/sharedproxy"
)

func stubFlush(t *testing.T, err error) *[]string {
	t.Helper()
	var calls []string
	orig := spFlush
	t.Cleanup(func() { spFlush = orig })
	spFlush = func(_ context.Context, ownerDir string) (bool, error) {
		calls = append(calls, ownerDir)
		return true, err
	}
	return &calls
}

func TestRestartServicesReattachesNginx(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	ctx := context.Background()
	owner, _, _ := proxyLayout(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	comp := &docker.Compose{DockerPath: filepath.Join(bin, "docker")}
	stubFlush(t, nil)

	calls := stubAttach(t, sharedproxy.Outcome{}, nil)
	if err := restartServices(ctx, comp, owner, nil, []string{"hasura"}, false); err != nil || len(*calls) != 0 {
		t.Fatalf("restarting another service must not attach: err %v calls %v", err, *calls)
	}
	if err := restartServices(ctx, comp, owner, nil, []string{"nginx"}, false); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0].owner != owner || strings.Join((*calls)[0].projects, ",") != "b,c" {
		t.Errorf("restart nginx must reattach every registration, got %+v", *calls)
	}
}

func TestOwnerStartPaysAPendingReload(t *testing.T) {
	ctx := context.Background()
	owner, _, _ := proxyLayout(t)
	stubAttach(t, sharedproxy.Outcome{}, nil)
	flushed := stubFlush(t, nil)
	if err := afterStartSharedProxy(ctx, nil, owner); err != nil {
		t.Fatal(err)
	}
	if len(*flushed) != 1 || (*flushed)[0] != owner {
		t.Errorf("the owner's start must flush the owed reload, got %v", *flushed)
	}
	// A failed flush warns; the start still succeeds.
	stubFlush(t, errors.New("boom"))
	out := captureStderr(t, func() { _ = afterStartSharedProxy(ctx, nil, owner) })
	if !strings.Contains(out, "could not be reloaded") {
		t.Errorf("want a warning, got %q", out)
	}
}

func TestUninstallRetryPaysAPendingReload(t *testing.T) {
	ctx := context.Background()
	owner, b, _ := proxyLayout(t)
	pending := filepath.Join(sharedproxy.DirOf(owner), sharedproxy.ReloadPendingFile)

	// b's registration is gone, as after a leave whose reload failed.
	if err := os.Remove(filepath.Join(sharedproxy.DirOf(owner), sharedproxy.FileName(b.Project))); err != nil {
		t.Fatal(err)
	}
	t.Run("nothing owed", func(t *testing.T) {
		flushed := stubFlush(t, nil)
		if err := leaveSharedProxy(ctx, b.ProjectDir); err != nil || len(*flushed) != 0 {
			t.Fatalf("err %v flushed %v", err, *flushed)
		}
	})
	if err := os.WriteFile(pending, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Run("owed and paid", func(t *testing.T) {
		flushed := stubFlush(t, nil)
		if err := leaveSharedProxy(ctx, b.ProjectDir); err != nil || len(*flushed) != 1 || (*flushed)[0] != owner {
			t.Fatalf("err %v flushed %v", err, *flushed)
		}
	})
	t.Run("owed and still failing", func(t *testing.T) {
		stubFlush(t, errors.New("reload boom"))
		var err error
		captureStderr(t, func() { err = leaveSharedProxy(ctx, b.ProjectDir) })
		if err == nil {
			t.Fatal("a retry that cannot reload must report the incomplete removal")
		}
	})
}
