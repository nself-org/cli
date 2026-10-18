package sharedproxy

// Tests for D-0928: a cold owner (stopped nginx) is never started with a
// configuration that fails `nginx -t`. The merged configuration is tested in a
// throwaway container first; startAndWait needs the container to stay up.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// crashOnBrokenStart makes `docker start` of the owner leave it restarting
// (a crash loop) while a listed file exists, as a real nginx with a broken
// conf does. The throwaway `start -a` is handled before the case statement.
const crashOnBrokenStart = `start) if [ -n "$(cat "$FAKE_DIR/broken.txt")" ] && [ -e "$OWNER_DIR/$(head -1 "$FAKE_DIR/broken.txt")" ]; then echo restarting > "$FAKE_DIR/state"; else echo running > "$FAKE_DIR/state"; fi ;;`

func TestAttachColdOwnerBrokenConfIsQuarantinedBeforeStart(t *testing.T) {
	startPoll, startWait, startGrace = time.Millisecond, time.Second, 5*time.Millisecond
	f := newProxyFixture(t, "b_network", "c_network")
	f.owner0()
	b, c := f.project("b"), f.project("c")
	f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/b.api.conf\n")
	f.setState("exited")
	variantDocker(t, startOK, crashOnBrokenStart)

	out, err := Attach(context.Background(), f.owner, "", []Registration{b, c})
	if !isE506(err) || !strings.Contains(err.Error(), "quarantine") {
		t.Fatalf("want an E506 naming the quarantine, got %v", err)
	}
	if len(out.Quarantined) != 1 || out.Quarantined[0] != "b" || len(out.Attached) != 1 || out.Attached[0] != "c" {
		t.Fatalf("b must be quarantined and c attached: %+v", out)
	}
	if f.state() != "running" || !out.Reloaded {
		t.Errorf("the owner must start without the broken project: state %s, %+v", f.state(), out)
	}
	if _, serr := os.Stat(filepath.Join(f.owner, "nginx", "sites", "b.api.conf")); !errors.Is(serr, os.ErrNotExist) {
		t.Errorf("the broken conf must be out of nginx/sites: %v", serr)
	}
	calls := f.calls()
	if ci, si := strings.Index(calls, "create "), strings.Index(calls, "start -- "); ci < 0 || si < ci {
		t.Errorf("the throwaway test must run before the owner starts:\n%s", calls)
	}
	if !strings.Contains(calls, "--volumes-from") || !strings.Contains(calls, "nginx -t") {
		t.Errorf("the throwaway must reuse the owner's mounts and run nginx -t:\n%s", calls)
	}
}

func TestAttachColdOwnerUnfixableConfStartsNothing(t *testing.T) {
	startPoll, startWait, startGrace = time.Millisecond, time.Second, 5*time.Millisecond
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	// The error names a file no registration owns, so no one can be quarantined.
	f.write(filepath.Join(f.owner, "nginx", "other.conf"), "bogus\n")
	f.write(filepath.Join(f.fake, "broken.txt"), "nginx/other.conf\n")
	f.setState("exited")

	out, err := Attach(context.Background(), f.owner, "", []Registration{b})
	if !isE506(err) || len(out.Attached) != 0 || out.Reloaded {
		t.Fatalf("an unattributable failure must attach and reload nothing: %+v %v", out, err)
	}
	if strings.Contains(f.calls(), "start -- ") || f.state() != "exited" {
		t.Errorf("the owner must stay stopped:\n%s", f.calls())
	}
	if strings.Contains(f.nets(), "b_network") {
		t.Errorf("the connection must be undone: %s", f.nets())
	}
}

func TestAttachColdOwnerThrowawayCannotRun(t *testing.T) {
	startPoll, startWait, startGrace = time.Millisecond, time.Second, 5*time.Millisecond
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	f.setState("exited")
	variantDocker(t, "  create) echo cid1 ;;", `  create) echo "no space left" >&2; exit 1 ;;`)

	out, err := Attach(context.Background(), f.owner, "", []Registration{b})
	if !isE506(err) || !strings.Contains(err.Error(), "before") || len(out.Attached) != 0 {
		t.Fatalf("a throwaway that cannot run is an E506, not a quarantine: %+v %v", out, err)
	}
	if len(out.Quarantined) != 0 || strings.Contains(f.calls(), "start -- ") {
		t.Errorf("nothing may be quarantined or started:\n%s", f.calls())
	}
	if _, serr := os.Stat(filepath.Join(f.owner, "nginx", "sites", "b.api.conf")); serr != nil {
		t.Errorf("the conf must stay where it is: %v", serr)
	}
}

func TestStartAndWaitRejectsACrashLoop(t *testing.T) {
	startPoll, startWait, startGrace = time.Millisecond, 2*time.Second, time.Second
	f := newProxyFixture(t)
	f.owner0()
	f.setState("exited")
	// The first sample says running; the next one says the container died.
	variantDocker(t, startOK, `start) echo running > "$FAKE_DIR/state"; (sleep 0.3; echo restarting > "$FAKE_DIR/state") & ;;`)

	err := startAndWait(context.Background(), "owner_nginx")
	if err == nil || !strings.Contains(err.Error(), "restarting") {
		t.Fatalf("a container that runs once and dies must fail the wait: %v", err)
	}
}

func TestStartAndWaitPassesAfterTheGrace(t *testing.T) {
	startPoll, startWait, startGrace = time.Millisecond, time.Second, 10*time.Millisecond
	f := newProxyFixture(t)
	f.owner0()
	f.setState("exited")
	if err := startAndWait(context.Background(), "owner_nginx"); err != nil {
		t.Fatalf("a container that stays up must pass: %v", err)
	}
}
