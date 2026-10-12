// Package deploy — unit tests for PushSecrets.
//
// These tests cover: dry-run path, missing env file error, and env file
// resolution order (target-specific first, then .env fallback).
// No actual scp is invoked.
package deploy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// baseSSHConfig returns a minimal SSHConfig with a placeholder host and a
// non-existent key path (scp is never invoked in these tests).
func baseSSHConfig() SSHConfig {
	return SSHConfig{
		Host:    "nself@192.0.2.1",
		KeyPath: "/nonexistent/id_ed25519",
	}
}

// TestPushSecrets_DryRun verifies that dry-run mode prints a log line and
// returns nil without invoking scp (which would fail with a bad key path).
func TestPushSecrets_DryRun(t *testing.T) {
	dir := t.TempDir()

	// Write a target-specific env file so resolution succeeds.
	envPath := filepath.Join(dir, ".env.staging")
	if err := os.WriteFile(envPath, []byte("KEY=value\n"), 0600); err != nil {
		t.Fatalf("setup: write env file: %v", err)
	}

	opts := PushSecretsOptions{
		Target:      "staging",
		DryRun:      true,
		ProjectRoot: dir,
	}

	err := PushSecrets(context.Background(), baseSSHConfig(), opts)
	if err != nil {
		t.Errorf("dry-run: expected nil error, got: %v", err)
	}
}

// TestPushSecrets_MissingEnvFile verifies that PushSecrets returns an error
// when neither the target-specific file nor the fallback .env exists.
func TestPushSecrets_MissingEnvFile(t *testing.T) {
	dir := t.TempDir()
	// No .env files written — both candidates are absent.

	opts := PushSecretsOptions{
		Target:      "prod",
		ProjectRoot: dir,
	}

	err := PushSecrets(context.Background(), baseSSHConfig(), opts)
	if err == nil {
		t.Error("expected error for missing env file, got nil")
	}
}

// TestPushSecrets_EnvFileResolutionOrder verifies that:
//   - When both .env.{target} and .env exist, .env.{target} is chosen.
//   - When only .env exists, .env is chosen.
func TestPushSecrets_EnvFileResolutionOrder(t *testing.T) {
	t.Run("target-specific preferred over fallback", func(t *testing.T) {
		dir := t.TempDir()

		targetFile := filepath.Join(dir, ".env.staging")
		fallbackFile := filepath.Join(dir, ".env")
		if err := os.WriteFile(targetFile, []byte("TARGET=1\n"), 0600); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(fallbackFile, []byte("FALLBACK=1\n"), 0600); err != nil {
			t.Fatalf("setup: %v", err)
		}

		got, err := resolveEnvFile("", dir, "staging")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != targetFile {
			t.Errorf("expected target-specific file %q, got %q", targetFile, got)
		}
	})

	t.Run("fallback .env used when target-specific absent", func(t *testing.T) {
		dir := t.TempDir()

		fallbackFile := filepath.Join(dir, ".env")
		if err := os.WriteFile(fallbackFile, []byte("FALLBACK=1\n"), 0600); err != nil {
			t.Fatalf("setup: %v", err)
		}

		got, err := resolveEnvFile("", dir, "prod")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != fallbackFile {
			t.Errorf("expected fallback file %q, got %q", fallbackFile, got)
		}
	})

	t.Run("explicit path overrides resolution", func(t *testing.T) {
		dir := t.TempDir()

		explicit := filepath.Join(dir, "custom.env")
		if err := os.WriteFile(explicit, []byte("CUSTOM=1\n"), 0600); err != nil {
			t.Fatalf("setup: %v", err)
		}

		got, err := resolveEnvFile(explicit, dir, "staging")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != explicit {
			t.Errorf("expected explicit path %q, got %q", explicit, got)
		}
	})
}

// TestPushSecrets_EmptyHost verifies that an empty SSHConfig.Host returns
// an error without touching the filesystem.
func TestPushSecrets_EmptyHost(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("K=v\n"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	opts := PushSecretsOptions{
		Target:      "staging",
		ProjectRoot: dir,
	}

	err := PushSecrets(context.Background(), SSHConfig{KeyPath: "/tmp/key"}, opts)
	if err == nil {
		t.Error("expected error for empty host, got nil")
	}
}

// TestPushSecretsRemotePathValidation checks the remote path before any
// process runs; DryRun means no scp is needed.
func TestPushSecretsRemotePathValidation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("K=v\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for path, ok := range map[string]bool{
		"/opt/app/.env": true, "~/app/.env": true,
		"app/.env": false, "~": false, "~/": false, "~//etc/x": false, "/opt/../etc/x": false,
		"/opt/x;id": false, "/opt/x$(id)": false, "-x": false, "/opt/x y": false, "/": false,
	} {
		err := PushSecrets(context.Background(), baseSSHConfig(), PushSecretsOptions{
			Target: "staging", ProjectRoot: dir, DryRun: true, RemotePath: path,
		})
		if ok && err != nil {
			t.Errorf("path %q refused: %v", path, err)
		}
		if !ok && err == nil {
			t.Errorf("path %q accepted", path)
		}
	}
}

// TestPushSecretsLocalOperand proves a relative env file named like host:path
// reaches scp as an absolute path.
func TestPushSecretsLocalOperand(t *testing.T) {
	requireKeygen(t)
	t.Setenv("HOME", pinHome(t))
	read := installFakeSSHTools(t)
	installFakeKeyscan(t)
	if err := (remote.PinnedHostKeys{Path: DeployKnownHostsPath()}).Add("nself-203.0.113.7-22", "ssh-ed25519 AQID"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "host:x.env"), []byte("K=v\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	err := PushSecrets(context.Background(), SSHConfig{Host: "deploy@203.0.113.7", KeyPath: "/k/id"}, PushSecretsOptions{
		Target: "prod", EnvFile: "host:x.env", ProjectRoot: dir, RemotePath: "/opt/app/.env",
	})
	if err != nil {
		t.Fatalf("PushSecrets: %v", err)
	}
	calls := read()
	if len(calls) != 1 || calls[0].tool != "scp" {
		t.Fatalf("calls = %#v", calls)
	}
	a := calls[0].args
	local := a[len(a)-2]
	if !strings.HasPrefix(local, "/") || !strings.HasSuffix(local, "host:x.env") {
		t.Errorf("local operand = %q", local)
	}
}

// TestPushSecretsUnknownHostRefused: in v1.4 secret shipping still verifies
// the host key; an unknown key is refused and scp never runs.
func TestPushSecretsUnknownHostRefused(t *testing.T) {
	t.Setenv("NSELF_V15", "")
	t.Setenv("HOME", pinHome(t))
	read := installFakeSSHTools(t)
	installFakeKeyscan(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("K=v\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := PushSecrets(context.Background(), SSHConfig{Host: "deploy@203.0.113.7", KeyPath: "/k/id"}, PushSecretsOptions{
		Target: "prod", ProjectRoot: dir, RemotePath: "/opt/app/.env",
	})
	if err == nil || !strings.Contains(err.Error(), "E487") || !strings.Contains(err.Error(), "--trust-host-key SHA256:") {
		t.Fatalf("unknown host: %v", err)
	}
	if n := len(read()); n != 0 {
		t.Fatalf("scp recorded %d call(s) after the refusal", n)
	}
}
