package sharedproxy

// Tests for the error branches of attach.go and attach_flow.go: a missing or
// stopped owner, a failing reload, a cold owner that does not start, and the
// docker calls that fail mid-attach. They reuse the fake docker of
// attach_test.go with one line of the script swapped per case.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// variantDocker replaces the fake docker script with a copy where each old
// snippet is swapped for its new one (pairs: old, new, old, new ...).
func variantDocker(t *testing.T, pairs ...string) {
	t.Helper()
	script := fakeProxyDocker
	for i := 0; i+1 < len(pairs); i += 2 {
		next := strings.Replace(script, pairs[i], pairs[i+1], 1)
		if next == script {
			t.Fatalf("the fake docker script changed: %q not found", pairs[i])
		}
		script = next
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func fastStart(t *testing.T) {
	t.Helper()
	oldPoll, oldWait := startPoll, startWait
	startPoll, startWait = time.Millisecond, 30*time.Millisecond
	t.Cleanup(func() { startPoll, startWait = oldPoll, oldWait })
}

const (
	psFails     = `ps) [ -f "$FAKE_DIR/stopped" ] || echo ownerid ;;`
	reloadFails = `echo reload >> "$FAKE_DIR/reloads.log"`
	netsFail    = `'{{json .NetworkSettings.Networks}}') nets_json ;;`
	startOK     = `start) echo running > "$FAKE_DIR/state" ;;`
	stopOK      = `stop) echo exited > "$FAKE_DIR/state" ;;`
	netInspect  = `inspect) n=$(eval echo \$$#); if`
	disconnect  = `      disconnect) `
)

func TestOwnerNginxStates(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t)
	f.owner0()

	name, err := OwnerNginx(ctx, f.owner)
	if err != nil || name == "" {
		t.Fatalf("a running owner: %q %v", name, err)
	}

	f.setState("exited")
	name, err = OwnerNginx(ctx, f.owner)
	if !errors.Is(err, ErrOwnerNotRunning) || isE506(err) || name != "" || !strings.Contains(err.Error(), "exited") {
		t.Errorf("a stopped owner: %q %v", name, err)
	}

	f.setState("running")
	f.write(filepath.Join(f.fake, "stopped"), "")
	if _, err = OwnerNginx(ctx, f.owner); !errors.Is(err, ErrOwnerNotRunning) {
		t.Errorf("no owner container at all: %v", err)
	}
}

func TestOwnerErr(t *testing.T) {
	if ownerErr(nil) != nil {
		t.Error("nil must stay nil")
	}
	wrapped := errors.Join(errors.New("x"), ErrOwnerNotRunning)
	if got := ownerErr(wrapped); got != wrapped {
		t.Errorf("the not-running sentinel must pass through untouched: %v", got)
	}
	got := ownerErr(errors.New("daemon down"))
	if !isE506(got) || errors.Is(got, ErrOwnerNotRunning) || !strings.Contains(got.Error(), "daemon down") {
		t.Errorf("another failure must become an E506 that keeps the cause: %v", got)
	}
}

func TestAttachDockerDownIsE506(t *testing.T) {
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	variantDocker(t, psFails, `ps) echo "daemon down" >&2; exit 1 ;;`)
	_, err := Attach(context.Background(), f.owner, "b", []Registration{b})
	if !isE506(err) || errors.Is(err, ErrOwnerNotRunning) {
		t.Fatalf("a docker failure is an E506, not 'not running': %v", err)
	}
	if err := Detach(context.Background(), f.owner, b); !isE506(err) {
		t.Errorf("Detach with docker down: %v", err)
	}
}

func TestAttachReloadFailure(t *testing.T) {
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	variantDocker(t, reloadFails, `echo "reload boom" >&2; exit 1`)
	out, err := Attach(context.Background(), f.owner, "b", []Registration{b})
	if !isE506(err) || !strings.Contains(err.Error(), "reload") {
		t.Fatalf("want an E506 about the reload, got %v", err)
	}
	if out.Reloaded || len(out.Attached) != 0 {
		t.Errorf("a failed reload must report no success: %+v", out)
	}
	if strings.Contains(f.nets(), "b_network") {
		t.Errorf("a failed reload must undo the connection:\n%s", f.nets())
	}
}

func TestAttachColdOwnerThatDoesNotStart(t *testing.T) {
	fastStart(t)
	ctx := context.Background()
	cases := []struct {
		name     string
		pairs    []string
		wantText string
	}{
		{"docker start fails", []string{startOK, `start) echo "no such image" >&2; exit 1 ;;`}, "did not start"},
		{"crash loop", []string{startOK, `start) echo restarting > "$FAKE_DIR/state" ;;`}, "did not start"},
		{"inspect fails after start", []string{
			startOK, `start) touch "$FAKE_DIR/dead" ;;`,
			"  inspect)\n    case \"$3\" in", "  inspect) [ -f \"$FAKE_DIR/dead\" ] && { echo \"no such container\" >&2; exit 1; }\n    case \"$3\" in",
		}, "did not start"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newProxyFixture(t, "b_network")
			f.owner0()
			b := f.project("b")
			f.setState("exited")
			variantDocker(t, tc.pairs...)
			out, err := Attach(ctx, f.owner, "", []Registration{b})
			if !isE506(err) || !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("want an E506 %q, got %v", tc.wantText, err)
			}
			if len(out.Attached) != 0 || out.Reloaded || f.reloads() != 0 {
				t.Errorf("a cold owner that never ran must report nothing attached or reloaded: %+v", out)
			}
		})
	}
}

