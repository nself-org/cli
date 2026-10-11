package admin

// Purpose: tests for the admin connect --user / --port defaults against a
// [user@]host[:port] spec (D-0465). The end-to-end case runs a fake ssh from
// PATH and never opens a connection.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/admin"
	"github.com/nself-org/cli/internal/compat"
)

func TestSpecUserPort(t *testing.T) {
	for _, tc := range []struct {
		host     string
		flags    []string
		wantUser string
		wantPort int
	}{
		{"h.example.test", nil, "nself", 22},
		{"alice@h.example.test", nil, "alice", 22},
		{"h.example.test:2222", nil, "nself", 2222},
		{"alice@h.example.test:2222", nil, "alice", 2222},
		{"[2001:db8::1]:2222", nil, "nself", 2222},
		{"alice@h.example.test", []string{"--user", "alice"}, "alice", 22},
		{"alice@h.example.test", []string{"--user", "bob"}, "bob", 22}, // still a conflict downstream
		{"h.example.test:2222", []string{"--port", "2200"}, "nself", 2200},
		{"h.example.test", []string{"--user", "ops", "--port", "2022"}, "ops", 2022},
		{"bad host", nil, "nself", 22}, // unparsable: flag values; ssh step reports it
	} {
		cmd := connectCommand()
		if err := cmd.ParseFlags(tc.flags); err != nil {
			t.Fatal(err)
		}
		user, port := specUserPort(cmd.Flags(), tc.host)
		if user != tc.wantUser || port != tc.wantPort {
			t.Errorf("%q %v = %q, %d; want %q, %d", tc.host, tc.flags, user, port, tc.wantUser, tc.wantPort)
		}
	}
}

// TestSpecUserPort_NoConflictEndToEnd proves the spec reaches ssh: before
// D-0465 the defaults nself/22 made alice@h:2222 fail with "conflicts with
// host specification" (the positive control below keeps that refusal for an
// explicit, different --user).
func TestSpecUserPort_NoConflictEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake ssh is a shell script")
	}
	t.Setenv(compat.EnvVar, "")
	bin := t.TempDir()
	argv := filepath.Join(bin, "argv")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + argv + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	cmd := connectCommand()
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}
	host := "alice@h.example.test:2222"
	user, port := specUserPort(cmd.Flags(), host)
	if err := admin.VerifySSHKey(context.Background(), user, host, port); err != nil {
		t.Fatalf("VerifySSHKey: %v", err)
	}
	raw, err := os.ReadFile(argv)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(raw))
	tail := strings.Join(got[len(got)-5:], " ")
	if tail != "-p 2222 -- alice@h.example.test true" {
		t.Errorf("ssh argv tail = %q", tail)
	}

	cmd = connectCommand()
	if err := cmd.ParseFlags([]string{"--user", "bob"}); err != nil {
		t.Fatal(err)
	}
	user, port = specUserPort(cmd.Flags(), host)
	if err := admin.VerifySSHKey(context.Background(), user, host, port); err == nil || !strings.Contains(err.Error(), "conflicts with host specification") {
		t.Errorf("explicit --user bob with alice@ spec: %v", err)
	}
}
