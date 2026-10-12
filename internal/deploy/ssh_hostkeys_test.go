package deploy

// Purpose: tests for deploy host-key policy: secret shipping pins in v1.4, the
//          pin-file lookup fails closed, and IP hosts get a synthetic alias.
// Inputs:  HOME set to a temp dir, fake ssh-keyscan, ssh, scp and rsync.
// Outputs: asserts error codes, argv and that no copy ran on refusal.
// Constraints: no network; only fake binaries run.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

func writeEnvFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env.live")
	if err := os.WriteFile(p, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// requireKeygen skips the test when ssh-keygen is missing: pinned and operator
// keys are matched with ssh-keygen -F, the matcher ssh itself uses.
func requireKeygen(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// HOME does not move os.UserHomeDir there, so the pin file would be the real one.
		t.Skip("pin-file tests need a POSIX HOME")
	}
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not on PATH")
	}
}

// TestSecretShippingPinsHostKeyV14: PushSecrets ships secrets, so it verifies
// the host key in v1.4 as well. DeployViaSsh with an env file stays accept-new
// in v1.4 (moved to P7-DEPL-29) and runs no ssh-keyscan.
// pinHome returns a 0700 temp dir for HOME. t.TempDir follows the umask, so
// on a host with umask 002 (ps1) it is 0775 and, under compat.V15, the pin
// store refuses it as a group-writable ancestor of ~/.config/nself (D-0637).
// The production check stays.
func pinHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestSecretShippingPinsHostKeyV14(t *testing.T) {
	requireKeygen(t)
	for _, v15 := range []string{"", "0"} {
		t.Run("PushSecrets NSELF_V15="+v15, func(t *testing.T) {
			t.Setenv("NSELF_V15", v15)
			t.Setenv("HOME", pinHome(t))
			read := installFakeSSHTools(t)
			installFakeKeyscan(t)
			cfg := SSHConfig{Host: "u@203.0.113.7", KeyPath: "/k/id", ServerName: "web"}
			opts := PushSecretsOptions{Target: "live", EnvFile: writeEnvFile(t), RemotePath: "/opt/nself/.env"}
			err := PushSecrets(context.Background(), cfg, opts)
			if err == nil || !strings.Contains(err.Error(), "E487") || !strings.Contains(err.Error(), "--trust-host-key SHA256:") {
				t.Fatalf("unknown host: %v", err)
			}
			if n := len(read()); n != 0 {
				t.Fatalf("%d process(es) ran before the host-key refusal", n)
			}
			alias := deployKeyAlias(mustSpec(t, cfg.Host))
			if err := (remote.PinnedHostKeys{Path: DeployKnownHostsPath()}).Add(alias, "ssh-ed25519 AQID"); err != nil {
				t.Fatal(err)
			}
			if err := PushSecrets(context.Background(), cfg, opts); err != nil {
				t.Fatalf("pinned push: %v", err)
			}
			calls := read()
			if len(calls) != 1 || calls[0].tool != "scp" {
				t.Fatalf("calls = %#v", calls)
			}
			joined := strings.Join(calls[0].args, " ")
			for _, w := range []string{"StrictHostKeyChecking=yes", "GlobalKnownHostsFile=" + DeployKnownHostsPath(), "HostKeyAlias=" + alias} {
				if !strings.Contains(joined, w) {
					t.Errorf("scp argv lacks %q: %s", w, joined)
				}
			}
		})
	}
	t.Run("DeployViaSsh env file v1.4 stays accept-new without a scan", func(t *testing.T) {
		t.Setenv("NSELF_V15", "")
		t.Setenv("HOME", pinHome(t))
		read := installFakeSSHTools(t)
		scans := installFakeKeyscan(t)
		cfg := SSHConfig{Host: "u@203.0.113.7", RemotePath: "/opt/nself", EnvName: "live", EnvFile: writeEnvFile(t), KeyPath: "/k/id"}
		if err := DeployViaSsh(context.Background(), cfg, "/work/compose.yml"); err != nil {
			t.Fatalf("env deploy: %v", err)
		}
		calls := read()
		if len(calls) == 0 || !strings.Contains(strings.Join(calls[0].args, " "), "StrictHostKeyChecking=accept-new") {
			t.Fatalf("calls = %#v", calls)
		}
		if n := scans(); n != 0 {
			t.Errorf("ssh-keyscan ran %d time(s)", n)
		}
	})
	t.Run("DeployViaSsh strict in v1.5", func(t *testing.T) {
		t.Setenv("NSELF_V15", "1")
		t.Setenv("HOME", pinHome(t))
		read := installFakeSSHTools(t)
		installFakeKeyscan(t)
		cfg := SSHConfig{Host: "u@203.0.113.7", RemotePath: "/opt/nself", EnvName: "live", EnvFile: writeEnvFile(t), KeyPath: "/k/id"}
		err := DeployViaSsh(context.Background(), cfg, "/work/compose.yml")
		if err == nil || !strings.Contains(err.Error(), "E487") || len(read()) != 0 {
			t.Fatalf("v1.5 unknown host: %v", err)
		}
		if err := (remote.PinnedHostKeys{Path: DeployKnownHostsPath()}).Add("nself-203.0.113.7-22", "ssh-ed25519 AQID"); err != nil {
			t.Fatal(err)
		}
		if err := DeployViaSsh(context.Background(), cfg, "/work/compose.yml"); err != nil {
			t.Fatalf("pinned deploy: %v", err)
		}
		if joined := strings.Join(read()[0].args, " "); !strings.Contains(joined, "StrictHostKeyChecking=yes") {
			t.Errorf("rsync argv: %s", joined)
		}
	})
	t.Run("compose-only v1.4 stays accept-new without a scan", func(t *testing.T) {
		t.Setenv("NSELF_V15", "")
		t.Setenv("HOME", pinHome(t))
		read := installFakeSSHTools(t)
		scans := installFakeKeyscan(t)
		cfg := SSHConfig{Host: "u@203.0.113.7", RemotePath: "/opt/nself", EnvName: "live", KeyPath: "/k/id"}
		if err := DeployViaSsh(context.Background(), cfg, "/work/compose.yml"); err != nil {
			t.Fatalf("compose-only deploy: %v", err)
		}
		calls := read()
		if len(calls) == 0 || !strings.Contains(strings.Join(calls[0].args, " "), "StrictHostKeyChecking=accept-new") {
			t.Fatalf("calls = %#v", calls)
		}
		if n := scans(); n != 0 {
			t.Errorf("ssh-keyscan ran %d time(s)", n)
		}
	})
}

