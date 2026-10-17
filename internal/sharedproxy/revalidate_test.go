package sharedproxy

// Tests for the cross-family review fixes of P7-ADOPT-21: Attach re-validates
// its registrations under the lock, Unregister deletes through an os.Root, and
// Leave reports an owner lookup failure.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttachDropsAProjectThatLeftBeforeTheLock(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	if err := os.Remove(filepath.Join(DirOf(f.owner), FileName("b"))); err != nil {
		t.Fatal(err) // b left after the caller read its registration
	}
	out, err := Attach(ctx, f.owner, "", []Registration{b})
	if err != nil || len(out.Attached) != 0 {
		t.Fatalf("a project that left is not attached: %+v %v", out, err)
	}
	if strings.Contains(f.calls(), "network connect") {
		t.Errorf("b_network must not be connected:\n%s", f.calls())
	}
}

func TestAttachDropsAChangedRegistration(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	b := f.project("b")
	stale := b
	stale.ProjectDir = filepath.Join(f.owner, "elsewhere")
	out, err := Attach(ctx, f.owner, "", []Registration{stale})
	if err != nil || len(out.Attached) != 0 || strings.Contains(f.calls(), "network connect") {
		t.Fatalf("a registration whose project_dir changed is dropped: %+v %v", out, err)
	}
	out, err = Attach(ctx, f.owner, "", []Registration{b})
	if err != nil || len(out.Attached) != 1 {
		t.Fatalf("the matching registration attaches: %+v %v", out, err)
	}
}

func TestUnregisterNeverDeletesThroughASwappedDirectory(t *testing.T) {
	f := newProxyFixture(t)
	f.owner0()
	f.project("b")
	outside := t.TempDir()
	victim := filepath.Join(outside, "privkey.pem")
	f.write(victim, "outside key\n")
	certDir := filepath.Join(f.owner, "ssl", "certificates", "b-cert")
	swapped := false
	old := beforeRemove
	beforeRemove = func(rel string) {
		if swapped || !strings.HasSuffix(rel, "privkey.pem") {
			return
		}
		swapped = true
		if err := os.RemoveAll(certDir); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, certDir); err != nil {
			t.Skip("symlinks unavailable:", err)
		}
	}
	t.Cleanup(func() { beforeRemove = old })
	_, err := Unregister(f.owner, "b")
	if !swapped {
		t.Fatal("the swap hook never ran")
	}
	if err == nil || !isE506(err) {
		t.Errorf("a swapped directory must be refused with an E506: %v", err)
	}
	if _, serr := os.Stat(victim); serr != nil {
		t.Errorf("the file outside the owner dir was deleted: %v", serr)
	}
}

func TestLeaveReturnsAnOwnerLookupFailure(t *testing.T) {
	f := newProxyFixture(t, "b_network")
	f.owner0()
	f.project("b")
	// The first ps (the detach) works, the second (the reload lookup) fails.
	variantDocker(t, psFails, `ps) n=$(grep -c '^ps' "$FAKE_DIR/calls.log"); if [ "$n" -gt 1 ]; then echo boom >&2; exit 1; fi; echo ownerid ;;`)
	removed, err := Leave(context.Background(), f.owner, "b")
	if !isE506(err) || errors.Is(err, ErrOwnerNotRunning) {
		t.Fatalf("a lookup failure must come back as an E506: %v", err)
	}
	if len(removed) == 0 || f.live("b") || f.reloads() != 0 {
		t.Errorf("files are gone and nothing reloaded: %v live=%v reloads=%d", removed, f.live("b"), f.reloads())
	}
}