func TestStartAndWaitReportsTheStuckState(t *testing.T) {
	fastStart(t)
	f := newProxyFixture(t)
	f.owner0()
	variantDocker(t, startOK, `start) echo restarting > "$FAKE_DIR/state" ;;`)
	f.setState("exited")
	err := startAndWait(context.Background(), "owner_nginx")
	if err == nil || !strings.Contains(err.Error(), "is restarting after") {
		t.Fatalf("a container that never runs must name its state: %v", err)
	}
}

func TestAttachDockerCallFailures(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		state string
		pairs []string
		want  string
	}{
		{"stop of a restarting owner", "restarting", []string{stopOK, `stop) echo "denied" >&2; exit 1 ;;`}, "cannot stop"},
		{"reading the owner networks", "running", []string{netsFail, `'{{json .NetworkSettings.Networks}}') echo "inspect boom" >&2; exit 1 ;;`}, "networks"},
		{"checking the network", "running", []string{netInspect, `inspect) echo "daemon boom" >&2; exit 1; n=$(eval echo \$$#); if`}, "cannot check network"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newProxyFixture(t, "b_network")
			f.owner0()
			b := f.project("b")
			f.setState(tc.state)
			variantDocker(t, tc.pairs...)
			out, err := Attach(ctx, f.owner, "", []Registration{b})
			if !isE506(err) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an E506 containing %q, got %v", tc.want, err)
			}
			if len(out.Attached) != 0 || strings.Contains(f.calls(), "network connect") {
				t.Errorf("nothing may be connected: %+v\n%s", out, f.calls())
			}
		})
	}
}

func TestAttachRefusesBadInput(t *testing.T) {
	ctx := context.Background()
	t.Run("a registration naming another project's network", func(t *testing.T) {
		f := newProxyFixture(t, "b_network", "x_network")
		f.owner0()
		b := f.project("b")
		b.Network = "x_network"
		if _, err := Attach(ctx, f.owner, "b", []Registration{b}); !isE506(err) {
			t.Fatalf("want E506, got %v", err)
		}
		if strings.Contains(f.calls(), "network connect") {
			t.Errorf("a foreign network must never be connected:\n%s", f.calls())
		}
	})
	t.Run("an unreadable registration", func(t *testing.T) {
		f := newProxyFixture(t, "b_network")
		f.owner0()
		b := f.project("b")
		f.write(filepath.Join(DirOf(f.owner), "broken.json"), "{ not json")
		if _, err := Attach(ctx, f.owner, "b", []Registration{b}); !isE506(err) || strings.Contains(f.calls(), "network connect") {
			t.Fatalf("want E506 before any connect, got %v\n%s", err, f.calls())
		}
		if err := Detach(ctx, f.owner, b); !isE506(err) {
			t.Errorf("Detach with an unreadable registration: %v", err)
		}
	})
	t.Run("no registrations", func(t *testing.T) {
		f := newProxyFixture(t)
		out, err := Attach(ctx, f.owner, "", nil)
		if err != nil || out.Reloaded || len(out.Attached) != 0 {
			t.Errorf("nothing to attach: %+v %v", out, err)
		}
	})
}

func TestAttachRestoreFailure(t *testing.T) {
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	f.quarantineAll(b)
	// nginx/sites becomes a file: the quarantined conf cannot go back.
	if err := os.RemoveAll(filepath.Join(f.owner, "nginx", "sites")); err != nil {
		t.Fatal(err)
	}
	f.write(filepath.Join(f.owner, "nginx", "sites"), "in the way")
	_, err := Attach(context.Background(), f.owner, "b", []Registration{b})
	if !isE506(err) || !strings.Contains(err.Error(), "restore") {
		t.Fatalf("want an E506 about the restore, got %v", err)
	}
	if strings.Contains(f.calls(), "network connect") {
		t.Errorf("no connect after a failed restore:\n%s", f.calls())
	}
}
