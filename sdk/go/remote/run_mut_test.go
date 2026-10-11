package remote

// Purpose: mutation-killing tests for run.go: the bare-IPv6 check() path,
// hasPortOption bounds, and Session.Kill. No network.

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestHasPortOption_Bounds(t *testing.T) {
	tests := []struct {
		opts []string
		want bool
	}{
		{nil, false},
		{[]string{"-p"}, false},
		{[]string{"-q", "-p"}, false},
		{[]string{"x", "-p"}, false},
		{[]string{"-p", "22"}, true},
		{[]string{"-q", "-p", "22"}, true},
		{[]string{"-P", "22"}, false},
		{[]string{"22", "-p"}, false},
	}
	for _, tc := range tests {
		if got := hasPortOption(tc.opts); got != tc.want {
			t.Errorf("hasPortOption(%q) = %v, want %v", tc.opts, got, tc.want)
		}
	}
}

// A bare IPv6 destination with a -p option takes the bracketed retry; the
// caller's other options must still go through checkOptions afterwards.
func TestCheck_BareIPv6StillChecksOptions(t *testing.T) {
	if err := (Target{Dest: "::1", Options: []string{"-p", "22"}}).check(); err != nil {
		t.Fatalf("bare ipv6 with -p refused: %v", err)
	}
	if err := (Target{Dest: "u@::1", Options: []string{"-p", "22"}}).check(); err != nil {
		t.Fatalf("user@ipv6 with -p refused: %v", err)
	}
	bad := Target{Dest: "::1", Options: []string{"-p", "22", "-o", "ProxyCommand=evil"}}
	if err := bad.check(); err == nil {
		t.Fatal("bare ipv6 destination skipped the option allowlist")
	}
}

// When the bracketed retry itself fails, its error is returned, not the
// generic canonical-destination error.
func TestCheck_BareIPv6BadUserReturnsParseError(t *testing.T) {
	err := (Target{Dest: "bad;user@::1", Options: []string{"-p", "22"}}).check()
	if err == nil {
		t.Fatal("bad user accepted")
	}
	if strings.Contains(err.Error(), "invalid remote destination") {
		t.Fatalf("got the generic error, want the parse error: %v", err)
	}
}

func TestSessionKill(t *testing.T) {
	unstarted := &Session{cmd: exec.Command("sleep", "30")}
	if err := unstarted.Kill(); err != nil {
		t.Fatalf("Kill before Start = %v, want nil", err)
	}
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("no sleep: %v", err)
	}
	s := &Session{cmd: cmd}
	if err := s.Kill(); err != nil {
		t.Fatalf("Kill = %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("process exited cleanly; Kill did not kill it")
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("process still running after Kill")
	}
}