func TestPinnedLookupRefusesForeignLine(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	t.Setenv("HOME", pinHome(t))
	read := installFakeSSHTools(t)
	installFakeKeyscan(t)
	pin := DeployKnownHostsPath()
	if err := os.MkdirAll(filepath.Dir(pin), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pin, []byte("* ssh-ed25519 AQID\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := SSHConfig{Host: "u@host.example", RemotePath: "/opt/nself", EnvName: "live", EnvFile: writeEnvFile(t), KeyPath: "/k/id"}
	err := DeployViaSsh(context.Background(), cfg, "/work/compose.yml")
	if err == nil || !strings.Contains(err.Error(), "E487") || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("foreign pin line: %v", err)
	}
	if n := len(read()); n != 0 {
		t.Fatalf("%d process(es) ran despite the refused pin file", n)
	}
}

func TestDeployKeyAliasIPHost(t *testing.T) {
	requireKeygen(t)
	for in, want := range map[string]string{
		"u@203.0.113.7":        "nself-203.0.113.7-22",
		"u@[2001:db8::1]:2222": "nself-2001-db8--1-2222",
		"u@host.example:2222":  "nself-host.example-2222",
		"u@host.example":       "host.example",
	} {
		if got := deployKeyAlias(mustSpec(t, in)); got != want {
			t.Errorf("deployKeyAlias(%q) = %q, want %q", in, got, want)
		}
	}
	t.Setenv("HOME", pinHome(t))
	installFakeKeyscan(t)
	ctx := context.Background()
	fp, err := remote.Fingerprint("h ssh-ed25519 AQID")
	if err != nil {
		t.Fatal(err)
	}
	if err := TrustHostKey(ctx, "u@203.0.113.7", fp); err != nil {
		t.Fatalf("TrustHostKey on an IP host: %v", err)
	}
	t.Setenv("NSELF_V15", "1")
	opts, err := HostKeyOptions(ctx, "u@203.0.113.7", "live", "web", true)
	if err != nil {
		t.Fatalf("HostKeyOptions: %v", err)
	}
	if got := strings.Join(opts, " "); !strings.Contains(got, "HostKeyAlias=nself-203.0.113.7-22") {
		t.Errorf("options = %q", got)
	}
}

// TestSecretShippingOperatorKnownHost: a key the operator already trusts in
// ~/.ssh/known_hosts satisfies strict checking with no HostKeyAlias.
func TestSecretShippingOperatorKnownHost(t *testing.T) {
	requireKeygen(t)
	for host, line := range map[string]string{
		"u@host.example":      "host.example ssh-ed25519 AQID",
		"u@host.example:2222": "[host.example]:2222 ssh-ed25519 AQID",
	} {
		host, line := host, line
		t.Run(host, func(t *testing.T) {
			t.Setenv("NSELF_V15", "1")
			home := pinHome(t)
			t.Setenv("HOME", home)
			if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, ".ssh", "known_hosts"), []byte(line+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			read := installFakeSSHTools(t)
			installFakeKeyscan(t)
			cfg := SSHConfig{Host: host, RemotePath: "/opt/nself", EnvName: "live", ServerName: "web",
				EnvFile: writeEnvFile(t), KeyPath: "/k/id"}
			if err := DeployViaSsh(context.Background(), cfg, "/work/compose.yml"); err != nil {
				t.Fatalf("deploy with operator-known host: %v", err)
			}
			calls := read()
			if len(calls) < 2 {
				t.Fatalf("calls = %#v", calls)
			}
			for _, c := range []recordedCall{calls[0], calls[len(calls)-1]} {
				joined := strings.Join(c.args, " ")
				for _, w := range []string{"StrictHostKeyChecking=yes", "GlobalKnownHostsFile=" + DeployKnownHostsPath()} {
					if !strings.Contains(joined, w) {
						t.Errorf("%s argv lacks %q: %s", c.tool, w, joined)
					}
				}
				if strings.Contains(joined, "HostKeyAlias") {
					t.Errorf("%s argv has HostKeyAlias: %s", c.tool, joined)
				}
			}
		})
	}
}
