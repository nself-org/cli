package admin

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// argvEnv is a fake-ssh sandbox: HOME and PATH point at temp dirs, a fake ssh
// records its argv (one word per line) and a fake ssh-keyscan leaves a marker.
type argvEnv struct {
	home, bin string
}

func (e argvEnv) argvPath() string { return filepath.Join(e.bin, "argv") }
func (e argvEnv) scanMark() string { return filepath.Join(e.bin, "keyscan-called") }

// argv returns the recorded ssh argv words, failing if ssh was never run.
func (e argvEnv) argv(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(e.argvPath())
	if err != nil {
		t.Fatalf("ssh was not run: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// newArgvEnv builds the sandbox. When trust is true it writes a throwaway host
// key for [host.test]:2222 into known_hosts and makes ssh-keyscan return it.
func newArgvEnv(t *testing.T, trust bool) argvEnv {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX SSH recorder")
	}
	e := argvEnv{home: t.TempDir(), bin: t.TempDir()}
	t.Setenv("HOME", e.home)
	scan := "#!/bin/sh\n: > '" + e.scanMark() + "'\n"
	if trust {
		key := filepath.Join(e.bin, "hostkey")
		if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
			t.Fatalf("keygen: %v %s", err, out)
		}
		pub, err := os.ReadFile(key + ".pub")
		if err != nil {
			t.Fatal(err)
		}
		f := strings.Fields(string(pub))
		line := "[host.test]:2222 " + f[0] + " " + f[1] + "\n"
		if err := os.MkdirAll(filepath.Join(e.home, ".ssh"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(e.home, ".ssh", "known_hosts"), []byte(line), 0600); err != nil {
			t.Fatal(err)
		}
		scan += "printf '%s' '" + line + "'\n"
	}
	if err := os.WriteFile(filepath.Join(e.bin, "ssh-keyscan"), []byte(scan), 0700); err != nil {
		t.Fatal(err)
	}
	ssh := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + e.argvPath() + "'\n"
	if err := os.WriteFile(filepath.Join(e.bin, "ssh"), []byte(ssh), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", e.bin+":"+os.Getenv("PATH"))
	return e
}

// runEntry runs one admin entry point against host.test:2222 as user u.
func runEntry(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	switch name {
	case "verify":
		if err := VerifySSHKey(ctx, "u", "host.test", 2222); err != nil {
			t.Fatal(err)
		}
	case "ensure":
		if err := EnsureRemoteAdmin(ctx, "u", "host.test", 2222); err != nil {
			t.Fatal(err)
		}
	case "tunnel":
		cmd, err := OpenTunnel(ctx, ConnectOpts{User: "u", Host: "host.test", SSHPort: 2222, LocalPort: 18080, RemotePort: 8080})
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown entry point %q", name)
	}
}

var hostKeyFlags = []string{"StrictHostKeyChecking", "GlobalKnownHostsFile", "HostKeyAlias", "accept-new"}

// TestAdminSSHArgvV14Exact pins the exact v1.4 (v1.4 mode) argv of all
// three entry points: no host-key option is added, the policy stays with the
// operator's ssh_config (ADR 0021).
func TestAdminSSHArgvV14Exact(t *testing.T) {
	cases := []struct {
		name string
		want []string
	}{
		{"verify", []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=30", "-p", "2222", "--", "u@host.test", "true"}},
		{"ensure", []string{"-o", "ServerAliveInterval=30", "-p", "2222", "--", "u@host.test", "systemctl --user start nself-admin || true"}},
		{"tunnel", []string{"-N", "-o", "ServerAliveInterval=30", "-o", "ExitOnForwardFailure=yes", "-L", "18080:127.0.0.1:8080", "-p", "2222", "--", "u@host.test"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, false)
			e := newArgvEnv(t, false)
			runEntry(t, tc.name)
			if got := e.argv(t); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("v1.4 argv mismatch\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// TestAdminSSHV14NoKeyscanNoHostKeyPolicy pins that v1.4 mode never scans host
// keys and never injects a host-key option (in particular never accept-new).
func TestAdminSSHV14NoKeyscanNoHostKeyPolicy(t *testing.T) {
	for _, name := range []string{"verify", "ensure", "tunnel"} {
		t.Run(name, func(t *testing.T) {
			compattest.Set(t, false)
			e := newArgvEnv(t, false)
			runEntry(t, name)
			if _, err := os.Stat(e.scanMark()); err == nil {
				t.Fatal("ssh-keyscan ran in v1.4 mode")
			}
			joined := strings.Join(e.argv(t), " ")
			for _, f := range hostKeyFlags {
				if strings.Contains(joined, f) {
					t.Fatalf("v1.4 argv carries %s: %s", f, joined)
				}
			}
		})
	}
}

// TestAdminSSHV15TrustedKeyExact pins the exact v1.5 (v1.5 mode) VerifySSHKey
// argv for a known_hosts-trusted key, and that accept-new never appears.
func TestAdminSSHV15TrustedKeyExact(t *testing.T) {
	compattest.Set(t, true)
	e := newArgvEnv(t, true)
	runEntry(t, "verify")
	want := []string{
		"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=30",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "GlobalKnownHostsFile=" + filepath.Join(e.home, ".config", "nself", "deploy_known_hosts"),
		"-p", "2222", "--", "u@host.test", "true",
	}
	got := e.argv(t)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("v1.5 argv mismatch\n got: %q\nwant: %q", got, want)
	}
	if strings.Contains(strings.Join(got, " "), "accept-new") {
		t.Fatal("accept-new in v1.5 argv")
	}
}

// TestAdminSSHArgsRejectsHostileInput pins that VerifySSHKey refuses option
// injection, shell metacharacters, whitespace and user/port conflicts in both
// modes, at validation: ssh and ssh-keyscan are never started and the error is
// not the E487 host-key error.
func TestAdminSSHArgsRejectsHostileInput(t *testing.T) {
	cases := []struct {
		name       string
		user, host string
		port       int
	}{
		{"option injection host", "u", "-oProxyCommand=touch_x", 2222},
		{"shell metachar host", "u", "host.test;id", 2222},
		{"space in host", "u", "host.test extra", 2222},
		{"option injection user", "-oX", "host.test", 2222},
		{"user conflict", "u", "other@host.test", 2222},
		{"port conflict", "u", "host.test:2200", 2222},
	}
	compattest.Both(t, func(t *testing.T) {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				e := newArgvEnv(t, false)
				err := VerifySSHKey(context.Background(), tc.user, tc.host, tc.port)
				if err == nil {
					t.Fatal("accepted hostile input")
				}
				if strings.Contains(err.Error(), "E487") {
					t.Fatalf("rejected by host-key scan, not validation: %v", err)
				}
				if _, statErr := os.Stat(e.argvPath()); statErr == nil {
					t.Fatal("ssh ran despite rejected input")
				}
				if _, statErr := os.Stat(e.scanMark()); statErr == nil {
					t.Fatal("ssh-keyscan ran before validation rejected input")
				}
			})
		}
	})
}
