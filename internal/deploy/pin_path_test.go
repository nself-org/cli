package deploy

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

func TestDeployPinPathWithSpace(t *testing.T) {
	home := filepath.Join(t.TempDir(), "a b")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// Exercise the production policy's option at the transport boundary too.
	option, err := knownHostsOption()
	if err != nil {
		t.Fatal(err)
	}
	want := `GlobalKnownHostsFile="` + filepath.ToSlash(DeployKnownHostsPath()) + `"`
	if option != want {
		t.Fatalf("pin option = %q, want %q", option, want)
	}
	if _, err := newSSHTransport(mustSpec(t, "host.test"), "", []string{"-o", option}); err != nil {
		t.Fatal(err)
	}
}

func TestDeployRefusesUnsafePinPathBeforeScan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell stub and path characters")
	}
	t.Setenv("NSELF_V15", "1")
	scans := installFakeKeyscan(t)
	for _, name := range []string{`a"b`, `a\b`, "a\nb", "a\tb"} {
		t.Setenv("HOME", filepath.Join(t.TempDir(), name))
		if _, err := HostKeyPolicy(context.Background(), "host.test", "live", "app", true, false); err == nil {
			t.Fatalf("accepted unsafe HOME %q", name)
		}
	}
	if scans() != 0 {
		t.Fatal("scanned host keys before refusing unquotable pin path")
	}
}

func TestRsyncPreservesQuotedPinPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell stub")
	}
	rsync, err := exec.LookPath("rsync")
	if err != nil {
		t.Skip("rsync not available")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "argv")
	stub := filepath.Join(dir, "ssh")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$PIN_ARGV_LOG\"\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PIN_ARGV_LOG", log)
	source := filepath.Join(dir, "source")
	if err := os.WriteFile(source, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	list := filepath.Join(dir, "list")
	if err := os.WriteFile(list, []byte("source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a b", "a'b", "a`b", "a$b"} {
		t.Run(name, func(t *testing.T) {
			option, err := remote.GlobalKnownHostsFileOption(filepath.Join(dir, name, "pins"))
			if err != nil {
				t.Fatal(err)
			}
			transport, err := newSSHTransport(mustSpec(t, "host.test"), "", []string{"-o", option})
			if err != nil {
				t.Fatal(err)
			}
			legacy, err := transport.rsyncArgv(source, "/remote")
			if err != nil {
				t.Fatal(err)
			}
			inventory, err := rsyncArgv(transport.spec, transport.opts, nil, list, dir, "/remote")
			if err != nil {
				t.Fatal(err)
			}
			// The stub deliberately refuses the rsync protocol after recording
			// argv. This invokes rsync's actual -e parser without a connection.
			for _, args := range [][]string{legacy, inventory} {
				if err := os.Remove(log); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				_ = exec.Command(rsync, args...).Run()
				data, err := os.ReadFile(log)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(data), "\n"+option+"\n") {
					t.Fatalf("rsync changed pin option: %q, want %q", data, option)
				}
			}
		})
	}
}

func TestTransportRefusesNoncanonicalPinOptions(t *testing.T) {
	for _, option := range []string{
		`GlobalKnownHostsFile="relative"`,
		`GlobalKnownHostsFile="/a" "/planted"`,
		`GlobalKnownHostsFile="/a\b"`,
		"GlobalKnownHostsFile=\"/a\nb\"",
		`GlobalKnownHostsFile="/a" -oProxyCommand=evil`,
	} {
		if err := checkTransportToken(option); err == nil {
			t.Fatalf("accepted %q", option)
		}
	}
}
