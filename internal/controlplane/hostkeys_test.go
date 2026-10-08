package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

func TestHostKeyPolicy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	bin := t.TempDir()
	key := "host.example ssh-ed25519 AQID"
	keyscan := "#!/bin/sh\nprintf '%s\\n' '" + key + "'\n"
	keygen := "#!/bin/sh\nif [ -f \"$4\" ]; then /bin/cat \"$4\"; fi\n"
	for name, script := range map[string]string{"ssh-keyscan": keyscan, "ssh-keygen": keygen} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("NSELF_V15", "0")
	opts, err := HostKeyOptions(context.Background(), "qa", "web", TierLocalServers, "u@host.example", true)
	if err != nil || !strings.Contains(strings.Join(opts, " "), "accept-new") {
		t.Fatalf("v1.4: %v %q", err, opts)
	}
	t.Setenv("NSELF_V15", "1")
	_, err = HostKeyOptions(context.Background(), "qa", "web", TierLocalServers, "u@host.example", true)
	if err == nil || !strings.Contains(err.Error(), "E487") || !strings.Contains(err.Error(), "env target add qa web --trust-host-key SHA256:") {
		t.Fatalf("unknown: %v", err)
	}
	if _, statErr := os.Stat(DeployKnownHostsPath()); !os.IsNotExist(statErr) {
		t.Fatal("unknown key changed pin file")
	}
	fp, err := remote.Fingerprint(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := TrustHostKey(context.Background(), "u@host.example", "SHA256:wrong"); err == nil {
		t.Fatal("wrong fingerprint accepted")
	}
	if _, statErr := os.Stat(DeployKnownHostsPath()); !os.IsNotExist(statErr) {
		t.Fatal("wrong fingerprint changed pin file")
	}
	if err := TrustHostKey(context.Background(), "u@host.example", fp); err != nil {
		t.Fatal(err)
	}
	opts, err = HostKeyOptions(context.Background(), "qa", "web", TierLocalServers, "u@host.example", true)
	if err != nil || !strings.Contains(strings.Join(opts, " "), "StrictHostKeyChecking=yes") {
		t.Fatalf("trusted: %v %q", err, opts)
	}
	if !strings.HasPrefix(DeployKnownHostsPath(), filepath.Join(dir, ".config")) {
		t.Fatal("pin path outside HOME")
	}
}

func TestHostSpecArgvSites(t *testing.T) {
	files := []string{
		"internal/controlplane/probe.go", "internal/controlplane/lb.go",
		"internal/deploy/ssh.go", "internal/deploy/remote_exec.go", "internal/deploy/secrets.go",
		"internal/access/transport_ssh.go", "cmd/commands/deploy_remote.go",
		"cmd/commands/deploy_run.go", "cmd/commands/deploy_diagnostics.go",
		"cmd/commands/deploy_health_remote.go", "cmd/commands/db_remote.go", "cmd/commands/db_remote_version.go",
	}
	for _, file := range files {
		b, err := os.ReadFile(filepath.Join("..", "..", file))
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if !strings.Contains(s, "ParseHostSpec") || !(strings.Contains(s, "SSHArgs()") || strings.Contains(s, "SSHOptions()")) {
			t.Errorf("%s does not parse hosts and build argv from HostSpec", file)
		}
	}
}
