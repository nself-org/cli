package deploy

// Purpose: tests for the shared ssh option builder and for the refusal of
//          hostile host, key path, remote path and env name inputs.
// Inputs:  fake ssh, scp, rsync and ssh-keyscan scripts first on PATH.
// Outputs: asserts argv shape and that refused inputs start no process.
// Constraints: no network; only the fake binaries run.

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// installFakeKeyscan puts a fake ssh-keyscan first on PATH. It prints one
// ed25519 line for the last argument (the host), key blob "AQID", and logs one
// line per run; the returned func reports how many runs happened.
func installFakeKeyscan(t *testing.T) func() int {
	t.Helper()
	if runtime.GOOS == "windows" {
		// A POSIX stub cannot run there, and the real ssh-keyscan would reach the network.
		t.Skip("fake ssh-keyscan is a POSIX shell stub")
	}
	dir := t.TempDir()
	logFile := filepath.Join(dir, "scans.log")
	if err := os.WriteFile(logFile, nil, 0o644); err != nil {
		t.Fatalf("create scan log: %v", err)
	}
	script := "#!/bin/sh\necho scan >> \"" + logFile + "\"\nfor a in \"$@\"; do h=\"$a\"; done\nprintf '%s ssh-ed25519 AQID\\n' \"$h\"\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh-keyscan"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ssh-keyscan: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() int {
		raw, err := os.ReadFile(logFile)
		if err != nil {
			t.Fatalf("read scan log: %v", err)
		}
		return strings.Count(string(raw), "scan\n")
	}
}

func mustSpec(t *testing.T, s string) remote.HostSpec {
	t.Helper()
	spec, err := remote.ParseHostSpec(s)
	if err != nil {
		t.Fatalf("ParseHostSpec(%q): %v", s, err)
	}
	return spec
}

func TestSSHTransportBuilder(t *testing.T) {
	policy := []string{"-o", "StrictHostKeyChecking=yes", "-o", "GlobalKnownHostsFile=/h/pin"}
	t.Run("empty key omits -i", func(t *testing.T) {
		tr, err := newSSHTransport(mustSpec(t, "u@203.0.113.7"), "", policy)
		if err != nil {
			t.Fatal(err)
		}
		want := append(append([]string{}, policy...), "-o", "ForwardAgent=no")
		if !reflect.DeepEqual(tr.opts, want) {
			t.Errorf("opts = %q, want %q", tr.opts, want)
		}
	})
	t.Run("key then policy then ForwardAgent", func(t *testing.T) {
		tr, err := newSSHTransport(mustSpec(t, "u@203.0.113.7"), "/k/id", policy)
		if err != nil {
			t.Fatal(err)
		}
		want := append(append([]string{"-i", "/k/id"}, policy...), "-o", "ForwardAgent=no")
		if !reflect.DeepEqual(tr.opts, want) {
			t.Errorf("opts = %q, want %q", tr.opts, want)
		}
	})
	t.Run("port in ssh, rsync -e and scp", func(t *testing.T) {
		tr, err := newSSHTransport(mustSpec(t, "u@203.0.113.7:2222"), "/k/id", nil)
		if err != nil {
			t.Fatal(err)
		}
		sa := tr.sshArgv("true")
		if got := strings.Join(sa, " "); got != "-i /k/id -o ForwardAgent=no -p 2222 -- u@203.0.113.7 true" {
			t.Errorf("sshArgv = %q", got)
		}
		ra, err := tr.rsyncArgv("/work/c.yml", "/opt/c.yml")
		if err != nil {
			t.Fatal(err)
		}
		wantE := "ssh -i /k/id -o ForwardAgent=no -p 2222"
		wantR := []string{"-az", "-e", wantE, "--", "/work/c.yml", "u@203.0.113.7:/opt/c.yml"}
		if !reflect.DeepEqual(ra, wantR) {
			t.Errorf("rsyncArgv = %q, want %q", ra, wantR)
		}
		sc, err := tr.scpArgv("/work/e", "/opt/e")
		if err != nil {
			t.Fatal(err)
		}
		wantS := []string{"-i", "/k/id", "-o", "ForwardAgent=no", "-P", "2222", "--", "/work/e", "u@203.0.113.7:/opt/e"}
		if !reflect.DeepEqual(sc, wantS) {
			t.Errorf("scpArgv = %q, want %q", sc, wantS)
		}
	})
	t.Run("IPv6 operand is bracketed", func(t *testing.T) {
		tr, err := newSSHTransport(mustSpec(t, "u@[2001:db8::1]:2222"), "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := tr.operand("/opt/x"); got != "u@[2001:db8::1]:/opt/x" {
			t.Errorf("operand = %q", got)
		}
	})
	t.Run("rsync -e equals ssh plus tokens joined by one space", func(t *testing.T) {
		tr, err := newSSHTransport(mustSpec(t, "u@203.0.113.7:2222"), "/k/id", policy)
		if err != nil {
			t.Fatal(err)
		}
		ra, err := tr.rsyncArgv("/a", "/b")
		if err != nil {
			t.Fatal(err)
		}
		toks := append(append([]string{}, tr.opts...), tr.spec.SSHOptions()...)
		if ra[2] != "ssh "+strings.Join(toks, " ") {
			t.Errorf("-e = %q", ra[2])
		}
		if got := strings.Fields(ra[2]); len(got) != len(toks)+1 {
			t.Errorf("-e re-splits into %d fields, want %d", len(got), len(toks)+1)
		}
	})
	for name, bad := range map[string]string{
		"space": "StrictHostKeyChecking=yes -oProxyCommand=x", "quote": "A='b'", "dquote": `A="b"`,
		"backslash": `A=b\c`, "backtick": "A=`id`", "newline": "A=b\nc", "empty": "",
	} {
		bad := bad
		t.Run("refuses "+name, func(t *testing.T) {
			if _, err := newSSHTransport(mustSpec(t, "u@203.0.113.7"), "", []string{"-o", bad}); err == nil {
				t.Errorf("token %q accepted", bad)
			}
		})
	}
	t.Run("refuses bad key paths", func(t *testing.T) {
		for _, k := range []string{"/k/id -oProxyCommand=x", "-F/tmp/x", "/k/i'd"} {
			if _, err := newSSHTransport(mustSpec(t, "u@203.0.113.7"), k, nil); err == nil {
				t.Errorf("key %q accepted", k)
			}
		}
	})
	t.Run("local operand is absolute", func(t *testing.T) {
		got, err := localOperand("host:x.env")
		if err != nil || !filepath.IsAbs(got) || !strings.HasSuffix(got, "host:x.env") {
			t.Errorf("localOperand = %q, %v", got, err)
		}
		if got, err := localOperand("/work/e"); err != nil || got != "/work/e" {
			t.Errorf("absolute path changed: %q, %v", got, err)
		}
		for _, bad := range []string{"", "a\nb", "a\x00b"} {
			if _, err := localOperand(bad); err == nil {
				t.Errorf("localOperand(%q) accepted", bad)
			}
		}
	})
}

