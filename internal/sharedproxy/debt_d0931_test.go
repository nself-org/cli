package sharedproxy

// Regression tests for D-0931: a failed reload after Unregister must be retried
// by the next Leave, and an owner stopped by a failed attach must be started
// again.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLeaveRetriesAFailedReload(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	f.project("b")
	variantDocker(t, reloadFails, `echo "reload boom" >&2; exit 1`)
	if _, err := Leave(ctx, f.owner, "b"); !isE506(err) {
		t.Fatalf("the first leave must report the failed reload: %v", err)
	}
	if f.reloads() != 0 {
		t.Fatalf("no reload may have succeeded yet: %d", f.reloads())
	}
	variantDocker(t) // docker works again
	removed, err := Leave(ctx, f.owner, "b")
	if err != nil || len(removed) != 0 {
		t.Fatalf("the retry has nothing left to remove and must succeed: %v %v", removed, err)
	}
	if f.reloads() != 1 {
		t.Fatalf("the retry must reload the owner once, got %d", f.reloads())
	}
	// The pending state is cleared: a third leave is a plain no-op.
	if _, err := Leave(ctx, f.owner, "b"); err != nil || f.reloads() != 1 {
		t.Fatalf("a settled leave must not reload again: %v reloads=%d", err, f.reloads())
	}
}

func TestLeaveRetryKeepsPendingWhileNginxTestFails(t *testing.T) {
	ctx := context.Background()
	f := newProxyFixture(t, "b_network")
	f.owner0()
	f.project("b")
	variantDocker(t, reloadFails, `echo "reload boom" >&2; exit 1`)
	_, _ = Leave(ctx, f.owner, "b")
	variantDocker(t)
	f.write(filepath.Join(f.fake, "broken.txt"), "nginx/sites/other.conf\n")
	f.write(filepath.Join(f.owner, "nginx", "sites", "other.conf"), "bogus\n")
	if _, err := Leave(ctx, f.owner, "b"); !isE506(err) || f.reloads() != 0 {
		t.Fatalf("a failing nginx -t must stop the retry: %v reloads=%d", err, f.reloads())
	}
	if err := os.Remove(filepath.Join(f.owner, "nginx", "sites", "other.conf")); err != nil {
		t.Fatal(err)
	}
	if _, err := Leave(ctx, f.owner, "b"); err != nil || f.reloads() != 1 {
		t.Fatalf("the reload is still owed once nginx -t passes: %v reloads=%d", err, f.reloads())
	}
}

func ownerState(f *proxyFixture) string {
	b, _ := os.ReadFile(filepath.Join(f.fake, "state"))
	return strings.TrimSpace(string(b))
}

func TestFailedAttachStartsARestartingOwnerAgain(t *testing.T) {
	ctx := context.Background()
	connect := `connect) echo "$4" >> "$FAKE_DIR/nets.txt" ;;`
	cases := []struct {
		name     string
		existing []string
		variant  []string
		wantErr  bool
	}{
		{"network connect fails", []string{"b_network"}, []string{connect, `connect) echo "denied" >&2; exit 1 ;;`}, true},
		{"every network is missing", nil, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newProxyFixture(t, tc.existing...)
			f.owner0()
			b := f.project("b")
			f.write(filepath.Join(f.fake, "state"), "restarting\n")
			if tc.variant != nil {
				variantDocker(t, tc.variant...)
			}
			_, err := Attach(ctx, f.owner, "", []Registration{b})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
			if got := ownerState(f); got != "running" {
				t.Fatalf("the owner it stopped must be started again, state = %q\n%s", got, f.calls())
			}
		})
	}
}
