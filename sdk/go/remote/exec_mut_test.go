package remote

// Purpose: mutation-killing tests for Command (D-0253): NUL bytes refused at
// every argument position and the exec hook never reached for them.
// No network; no real ssh runs.

import (
	"context"
	"strings"
	"testing"
)

// Mutation-killing tests (D-0253).

func TestCommand_RefusesNULAtEveryPosition(t *testing.T) {
	n := countExecs(t)
	for _, a := range []string{"\x00", "\x00abc", "abc\x00", "a\x00c"} {
		if _, err := Command(context.Background(), "ssh", "ok", a); err == nil {
			t.Errorf("Command(ssh, %q) = nil error", a)
		}
	}
	if *n != 0 {
		t.Fatalf("hook called %d times for NUL arguments", *n)
	}
	if _, err := Command(context.Background(), "ssh", "ok", "plain"); err != nil {
		t.Fatalf("plain argv refused: %v", err)
	}
}

func TestEnvAllowlist_SkipsEmptyKeepsSet(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", "/h")
	t.Setenv("USER", "")      // set but empty: dropped
	t.Setenv("LOGNAME", "ln") // set: kept
	t.Setenv("SSH_AUTH_SOCK", "")
	got := strings.Join(EnvAllowlist(), "\n")
	for _, w := range []string{"PATH=/usr/bin", "HOME=/h", "LOGNAME=ln", "LANG=C"} {
		if !strings.Contains("\n"+got+"\n", "\n"+w+"\n") {
			t.Errorf("EnvAllowlist missing %q in %q", w, got)
		}
	}
	for _, bad := range []string{"USER=", "SSH_AUTH_SOCK="} {
		if strings.Contains("\n"+got, "\n"+bad) {
			t.Errorf("EnvAllowlist kept an empty variable %q", bad)
		}
	}
}
