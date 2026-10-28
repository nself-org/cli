package admin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminSSHKnownHostAndEntryPoints(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	e := newArgvEnv(t, "", &hostKey{alias: "[host.test]:2222"})
	for _, entry := range entryNames {
		if err := callEntry(entry, "u", "host.test", 2222); err != nil {
			t.Fatal(err)
		}
	}
	args := strings.Join(e.argv(t), " ")
	if !strings.Contains(args, "StrictHostKeyChecking=yes") || strings.Contains(args, "HostKeyAlias=") {
		t.Fatalf("trusted port argv: %s", args)
	}
	if err := os.Remove(filepath.Join(e.home, ".ssh", "known_hosts")); err != nil {
		t.Fatal(err)
	}
	if err := VerifySSHKey(context.Background(), "u", "host.test", 2222); err == nil || !strings.Contains(err.Error(), "E487") || !strings.Contains(err.Error(), "known_hosts") || strings.Contains(err.Error(), "env target add") {
		t.Fatalf("unknown key hint: %v", err)
	}
}

func TestAdminSSHKnownHostDefaultPort(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	e := newArgvEnv(t, "", &hostKey{alias: "host.test"})
	if err := VerifySSHKey(context.Background(), "u", "host.test", 22); err != nil {
		t.Fatal(err)
	}
	args := strings.Join(e.argv(t), " ")
	if !strings.Contains(args, "StrictHostKeyChecking=yes") || strings.Contains(args, "HostKeyAlias=") {
		t.Fatalf("default-port argv: %s", args)
	}
}
