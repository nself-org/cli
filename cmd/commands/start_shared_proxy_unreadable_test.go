package commands

// Tests for start_shared_proxy.go when the registrations cannot be read (one
// invalid registration of any project makes sharedproxy.Locate fail for every
// project under that owner). The registrations and layout are real.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/sharedproxy"
)

// corruptRegistration adds a registration file that does not validate.
func corruptRegistration(t *testing.T, owner string) {
	t.Helper()
	bad := filepath.Join(sharedproxy.DirOf(owner), "zzz.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func isCLIE506(err error) bool {
	var ce *errs.CLIError
	return errors.As(err, &ce) && ce.Code == "E506"
}

func TestUnreadableRegistrations(t *testing.T) {
	ctx := context.Background()
	owner, b, _ := proxyLayout(t)
	corruptRegistration(t, owner)

	t.Run("a fronted start fails with E506", func(t *testing.T) {
		calls := stubAttach(t, sharedproxy.Outcome{}, nil)
		err := afterStartSharedProxy(ctx, fronted("a"), b.ProjectDir)
		if !isCLIE506(err) || len(*calls) != 0 {
			t.Fatalf("err %v calls %v: want E506 and no attach", err, *calls)
		}
	})

	t.Run("the owner's start only warns", func(t *testing.T) {
		stubAttach(t, sharedproxy.Outcome{}, nil)
		var err error
		out := captureStderr(t, func() { err = afterStartSharedProxy(ctx, &config.Config{}, owner) })
		if err != nil || !strings.Contains(out, "not readable") {
			t.Fatalf("err %v, stderr %q", err, out)
		}
	})

	t.Run("no owner and no registration is not an error", func(t *testing.T) {
		calls := stubAttach(t, sharedproxy.Outcome{}, nil)
		if err := afterStartSharedProxy(ctx, fronted("a"), t.TempDir()); err != nil || len(*calls) != 0 {
			t.Fatalf("err %v calls %v", err, *calls)
		}
	})

	t.Run("the pre-down hook warns with the cause and continues", func(t *testing.T) {
		detached := false
		orig := spDetach
		t.Cleanup(func() { spDetach = orig })
		spDetach = func(context.Context, string, ...sharedproxy.Registration) error { detached = true; return nil }
		out := captureStderr(t, func() { beforeStopSharedProxy(ctx, b.ProjectDir) })
		if detached || !strings.Contains(out, "not readable") || !strings.Contains(out, "zzz.json") {
			t.Fatalf("detached=%v, stderr %q: want a warning naming the unreadable file", detached, out)
		}
	})

	t.Run("uninstall's leave returns E506", func(t *testing.T) {
		left := false
		orig := spLeave
		t.Cleanup(func() { spLeave = orig })
		spLeave = func(context.Context, string, string) ([]string, error) { left = true; return nil, nil }
		var err error
		captureStderr(t, func() { err = leaveSharedProxy(ctx, b.ProjectDir) })
		if !isCLIE506(err) || left {
			t.Fatalf("err %v left=%v: uninstall must see an E506 and not report a clean leave", err, left)
		}
	})
}
