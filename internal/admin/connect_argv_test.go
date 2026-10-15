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

// hostKey places a throwaway key under alias: in ~/.ssh/known_hosts, or in the
// nSelf pin file when pin is true. The fake ssh-keyscan returns the same key.
type hostKey struct {
	alias string
	pin   bool
}

func (e argvEnv) argvPath() string { return filepath.Join(e.bin, "argv") }
func (e argvEnv) scanMark() string { return filepath.Join(e.bin, "keyscan-called") }
func (e argvEnv) pinFile() string {
	return filepath.Join(e.home, ".config", "nself", "deploy_known_hosts")
}

// argv returns the recorded ssh argv words, failing if ssh was never run.
func (e argvEnv) argv(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(e.argvPath())
	if err != nil {
		t.Fatalf("ssh was not run: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// mustExist / mustNotExist assert a marker file (keyscan or ssh ran or not).
func mustExist(t *testing.T, path, why string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s: %v", why, err)
	}
}

func mustNotExist(t *testing.T, path, why string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatal(why)
	}
}

// newArgvEnv builds the sandbox. home "" means a fresh temp dir. A non-nil key
// is written to the trust store it names and returned by the fake ssh-keyscan.
func newArgvEnv(t *testing.T, home string, key *hostKey) argvEnv {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX SSH recorder")
	}
	if home == "" {
		home = t.TempDir()
	}
	e := argvEnv{home: home, bin: t.TempDir()}
	t.Setenv("HOME", e.home)
	scan := "#!/bin/sh\n: > '" + e.scanMark() + "'\n"
	if key != nil {
		if _, err := exec.LookPath("ssh-keygen"); err != nil {
			t.Skip("ssh-keygen not on PATH")
		}
		kf := filepath.Join(e.bin, "hostkey")
		if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", kf).CombinedOutput(); err != nil {
			t.Fatalf("keygen: %v %s", err, out)
		}
		pub, err := os.ReadFile(kf + ".pub")
		if err != nil {
			t.Fatal(err)
		}
		f := strings.Fields(string(pub))
		line := key.alias + " " + f[0] + " " + f[1] + "\n"
		store := filepath.Join(e.home, ".ssh", "known_hosts")
		if key.pin {
			store = e.pinFile()
		}
		if err := os.MkdirAll(filepath.Dir(store), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(store, []byte(line), 0600); err != nil {
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

// callEntry runs one admin entry point (verify, ensure or tunnel) and waits for
// the tunnel's ssh to exit. It returns the first error.
func callEntry(name, user, host string, port int) error {
	ctx := context.Background()
	switch name {
	case "verify":
		return VerifySSHKey(ctx, user, host, port)
	case "ensure":
		return EnsureRemoteAdmin(ctx, user, host, port)
	default:
		cmd, err := OpenTunnel(ctx, ConnectOpts{User: user, Host: host, SSHPort: port, LocalPort: 18080, RemotePort: 8080})
		if err != nil {
			return err
		}
		return cmd.Wait()
	}
}

var entryNames = []string{"verify", "ensure", "tunnel"}

// TestAdminSSHArgvV14Exact pins the exact v1.4 argv of all three entry points
// for a DNS name and a bracketed IPv6 literal: no host-key option is added, the
// policy stays with the operator's ssh_config (ADR 0021).
func TestAdminSSHArgvV14Exact(t *testing.T) {
	wants := map[string]func(dest string) []string{
		"verify": func(d string) []string {
			return []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=30", "-p", "2222", "--", d, "true"}
		},
		"ensure": func(d string) []string {
			return []string{"-o", "ServerAliveInterval=30", "-p", "2222", "--", d, "systemctl --user start nself-admin || true"}
		},
		"tunnel": func(d string) []string {
			return []string{"-N", "-o", "ServerAliveInterval=30", "-o", "ExitOnForwardFailure=yes", "-L", "18080:127.0.0.1:8080", "-p", "2222", "--", d}
		},
	}
	hosts := []struct{ spec, dest string }{
		{"host.test", "u@host.test"},
		{"[2001:db8::1]", "u@2001:db8::1"},
	}
	for _, h := range hosts {
		for _, name := range entryNames {
			t.Run(name+"/"+h.spec, func(t *testing.T) {
				compattest.Set(t, false)
				e := newArgvEnv(t, "", nil)
				if err := callEntry(name, "u", h.spec, 2222); err != nil {
					t.Fatal(err)
				}
				if got, want := e.argv(t), wants[name](h.dest); !reflect.DeepEqual(got, want) {
					t.Fatalf("v1.4 argv mismatch\n got: %q\nwant: %q", got, want)
				}
			})
		}
	}
}

// TestAdminSSHV14NoKeyscanNoHostKeyPolicy pins that v1.4 mode never scans host
// keys and never injects a host-key option (in particular never accept-new).
func TestAdminSSHV14NoKeyscanNoHostKeyPolicy(t *testing.T) {
	flags := []string{"StrictHostKeyChecking", "GlobalKnownHostsFile", "HostKeyAlias", "accept-new"}
	for _, name := range entryNames {
		t.Run(name, func(t *testing.T) {
			compattest.Set(t, false)
			e := newArgvEnv(t, "", nil)
			if err := callEntry(name, "u", "host.test", 2222); err != nil {
				t.Fatal(err)
			}
			mustNotExist(t, e.scanMark(), "ssh-keyscan ran in v1.4 mode")
			joined := strings.Join(e.argv(t), " ")
			for _, f := range flags {
				if strings.Contains(joined, f) {
					t.Fatalf("v1.4 argv carries %s: %s", f, joined)
				}
			}
		})
	}
}

// TestAdminSSHV15TrustedKeyExact pins the exact v1.5 VerifySSHKey argv for a
// key trusted through the operator's known_hosts: strict checking and the
// disabled nSelf pin file, no alias, no accept-new. The keyscan marker is the
// positive control for the v1.4 absence checks.
func TestAdminSSHV15TrustedKeyExact(t *testing.T) {
	compattest.Set(t, true)
	e := newArgvEnv(t, "", &hostKey{alias: "[host.test]:2222"})
	if err := callEntry("verify", "u", "host.test", 2222); err != nil {
		t.Fatal(err)
	}
	mustExist(t, e.scanMark(), "ssh-keyscan must run in v1.5 mode")
	want := []string{
		"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=30",
		"-o", "StrictHostKeyChecking=yes",
		"-o", `GlobalKnownHostsFile="/dev/null"`,
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

// TestAdminSSHArgsRejectsHostileInput pins that all three entry points refuse
// option injection, shell metacharacters, whitespace, user/port conflicts and
// bad ports in both modes, at validation: ssh and ssh-keyscan are never started
// and the error is not the E487 host-key error.
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
		{"negative port", "u", "host.test", -1},
		{"port above 65535", "u", "host.test", 70000},
		// ParseHostSpec refuses an unbracketed IPv6 literal ("needs brackets").
		{"unbracketed ipv6 with port", "u", "2001:db8::1:2222", 2222},
		// The legacy host:/path form cannot carry a port.
		{"legacy path with port", "u", "h:/etc", 2222},
	}
	compattest.Both(t, func(t *testing.T) {
		for _, name := range entryNames {
			for _, tc := range cases {
				t.Run(name+"/"+tc.name, func(t *testing.T) {
					e := newArgvEnv(t, "", nil)
					err := callEntry(name, tc.user, tc.host, tc.port)
					if err == nil {
						t.Fatal("accepted hostile input")
					}
					if strings.Contains(err.Error(), "E487") {
						t.Fatalf("rejected by host-key scan, not validation: %v", err)
					}
					mustNotExist(t, e.argvPath(), "ssh ran despite rejected input")
					mustNotExist(t, e.scanMark(), "ssh-keyscan ran before validation rejected input")
				})
			}
		}
	})
}
