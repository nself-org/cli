package deploy

// Purpose: tests for P7-DEPL-29: secret shipping is strict in v1.4 and v1.5,
//          accept-new never writes the operator's known_hosts in v1.5, and the
//          key scan falls back from ed25519 to ecdsa and rsa.
// Inputs:  HOME in a temp dir, fake ssh-keyscan, ssh, scp, rsync on PATH.
// Outputs: asserts E487, argv, file bytes and the keyscan call count.
// Constraints: no network, no real host; every test sets NSELF_V15 itself.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// installTypedKeyscan puts a fake ssh-keyscan first on PATH that answers per
// algorithm: keys maps "ed25519" to "ssh-ed25519 AQID" and so on; an absent
// type prints nothing. failAll makes every call exit 1 (a connect error). The
// returned func lists the -t values asked, in order.
func installTypedKeyscan(t *testing.T, keys map[string]string, failAll bool, missRC int, bannerFD int) func() []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake ssh-keyscan is a POSIX shell stub")
	}
	dir := t.TempDir()
	logFile := filepath.Join(dir, "types.log")
	if err := os.WriteFile(logFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for typ, line := range keys {
		if err := os.WriteFile(filepath.Join(dir, "key-"+typ), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\nt=''; p=''\nfor a in \"$@\"; do [ \"$p\" = '-t' ] && t=\"$a\"; p=\"$a\"; h=\"$a\"; done\n" +
		"echo \"$t\" >> '" + logFile + "'\n"
	if failAll {
		script += "exit 1\n"
	} else {
		// Like OpenSSH: a host with no key of the asked type still gets its banner
		// line on stdout; OpenSSH 10 then exits 1, older releases exit 0 (missRC).
		script += "if [ -f '" + dir + "/key-'\"$t\" ]; then printf '%s %s\\n' \"$h\" \"$(cat '" + dir + "/key-'\"$t\")\"; else printf '# %s SSH-2.0-Fake\\n' \"$h\" >&" + strconv.Itoa(bannerFD) + "; exit " + strconv.Itoa(missRC) + "; fi\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "ssh-keyscan"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() []string {
		raw, err := os.ReadFile(logFile)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Fields(string(raw))
	}
}

func envShipCfg(t *testing.T, host string) SSHConfig {
	t.Helper()
	return SSHConfig{Host: host, RemotePath: "/opt/nself", EnvName: "live", ServerName: "web", EnvFile: writeEnvFile(t), KeyPath: "/k/id"}
}

// TestDeployViaSshEnvFileStrictV14: shipping an env file is strict in v1.4 and
// v1.5 (E487 before any rsync); compose-only keeps accept-new in v1.4.
func TestDeployViaSshEnvFileStrictV14(t *testing.T) {
	requireKeygen(t)
	for _, v15 := range []string{"", "1"} {
		t.Run("env file NSELF_V15="+v15, func(t *testing.T) {
			t.Setenv("NSELF_V15", v15)
			t.Setenv("HOME", pinHome(t))
			read := installFakeSSHTools(t)
			installFakeKeyscan(t)
			cfg := envShipCfg(t, "u@203.0.113.7")
			err := DeployViaSsh(context.Background(), cfg, "/work/compose.yml")
			if err == nil || !strings.Contains(err.Error(), "E487") || !strings.Contains(err.Error(), "--trust-host-key SHA256:") {
				t.Fatalf("unknown host: %v", err)
			}
			if n := len(read()); n != 0 {
				t.Fatalf("%d process(es) ran before the host-key refusal", n)
			}
			if err := (remote.PinnedHostKeys{Path: DeployKnownHostsPath()}).Add("nself-203.0.113.7-22", "ssh-ed25519 AQID"); err != nil {
				t.Fatal(err)
			}
			if err := DeployViaSsh(context.Background(), cfg, "/work/compose.yml"); err != nil {
				t.Fatalf("pinned deploy: %v", err)
			}
			calls := read()
			if len(calls) < 2 || calls[0].tool != "rsync" {
				t.Fatalf("calls = %#v", calls)
			}
			for _, c := range calls {
				joined := strings.Join(c.args, " ")
				if !strings.Contains(joined, "StrictHostKeyChecking=yes") || strings.Contains(joined, "accept-new") {
					t.Errorf("%s argv is not strict: %s", c.tool, joined)
				}
			}
		})
	}
	t.Run("compose-only v1.4 keeps accept-new", func(t *testing.T) {
		t.Setenv("NSELF_V15", "")
		t.Setenv("HOME", pinHome(t))
		read := installFakeSSHTools(t)
		scans := installFakeKeyscan(t)
		cfg := envShipCfg(t, "u@203.0.113.7")
		cfg.EnvFile = ""
		if err := DeployViaSsh(context.Background(), cfg, "/work/compose.yml"); err != nil {
			t.Fatal(err)
		}
		if got := read()[0].args; len(got) < 3 || got[2] != "ssh "+strings.Join(goldenBase("/k/id"), " ") {
			t.Errorf("argv = %q", got)
		}
		if n := scans(); n != 0 {
			t.Errorf("ssh-keyscan ran %d time(s)", n)
		}
	})
}

// TestHostKeyPolicyShipsSecrets: shipsSecrets forces the strict check in both
// modes even when strict is false; without it v1.4 stays accept-new.
func TestHostKeyPolicyShipsSecrets(t *testing.T) {
	requireKeygen(t)
	for _, v15 := range []string{"", "1"} {
		t.Run("NSELF_V15="+v15, func(t *testing.T) {
			t.Setenv("NSELF_V15", v15)
			t.Setenv("HOME", pinHome(t))
			installFakeKeyscan(t)
			ctx := context.Background()
			_, err := HostKeyPolicy(ctx, "u@203.0.113.7", "live", "web", false, true)
			if err == nil || !strings.Contains(err.Error(), "E487") {
				t.Fatalf("unknown host, shipsSecrets: %v", err)
			}
			if v15 == "" {
				got, err := HostKeyPolicy(ctx, "u@203.0.113.7", "live", "web", true, false)
				if err != nil || !reflect.DeepEqual(got, []string{"-o", "StrictHostKeyChecking=accept-new"}) {
					t.Fatalf("v1.4 without shipsSecrets: %q, %v", got, err)
				}
			}
			if err := (remote.PinnedHostKeys{Path: DeployKnownHostsPath()}).Add("nself-203.0.113.7-22", "ssh-ed25519 AQID"); err != nil {
				t.Fatal(err)
			}
			got, err := HostKeyPolicy(ctx, "u@203.0.113.7", "live", "web", false, true)
			if err != nil || !strings.Contains(strings.Join(got, " "), "StrictHostKeyChecking=yes") {
				t.Fatalf("pinned host: %q, %v", got, err)
			}
		})
	}
}

// TestScanDeployKeysFallbackTypes: ed25519 first, then ecdsa, then rsa, one
// call per try and only after an empty answer; a connect error stops at once.
func TestScanDeployKeysFallbackTypes(t *testing.T) {
	spec := mustSpec(t, "u@203.0.113.7")
	rsaLine := "ssh-rsa AQID"
	cases := []struct {
		name     string
		keys     map[string]string
		fail     bool
		missRC   int
		bannerFD int
		wantAsk  []string
		wantType string
		wantErr  bool
	}{
		{"ed25519 host: one call", map[string]string{"ed25519": "ssh-ed25519 AQID"}, false, 1, 1, []string{"ed25519"}, "ssh-ed25519", false},
		{"ecdsa-only host", map[string]string{"ecdsa": "ecdsa-sha2-nistp256 AQID"}, false, 1, 1, []string{"ed25519", "ecdsa"}, "ecdsa-sha2-nistp256", false},
		{"rsa-only host", map[string]string{"rsa": rsaLine}, false, 1, 1, []string{"ed25519", "ecdsa", "rsa"}, "ssh-rsa", false},
		{"rsa-only host, keyscan exits 0 on a miss", map[string]string{"rsa": rsaLine}, false, 0, 1, []string{"ed25519", "ecdsa", "rsa"}, "ssh-rsa", false},
		{"rsa-only host, banner on stderr (other OpenSSH releases)", map[string]string{"rsa": rsaLine}, false, 1, 2, []string{"ed25519", "ecdsa", "rsa"}, "ssh-rsa", false},
		{"no key at all", nil, false, 1, 1, []string{"ed25519", "ecdsa", "rsa"}, "", true},
		{"connect error stops at once", map[string]string{"rsa": rsaLine}, true, 1, 1, []string{"ed25519"}, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			asked := installTypedKeyscan(t, c.keys, c.fail, c.missRC, c.bannerFD)
			keys, err := ScanDeployKeys(context.Background(), spec)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if got := asked(); !reflect.DeepEqual(got, c.wantAsk) {
				t.Errorf("keyscan -t sequence = %v, want %v", got, c.wantAsk)
			}
			if !c.wantErr && (len(keys) != 1 || keys[0].Type != c.wantType) {
				t.Errorf("keys = %#v, want one of type %s", keys, c.wantType)
			}
		})
	}
	t.Run("rsa-only host can be pinned and then passes the strict check", func(t *testing.T) {
		requireKeygen(t)
		t.Setenv("NSELF_V15", "")
		t.Setenv("HOME", pinHome(t))
		installTypedKeyscan(t, map[string]string{"rsa": rsaLine}, false, 1, 1)
		ctx := context.Background()
		if _, err := HostKeyPolicy(ctx, "u@203.0.113.7", "live", "web", true, true); err == nil || !strings.Contains(err.Error(), "E487") {
			t.Fatalf("unpinned rsa host: %v", err)
		}
		fp, err := remote.Fingerprint("h " + rsaLine)
		if err != nil {
			t.Fatal(err)
		}
		if err := TrustHostKey(ctx, "u@203.0.113.7", fp); err != nil {
			t.Fatalf("TrustHostKey: %v", err)
		}
		if _, err := HostKeyPolicy(ctx, "u@203.0.113.7", "live", "web", true, true); err != nil {
			t.Fatalf("pinned rsa host: %v", err)
		}
	})
}

// TestAcceptNewNeverWritesOperatorKnownHostsV15: in v1.5 an accept-new
// connection reads the operator file as the global file and writes first-seen
// keys to nself's own file; the strict check never trusts that file.
func TestAcceptNewNeverWritesOperatorKnownHostsV15(t *testing.T) {
	requireKeygen(t)
	t.Setenv("NSELF_V15", "1")
	home := pinHome(t)
	t.Setenv("HOME", home)
	installFakeKeyscan(t)
	operator := filepath.Join(home, ".ssh", "known_hosts")
	if err := os.MkdirAll(filepath.Dir(operator), 0o700); err != nil {
		t.Fatal(err)
	}
	const operatorBody = "other.example ssh-ed25519 AQID\n"
	if err := os.WriteFile(operator, []byte(operatorBody), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	opts, err := HostKeyPolicy(ctx, "u@host.example", "qa", "web", false, false)
	if err != nil {
		t.Fatal(err)
	}
	tofu := filepath.ToSlash(DeployTofuHostsPath())
	want := []string{"-o", "StrictHostKeyChecking=accept-new", "-o", "UserKnownHostsFile=" + tofu, "-o", "GlobalKnownHostsFile=" + filepath.ToSlash(operator)}
	if !reflect.DeepEqual(opts, want) {
		t.Fatalf("opts = %q, want %q", opts, want)
	}
	for _, o := range opts {
		if o != "-o" {
			if err := checkTransportToken(o); err != nil {
				t.Errorf("token %q: %v", o, err)
			}
		}
	}
	if runtime.GOOS != "windows" {
		for p, perm := range map[string]os.FileMode{filepath.Dir(DeployTofuHostsPath()): 0o700, DeployTofuHostsPath(): 0o600} {
			if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != perm {
				t.Errorf("%s: %v, %v (want %v)", p, fi, err, perm)
			}
		}
	}
	if got, _ := os.ReadFile(operator); string(got) != operatorBody {
		t.Errorf("operator file changed: %q", got)
	}
	// A key that only the TOFU file holds never satisfies the strict check.
	if err := os.WriteFile(DeployTofuHostsPath(), []byte("203.0.113.7 ssh-ed25519 AQID\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := HostKeyPolicy(ctx, "u@203.0.113.7", "qa", "web", true, true); err == nil || !strings.Contains(err.Error(), "E487") {
		t.Fatalf("a key present only in the TOFU file passed the strict check: %v", err)
	}
	// A home path that cannot go into one ssh option keeps plain accept-new and warns once.
	spaced := filepath.Join(t.TempDir(), "a b")
	if err := os.MkdirAll(spaced, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", spaced)
	var warn bytes.Buffer
	oldWarn := tofuWarn
	tofuWarn, tofuWarnOnce = &warn, sync.Once{}
	defer func() { tofuWarn, tofuWarnOnce = oldWarn, sync.Once{} }()
	opts, err = HostKeyPolicy(ctx, "u@host.example", "qa", "web", false, false)
	if err != nil || !reflect.DeepEqual(opts, []string{"-o", "StrictHostKeyChecking=accept-new"}) {
		t.Fatalf("spaced home: %q, %v", opts, err)
	}
	if strings.Count(warn.String(), "\n") != 1 {
		t.Errorf("warning = %q, want one line", warn.String())
	}
	// v1.4: byte for byte accept-new, no file created.
	t.Setenv("NSELF_V15", "")
	t.Setenv("HOME", home)
	_ = os.Remove(DeployTofuHostsPath())
	opts, err = HostKeyPolicy(ctx, "u@host.example", "qa", "web", false, false)
	if err != nil || !reflect.DeepEqual(opts, []string{"-o", "StrictHostKeyChecking=accept-new"}) {
		t.Fatalf("v1.4: %q, %v", opts, err)
	}
	if _, err := os.Stat(DeployTofuHostsPath()); err == nil {
		t.Error("v1.4 accept-new created the TOFU file")
	}
}

// TestAcceptNewTofuFileUnavailableRecordsNothingV15: when nself's own file cannot
// be created, an accept-new connection must not fall back to ssh's default file
// (the operator's known_hosts): it records nothing and still reads the operator
// file for a changed-key check.
func TestAcceptNewTofuFileUnavailableRecordsNothingV15(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// The TOFU path is a directory, so it cannot be opened as a file.
	if err := os.MkdirAll(DeployTofuHostsPath(), 0o700); err != nil {
		t.Fatal(err)
	}
	var warn bytes.Buffer
	oldWarn := tofuWarn
	tofuWarn, tofuWarnOnce = &warn, sync.Once{}
	defer func() { tofuWarn, tofuWarnOnce = oldWarn, sync.Once{} }()
	opts := acceptNewPolicy()
	operator := filepath.ToSlash(filepath.Join(home, ".ssh", "known_hosts"))
	want := []string{"-o", "StrictHostKeyChecking=accept-new", "-o", "UserKnownHostsFile=" + filepath.ToSlash(os.DevNull), "-o", "GlobalKnownHostsFile=" + operator}
	if !reflect.DeepEqual(opts, want) {
		t.Fatalf("opts = %q, want %q", opts, want)
	}
	if strings.Count(warn.String(), "\n") != 1 {
		t.Errorf("warning = %q, want one line", warn.String())
	}
}

// TestStrictPolicyNeverSharesConnections: every strict policy carries
// ControlMaster=no and ControlPath=none, in v1.4 and v1.5. A client that joins an
// existing master socket does no host-key check, and an accept-new connection
// (a probe) may have opened that master.
func TestStrictPolicyNeverSharesConnections(t *testing.T) {
	requireKeygen(t)
	for _, v15 := range []string{"", "1"} {
		t.Run("v15="+v15, func(t *testing.T) {
			t.Setenv("NSELF_V15", v15)
			t.Setenv("HOME", pinHome(t))
			installFakeKeyscan(t)
			if err := (remote.PinnedHostKeys{Path: DeployKnownHostsPath()}).Add("nself-203.0.113.7-22", "ssh-ed25519 AQID"); err != nil {
				t.Fatal(err)
			}
			opts, err := HostKeyPolicy(context.Background(), "u@203.0.113.7", "qa", "web", true, true)
			if err != nil {
				t.Fatal(err)
			}
			joined := " " + strings.Join(opts, " ") + " "
			for _, want := range []string{" StrictHostKeyChecking=yes ", " ControlMaster=no ", " ControlPath=none "} {
				if !strings.Contains(joined, want) {
					t.Errorf("strict policy lacks%s: %q", want, opts)
				}
			}
		})
	}
}
