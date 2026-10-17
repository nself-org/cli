package commands

// Tests for start_shared_proxy.go. The sharedproxy calls are stubbed through
// the package variables; the registrations and the directory layout are real.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/sharedproxy"
)

// proxyLayout writes an owner `a` with fronted projects `b` and `c` nested
// under it and returns the owner dir and the registrations.
func proxyLayout(t *testing.T) (owner string, b, c sharedproxy.Registration) {
	t.Helper()
	owner = filepath.Join(t.TempDir(), "a")
	dir := sharedproxy.DirOf(owner)
	if err := sharedproxy.Write(dir, sharedproxy.OwnerFile, sharedproxy.New("a", owner)); err != nil {
		t.Fatal(err)
	}
	b = sharedproxy.New("b", filepath.Join(owner, "b"))
	c = sharedproxy.New("c", filepath.Join(owner, "c"))
	for _, r := range []sharedproxy.Registration{b, c} {
		if err := os.MkdirAll(r.ProjectDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := sharedproxy.Write(dir, sharedproxy.FileName(r.Project), r); err != nil {
			t.Fatal(err)
		}
	}
	return owner, b, c
}

type attachCall struct {
	owner, self string
	projects    []string
}

func stubAttach(t *testing.T, out sharedproxy.Outcome, err error) *[]attachCall {
	t.Helper()
	var calls []attachCall
	orig := spAttach
	t.Cleanup(func() { spAttach = orig })
	spAttach = func(_ context.Context, owner, self string, regs []sharedproxy.Registration) (sharedproxy.Outcome, error) {
		call := attachCall{owner: owner, self: self}
		for _, r := range regs {
			call.projects = append(call.projects, r.Project)
		}
		calls = append(calls, call)
		return out, err
	}
	return &calls
}

func fronted(by string) *config.Config {
	cfg := &config.Config{}
	cfg.Nginx.FrontedBy = by
	return cfg
}

func TestAfterStartSharedProxy(t *testing.T) {
	ctx := context.Background()
	owner, b, _ := proxyLayout(t)
	e506 := errs.New("E506", "nginx -t rejected")

	t.Run("a project outside a shared proxy is untouched", func(t *testing.T) {
		calls := stubAttach(t, sharedproxy.Outcome{}, nil)
		if err := afterStartSharedProxy(ctx, fronted(""), t.TempDir()); err != nil || len(*calls) != 0 {
			t.Fatalf("err %v calls %v", err, *calls)
		}
	})

	t.Run("the owner attaches every registration and never fails", func(t *testing.T) {
		calls := stubAttach(t, sharedproxy.Outcome{Missing: []string{"c"}}, e506)
		if err := afterStartSharedProxy(ctx, &config.Config{}, owner); err != nil {
			t.Fatalf("an owner start must not fail on a fronted project: %v", err)
		}
		want := attachCall{owner: owner, self: "", projects: []string{"b", "c"}}
		if len(*calls) != 1 || (*calls)[0].owner != want.owner || (*calls)[0].self != "" || strings.Join((*calls)[0].projects, ",") != "b,c" {
			t.Fatalf("calls = %+v, want one %+v", *calls, want)
		}
	})

	t.Run("a fronted project attaches itself", func(t *testing.T) {
		calls := stubAttach(t, sharedproxy.Outcome{Attached: []string{"b"}, Reloaded: true}, nil)
		if err := afterStartSharedProxy(ctx, fronted("a"), b.ProjectDir); err != nil {
			t.Fatal(err)
		}
		if len(*calls) != 1 || (*calls)[0].owner != owner || (*calls)[0].self != "b" || strings.Join((*calls)[0].projects, ",") != "b" {
			t.Fatalf("calls = %+v", *calls)
		}
	})

	t.Run("without NGINX_FRONTED_BY nothing is attached", func(t *testing.T) {
		calls := stubAttach(t, sharedproxy.Outcome{}, nil)
		if err := afterStartSharedProxy(ctx, fronted(""), b.ProjectDir); err != nil || len(*calls) != 0 {
			t.Fatalf("err %v calls %v", err, *calls)
		}
	})

	t.Run("a stopped owner or a missing network is a warning", func(t *testing.T) {
		stubAttach(t, sharedproxy.Outcome{}, sharedproxy.ErrOwnerNotRunning)
		if err := afterStartSharedProxy(ctx, fronted("a"), b.ProjectDir); err != nil {
			t.Errorf("owner not running must be a warning: %v", err)
		}
		stubAttach(t, sharedproxy.Outcome{Missing: []string{"b"}}, nil)
		if err := afterStartSharedProxy(ctx, fronted("a"), b.ProjectDir); err != nil {
			t.Errorf("a missing network must be a warning: %v", err)
		}
	})

	t.Run("a failed attach of this project is E506", func(t *testing.T) {
		stubAttach(t, sharedproxy.Outcome{Quarantined: []string{"b"}}, e506)
		err := afterStartSharedProxy(ctx, fronted("a"), b.ProjectDir)
		if !errors.Is(err, e506) {
			t.Errorf("want the E506 error, got %v", err)
		}
	})

	t.Run("another project's quarantine does not fail this start", func(t *testing.T) {
		stubAttach(t, sharedproxy.Outcome{Attached: []string{"b"}, Quarantined: []string{"c"}, Reloaded: true}, e506)
		if err := afterStartSharedProxy(ctx, fronted("a"), b.ProjectDir); err != nil {
			t.Errorf("B is attached and serving; C's quarantine is a warning: %v", err)
		}
	})
}

func TestSharedProxyDetachOnDown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	owner, b, _ := proxyLayout(t)
	bin := t.TempDir()
	order := filepath.Join(bin, "order.log")
	script := "#!/bin/sh\necho \"docker $*\" >> '" + order + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	comp := &docker.Compose{DockerPath: filepath.Join(bin, "docker")}

	var detached []string
	var detachErr error
	orig := spDetach
	t.Cleanup(func() { spDetach = orig })
	spDetach = func(_ context.Context, ownerDir string, regs ...sharedproxy.Registration) error {
		if data, _ := os.ReadFile(order); len(data) != 0 {
			t.Errorf("detach ran after docker: %q", data)
		}
		for _, r := range regs {
			detached = append(detached, ownerDir+"|"+r.Project)
		}
		return detachErr
	}

	// Every compose-down path of a fronted project detaches first.
	for _, opts := range []docker.DownOptions{{RemoveOrphans: true}, {}, {RemoveVolumes: true, RemoveOrphans: true}, {RemoveImages: true}} {
		detached = nil
		_ = os.Remove(order)
		if err := comp.ComposeDown(context.Background(), b.ProjectDir, opts); err != nil {
			t.Fatal(err)
		}
		if len(detached) != 1 || detached[0] != owner+"|b" {
			t.Errorf("down %+v detached %v, want [%s|b]", opts, detached, owner)
		}
		if data, _ := os.ReadFile(order); !strings.HasPrefix(string(data), "docker compose down") {
			t.Errorf("docker down did not run after the detach: %q", data)
		}
	}

	// The owner's own down, and a project outside any proxy, detach nothing.
	detached = nil
	for _, dir := range []string{owner, t.TempDir()} {
		_ = os.Remove(order)
		if err := comp.ComposeDown(context.Background(), dir, docker.DownOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if len(detached) != 0 {
		t.Errorf("only a fronted project detaches, got %v", detached)
	}

	// A stopped owner or a failing detach never blocks the down.
	for _, e := range []error{sharedproxy.ErrOwnerNotRunning, errors.New("boom")} {
		detachErr = e
		_ = os.Remove(order)
		if err := comp.ComposeDown(context.Background(), b.ProjectDir, docker.DownOptions{}); err != nil {
			t.Errorf("detach error %v must not fail the down: %v", e, err)
		}
	}
}

func TestLeaveSharedProxyOnlyForFronted(t *testing.T) {
	owner, b, _ := proxyLayout(t)
	var left []string
	orig := spLeave
	t.Cleanup(func() { spLeave = orig })
	spLeave = func(_ context.Context, ownerDir, project string) ([]string, error) {
		left = append(left, ownerDir+"|"+project)
		return nil, nil
	}
	leaveSharedProxy(context.Background(), owner)
	leaveSharedProxy(context.Background(), t.TempDir())
	if len(left) != 0 {
		t.Fatalf("only a fronted project leaves: %v", left)
	}
	if err := leaveSharedProxy(context.Background(), b.ProjectDir); err != nil {
		t.Errorf("a clean leave: %v", err)
	}
	if len(left) != 1 || left[0] != owner+"|b" {
		t.Errorf("left = %v", left)
	}

	// A failed leave is returned, so uninstall cannot report full removal.
	spLeave = func(context.Context, string, string) ([]string, error) { return nil, errors.New("registration dir is read-only") }
	if err := leaveSharedProxy(context.Background(), b.ProjectDir); err == nil {
		t.Error("a failed leave must be returned to the caller")
	}
}