func TestDeployArgvHostileInputs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NSELF_V15", "")
	read := installFakeSSHTools(t)
	scans := installFakeKeyscan(t)
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env.prod")
	if err := os.WriteFile(envFile, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deploy := map[string]SSHConfig{
		"host semicolon":      {Host: "u@h;id", KeyPath: "/k/id"},
		"host option":         {Host: "u@-oProxyCommand=x", KeyPath: "/k/id"},
		"legacy path subst":   {Host: "u@h:/opt/x$(id)", KeyPath: "/k/id"},
		"key with option":     {Host: "u@h.example", KeyPath: "/k/id -oProxyCommand=/tmp/x"},
		"key dash F":          {Host: "u@h.example", KeyPath: "-F/tmp/x"},
		"path dotdot":         {Host: "u@h.example", RemotePath: "/opt/../etc", KeyPath: "/k/id"},
		"path semicolon":      {Host: "u@h.example", RemotePath: "/opt/x;id", KeyPath: "/k/id"},
		"path backtick":       {Host: "u@h.example", RemotePath: "/opt/`id`", KeyPath: "/k/id"},
		"path leading dash":   {Host: "u@h.example", RemotePath: "-x", KeyPath: "/k/id"},
		"env name with slash": {Host: "u@h.example", RemotePath: "/opt/x", EnvName: "a/b", EnvFile: envFile, KeyPath: "/k/id"},
	}
	for name, cfg := range deploy {
		err := DeployViaSsh(context.Background(), cfg, "/work/compose.yml")
		if err == nil {
			t.Errorf("DeployViaSsh %s: expected an error", name)
		} else if strings.Contains(err.Error(), "E487") {
			t.Errorf("DeployViaSsh %s: refused only at the host-key stage: %v", name, err)
		}
	}
	// A bad compose path must be refused before the env file is sent and before any scan.
	cpErr := DeployViaSsh(context.Background(), SSHConfig{Host: "u@h.example", RemotePath: "/opt/x", EnvName: "live",
		EnvFile: envFile, KeyPath: "/k/id"}, "/w/a\nb.yml")
	if cpErr == nil {
		t.Error("DeployViaSsh: newline in compose path accepted")
	}
	push := map[string]struct {
		cfg  SSHConfig
		path string
	}{
		"host semicolon":  {SSHConfig{Host: "u@h;id", KeyPath: "/k/id"}, "/opt/x"},
		"host option":     {SSHConfig{Host: "u@-oProxyCommand=x", KeyPath: "/k/id"}, "/opt/x"},
		"key with option": {SSHConfig{Host: "u@h.example", KeyPath: "/k/id -oProxyCommand=/tmp/x"}, "/opt/x"},
		"key dash F":      {SSHConfig{Host: "u@h.example", KeyPath: "-F/tmp/x"}, "/opt/x"},
		"path dotdot":     {SSHConfig{Host: "u@h.example", KeyPath: "/k/id"}, "/opt/../etc"},
		"path semicolon":  {SSHConfig{Host: "u@h.example", KeyPath: "/k/id"}, "/opt/x;id"},
		"path backtick":   {SSHConfig{Host: "u@h.example", KeyPath: "/k/id"}, "/opt/`id`"},
		"path dash":       {SSHConfig{Host: "u@h.example", KeyPath: "/k/id"}, "-x"},
	}
	for name, c := range push {
		err := PushSecrets(context.Background(), c.cfg, PushSecretsOptions{Target: "prod", EnvFile: envFile, RemotePath: c.path})
		if err == nil {
			t.Errorf("PushSecrets %s: expected an error", name)
		} else if strings.Contains(err.Error(), "E487") {
			t.Errorf("PushSecrets %s: refused only at the host-key stage: %v", name, err)
		}
	}
	if calls := read(); len(calls) != 0 {
		t.Fatalf("refused inputs started %d process(es): %#v", len(calls), calls)
	}
	if n := scans(); n != 0 {
		t.Fatalf("refused inputs ran ssh-keyscan %d time(s)", n)
	}
}
