package controlplane

// Tests for P7-DEPL-29: a run that ships an env file never lets one of its own
// probes enrol the key the strict check trusts (D-0248, DEPL-28 MUST-FIX 2).

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// shippingRunFixture builds a one-server remote env, fake ssh tools that log
// their argv, and a temp HOME. It returns the inventory, compose and env paths,
// the tool dir and the host.
func shippingRunFixture(t *testing.T) (inv *Inventory, compose, envFile, bin, host string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX command stubs")
	}
	t.Setenv("NSELF_V15", "0")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := t.TempDir()
	host = "u@host.example"
	srv := Server{Name: "web", Role: RoleApp, Host: host, SSHKeyRef: "NSELF_SSH_KEY_SHIP", RemotePath: "/opt/nself", Primary: true}
	inv = &Inventory{SchemaVersion: 2, Environments: map[string]Environment{
		"local": {Name: "local", Kind: "local", Tier: TierLocal},
		"qa":    {Name: "qa", Kind: "remote", Tier: TierLocalServers, Servers: []Server{srv}},
	}}
	if err := Write(root, inv); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	bin = t.TempDir()
	log := filepath.Join(bin, "calls.log")
	for name, body := range map[string]string{
		"ssh-keyscan": "#!/bin/sh\nprintf 'host.example ssh-ed25519 AQID\\n'\n",
		"ssh-keygen":  "#!/bin/sh\nif [ -f \"$4\" ]; then /bin/cat \"$4\"; fi\n",
		"ssh":         "#!/bin/sh\necho \"ssh $*\" >> '" + log + "'\nexit 0\n",
		"rsync":       "#!/bin/sh\necho \"rsync $*\" >> '" + log + "'\nexit 0\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	key := filepath.Join(bin, "id_test")
	if err := os.WriteFile(key, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_SSH_KEY_SHIP", key)
	compose = filepath.Join(root, "compose.yml")
	envFile = filepath.Join(root, ".env.snapshot")
	for p, body := range map[string]string{compose: "services: {}\n", envFile: "FAKE_VALUE=not-a-secret\n"} {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return inv, compose, envFile, bin, host
}

// TestRunWithEnvProbeCannotSeedTrust is the DEPL-28 adversarial repro. A real
// SSHProber runs the pipeline of a shipping run in v1.4: an unknown host fails
// with E487 before any ssh or rsync call (so no accept-new probe can write
// ~/.ssh/known_hosts), and a pinned host is reached only with strict argv.
func TestRunWithEnvProbeCannotSeedTrust(t *testing.T) {
	t.Run("unknown host", func(t *testing.T) {
		inv, compose, envFile, bin, _ := shippingRunFixture(t)
		_, err := RunWithEnv(context.Background(), inv, "qa", NewSSHProber(filepath.Dir(compose), false), compose, envFile, "qa")
		if err == nil || !strings.Contains(err.Error(), "E487") {
			t.Fatalf("want E487 before any probe, got %v", err)
		}
		if b, _ := os.ReadFile(filepath.Join(bin, "calls.log")); len(b) != 0 {
			t.Fatalf("a probe or copy ran before the host key check:\n%s", b)
		}
		home, _ := os.UserHomeDir()
		if _, err := os.Stat(filepath.Join(home, ".ssh", "known_hosts")); !os.IsNotExist(err) {
			t.Fatalf("~/.ssh/known_hosts was touched: %v", err)
		}
	})
	t.Run("key changes at the first probe", func(t *testing.T) {
		inv, compose, envFile, bin, host := shippingRunFixture(t)
		fp, err := remote.Fingerprint("host.example ssh-ed25519 AQID")
		if err != nil {
			t.Fatal(err)
		}
		if err := TrustHostKey(context.Background(), host, fp); err != nil {
			t.Fatal(err)
		}
		// The first scan (the pre-check) shows the pinned key, every later one another key.
		count := filepath.Join(bin, "scans")
		script := "#!/bin/sh\necho x >> '" + count + "'\nif [ \"$(wc -l < '" + count + "')\" -le 1 ]; then k=AQID; else k=AQIE; fi\nprintf 'host.example ssh-ed25519 %s\\n' \"$k\"\n"
		if err := os.WriteFile(filepath.Join(bin, "ssh-keyscan"), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
		_, err = RunWithEnv(context.Background(), inv, "qa", NewSSHProber(filepath.Dir(compose), false), compose, envFile, "qa")
		if err == nil || !strings.Contains(err.Error(), "E487") || !strings.Contains(err.Error(), "unknown host key") {
			t.Fatalf("want the mid-run E487 abort, got %v", err)
		}
		if b, _ := os.ReadFile(filepath.Join(bin, "calls.log")); strings.Contains(string(b), "rsync") {
			t.Fatalf("a copy ran after the key changed:\n%s", b)
		}
	})
	t.Run("key changes at the docker probe", func(t *testing.T) {
		inv, compose, envFile, bin, host := shippingRunFixture(t)
		fp, err := remote.Fingerprint("host.example ssh-ed25519 AQID")
		if err != nil {
			t.Fatal(err)
		}
		if err := TrustHostKey(context.Background(), host, fp); err != nil {
			t.Fatal(err)
		}
		// The pre-check and the reachability probe see the pinned key, the docker probe another one.
		count := filepath.Join(bin, "scans")
		script := "#!/bin/sh\necho x >> '" + count + "'\nif [ \"$(wc -l < '" + count + "')\" -le 2 ]; then k=AQID; else k=AQIE; fi\nprintf 'host.example ssh-ed25519 %s\\n' \"$k\"\n"
		if err := os.WriteFile(filepath.Join(bin, "ssh-keyscan"), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
		_, err = RunWithEnv(context.Background(), inv, "qa", NewSSHProber(filepath.Dir(compose), false), compose, envFile, "qa")
		if err == nil || !strings.Contains(err.Error(), "E487") || !strings.Contains(err.Error(), "unknown host key") {
			t.Fatalf("want the mid-run E487 abort, got %v", err)
		}
		if b, _ := os.ReadFile(filepath.Join(bin, "calls.log")); strings.Contains(string(b), "rsync") {
			t.Fatalf("a copy ran after the key changed:\n%s", b)
		}
	})
	t.Run("pin-only host", func(t *testing.T) {
		inv, compose, envFile, bin, host := shippingRunFixture(t)
		fp, err := remote.Fingerprint("host.example ssh-ed25519 AQID")
		if err != nil {
			t.Fatal(err)
		}
		if err := TrustHostKey(context.Background(), host, fp); err != nil {
			t.Fatal(err)
		}
		res, err := RunWithEnv(context.Background(), inv, "qa", NewSSHProber(filepath.Dir(compose), false), compose, envFile, "qa")
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if len(res.Servers) != 1 || res.Servers[0].Status != "ok" {
			t.Fatalf("result = %+v", res.Servers)
		}
		b, _ := os.ReadFile(filepath.Join(bin, "calls.log"))
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		sawSSH := false
		for _, l := range lines {
			if strings.HasPrefix(l, "ssh ") {
				sawSSH = true
			}
			if strings.Contains(l, "accept-new") || !strings.Contains(l, "StrictHostKeyChecking=yes") {
				t.Errorf("connection is not strict: %s", l)
			}
		}
		if !sawSSH || len(lines) < 3 {
			t.Fatalf("too few connections for the assertion to mean anything:\n%s", b)
		}
	})
}

// TestMarkShippingOverlappingRuns: two shipping runs on one prober; the first to
// finish must not clear the mode of the one still running. A nil ctx is safe.
func TestMarkShippingOverlappingRuns(t *testing.T) {
	sp := NewSSHProber(t.TempDir(), false)
	endA := markShipping(sp)
	endB := markShipping(sp)
	endA()
	if !sp.shipping.on() {
		t.Fatal("run A ending cleared run B's shipping mode")
	}
	endB()
	if sp.shipping.on() {
		t.Fatal("shipping mode stayed on after every run ended")
	}
	markShipping(struct{}{})()                                      // a non-SSHProber is a no-op
	if !isShippingRun(WithShippingRun(nil)) || isShippingRun(nil) { //nolint:staticcheck // callers do pass nil
		t.Fatal("nil ctx handling")
	}
}
