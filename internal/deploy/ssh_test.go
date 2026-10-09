package deploy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeOnlyDeployRequiresKnownHost(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	marker := filepath.Join(bin, "rsync-ran")
	for name, body := range map[string]string{
		"ssh-keyscan": "#!/bin/sh\nprintf 'host.example ssh-ed25519 AQID\\n'\n",
		"rsync":       "#!/bin/sh\ntouch '" + marker + "'\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	err := DeployViaSsh(context.Background(), SSHConfig{Host: "u@host.example", RemotePath: "/opt/nself", EnvName: "live", ServerName: "web"}, "/tmp/compose.yml")
	if err == nil || !strings.Contains(err.Error(), "E487") {
		t.Fatalf("compose-only deploy: %v", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatal("rsync ran before host-key refusal")
	}
}

// TestDeployViaSsh_RejectsInjectedRemotePath (T31) is the defense-in-depth
// test for DeployViaSsh: even when an SSHConfig is constructed directly
// (bypassing the CLI-flag and inventory-file validation layers in
// cmd/commands and internal/controlplane), a Host carrying a RemotePath
// with shell metacharacters must be rejected before any rsync/ssh exec call
// is attempted -- so this test needs neither binary to be present on PATH.
func TestDeployViaSsh_RejectsInjectedRemotePath(t *testing.T) {
	payloads := []string{
		"ubuntu@example.com:/opt/x; id",
		"ubuntu@example.com:/opt/x$(id)",
		"ubuntu@example.com:/opt/x`id`",
	}

	for _, host := range payloads {
		cfg := SSHConfig{Host: host, KeyPath: "/tmp/does-not-matter"}
		err := DeployViaSsh(context.Background(), cfg, "/tmp/nself-compose.yml")
		if err == nil {
			t.Fatalf("DeployViaSsh(%q): expected rejection, got nil error", host)
		}
		if !strings.Contains(err.Error(), "unsafe characters") {
			t.Errorf("DeployViaSsh(%q): unexpected error: %v", host, err)
		}
	}
}

// TestDeployViaSsh_EmptyHost proves the pre-existing empty-Host guard is
// unaffected by the new RemotePath check (which runs after it).
func TestDeployViaSsh_EmptyHost(t *testing.T) {
	err := DeployViaSsh(context.Background(), SSHConfig{}, "/tmp/nself-compose.yml")
	if err == nil {
		t.Fatal("DeployViaSsh with empty Host: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Host is empty") {
		t.Errorf("unexpected error: %v", err)
	}
}
