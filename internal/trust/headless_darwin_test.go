//go:build darwin

// Package trust — headless_darwin_test.go: the macOS steps under a fake
// runner. Nothing executes: the fake records what the code would have run, so
// each test can assert that headless mode made no system change and that the
// interactive path still issues the admin commands (the positive control).
package trust

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/trust/sysexec"
)

// mutating reports whether the recorded call would change the system.
func mutating(c sysexec.Call) bool { return sysexec.Privileged(c.Name, c.Args) != "" }

func assertNoSystemChange(t *testing.T, f *sysexec.Fake) {
	t.Helper()
	for _, c := range f.Calls() {
		if mutating(c) {
			t.Errorf("headless run issued a system-changing command: %s", c)
		}
	}
}

func setHeadless(t *testing.T, env, val string) {
	t.Helper()
	t.Setenv("NSELF_HEADLESS", "")
	t.Setenv("CI", "")
	t.Setenv(env, val)
}

func TestSetupPortsDarwin_Headless(t *testing.T) {
	for _, env := range []string{"NSELF_HEADLESS", "CI"} {
		t.Run(env, func(t *testing.T) {
			fake := hermetic(t)
			setHeadless(t, env, "1")
			already, err := SetupPortsDarwin(TrustConfig{NginxHTTPPort: 8080, NginxSSLPort: 8443})
			if !isSkipped(err) || already {
				t.Fatalf("got (%v, %v), want (false, ErrSkippedHeadless)", already, err)
			}
			if fake.Saw("osascript") {
				t.Error("headless run called osascript")
			}
			assertNoSystemChange(t, fake)
		})
	}
}

// Check-first contract: headless with the state already in place is success,
// not a skip.
func TestSetupPortsDarwin_HeadlessAlreadyConfigured(t *testing.T) {
	fake := hermetic(t)
	fake.Reply = func(c sysexec.Call) ([]byte, error) {
		if c.Name == "pfctl" {
			return []byte("rdr pass on lo0 inet proto tcp from any to any port 80 -> 127.0.0.1 port 8080\n" +
				"rdr pass on lo0 inet proto tcp from any to any port 443 -> 127.0.0.1 port 8443\n"), nil
		}
		return nil, nil
	}
	setHeadless(t, "NSELF_HEADLESS", "1")
	already, err := SetupPortsDarwin(TrustConfig{NginxHTTPPort: 8080, NginxSSLPort: 8443})
	if err != nil || !already {
		t.Fatalf("got (%v, %v), want (true, nil)", already, err)
	}
}

// Positive control: the same state, not headless, does issue the admin prompt.
func TestSetupPortsDarwin_InteractiveUsesOsascript(t *testing.T) {
	fake := hermetic(t)
	already, err := SetupPortsDarwin(TrustConfig{NginxHTTPPort: 8080, NginxSSLPort: 8443})
	if err != nil || already {
		t.Fatalf("got (%v, %v), want (false, nil)", already, err)
	}
	if !fake.Saw("osascript -e do shell script") || !fake.Saw("with administrator privileges") {
		t.Errorf("interactive path should run the admin osascript, saw %v", fake.Names())
	}
}

func TestSetupPortsDarwin_NSELFHeadlessZeroBeatsCI(t *testing.T) {
	fake := hermetic(t)
	t.Setenv("CI", "1")
	t.Setenv("NSELF_HEADLESS", "0")
	if _, err := SetupPortsDarwin(TrustConfig{NginxHTTPPort: 8080, NginxSSLPort: 8443}); err != nil {
		t.Fatalf("NSELF_HEADLESS=0 must allow the interactive path: %v", err)
	}
	if !fake.Saw("osascript") {
		t.Error("NSELF_HEADLESS=0 inside CI should still reach osascript (via the fake)")
	}
}

func TestSetupDNSDarwin_Headless(t *testing.T) {
	fake := hermetic(t)
	setHeadless(t, "NSELF_HEADLESS", "1")
	dns, resolver, err := SetupDNSDarwin(TrustConfig{BaseDomain: "h.local"})
	if !isSkipped(err) || dns || resolver {
		t.Fatalf("got (%v, %v, %v), want (false, false, ErrSkippedHeadless)", dns, resolver, err)
	}
	for _, bad := range []string{"osascript", "brew install", "brew services", "dscacheutil", "killall"} {
		if fake.Saw(bad) {
			t.Errorf("headless run called %q", bad)
		}
	}
	assertNoSystemChange(t, fake)
	if _, statErr := os.Stat(sysPath(resolverPath)); !os.IsNotExist(statErr) {
		t.Errorf("headless run wrote %s", resolverPath)
	}
}

func TestSetupDNSDarwin_HeadlessAlreadyConfigured(t *testing.T) {
	fake := hermetic(t)
	setHeadless(t, "CI", "1")
	conf := sysPath("/opt/homebrew/etc/dnsmasq.conf")
	if err := os.MkdirAll(filepath.Dir(conf), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conf, []byte(dnsmasqConfLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := sysPath(resolverPath)
	if err := os.MkdirAll(filepath.Dir(res), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(res, []byte(resolverContent), 0o644); err != nil {
		t.Fatal(err)
	}
	dns, resolver, err := SetupDNSDarwin(TrustConfig{BaseDomain: "h.local"})
	if err != nil || !dns || !resolver {
		t.Fatalf("got (%v, %v, %v), want (true, true, nil)", dns, resolver, err)
	}
	if len(fake.Calls()) != 0 {
		t.Errorf("already-configured run should not run any command, saw %v", fake.Names())
	}
}

// Positive control: not headless, the resolver is written through osascript
// (to the fake) and dnsmasq is installed/restarted through brew.
func TestSetupDNSDarwin_InteractiveRunsAdminCommands(t *testing.T) {
	fake := hermetic(t)
	fake.Reply = func(c sysexec.Call) ([]byte, error) {
		if c.String() == "brew list dnsmasq" {
			return nil, os.ErrNotExist // not installed yet
		}
		return nil, nil
	}
	if _, _, err := SetupDNSDarwin(TrustConfig{BaseDomain: "h.local"}); err != nil {
		t.Fatalf("SetupDNSDarwin: %v", err)
	}
	for _, want := range []string{"brew install dnsmasq", "brew services restart dnsmasq", "/etc/resolver/local", "dscacheutil -flushcache"} {
		if !fake.Saw(want) {
			t.Errorf("interactive run should have issued %q, saw %v", want, fake.Calls())
		}
	}
}

func TestSetupMkcert_HeadlessCANotTrusted(t *testing.T) {
	fake := hermetic(t)
	setHeadless(t, "NSELF_HEADLESS", "1")
	already, err := SetupMkcert(TrustConfig{WorkDir: t.TempDir(), BaseDomain: "h.local"})
	if !isSkipped(err) || already {
		t.Fatalf("got (%v, %v), want (false, ErrSkippedHeadless)", already, err)
	}
	for _, bad := range []string{"osascript", "add-trusted-cert", "brew install", "-install"} {
		if fake.Saw(bad) {
			t.Errorf("headless run called %q", bad)
		}
	}
	assertNoSystemChange(t, fake)
}

func TestSetupMkcert_HeadlessMkcertMissing(t *testing.T) {
	fake := hermetic(t)
	fake.Missing = map[string]bool{"mkcert": true}
	setHeadless(t, "CI", "1")
	_, err := SetupMkcert(TrustConfig{WorkDir: t.TempDir(), BaseDomain: "h.local"})
	if !isSkipped(err) {
		t.Fatalf("want ErrSkippedHeadless, got %v", err)
	}
	if fake.Saw("brew") {
		t.Error("headless run must not brew install mkcert")
	}
}

// Positive control: interactive, the CA is trusted through the admin
// osascript (to the fake), never the real one.
func TestSetupMkcert_InteractiveTrustsCAViaOsascript(t *testing.T) {
	fake := hermetic(t)
	caRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(caRoot, "rootCA.pem"), []byte("not a real cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake.Reply = func(c sysexec.Call) ([]byte, error) {
		switch {
		case c.String() == "mkcert -CAROOT":
			return []byte(caRoot + "\n"), nil
		case c.Name == "security" && len(c.Args) > 0 && c.Args[0] == "verify-cert":
			return nil, os.ErrPermission // CA not trusted yet
		}
		return nil, nil
	}
	if _, err := SetupMkcert(TrustConfig{WorkDir: t.TempDir(), BaseDomain: "h.local"}); err != nil {
		t.Fatalf("SetupMkcert: %v", err)
	}
	if !fake.Saw("osascript -e do shell script \"security add-trusted-cert") {
		t.Errorf("CA trust should go through the admin osascript, saw %v", fake.Calls())
	}
	if !fake.Saw("mkcert -cert-file") {
		t.Errorf("certs should be generated through mkcert, saw %v", fake.Calls())
	}
}

func TestSetupResolver_WritesViaOsascriptFake(t *testing.T) {
	fake := hermetic(t)
	if err := setupResolver(); err != nil {
		t.Fatalf("setupResolver: %v", err)
	}
	if !fake.Saw("osascript") || !fake.Saw("/etc/resolver/local") || !fake.Saw("administrator privileges") {
		t.Errorf("setupResolver should run the admin osascript, saw %v", fake.Calls())
	}
	if strings.Contains(strings.Join(fake.Names(), " "), "sudo") {
		t.Error("setupResolver must not use sudo")
	}
}

func TestWritePfAnchorDirect_TempRootOnly(t *testing.T) {
	hermetic(t)
	if err := writePfAnchorDirect(TrustConfig{NginxHTTPPort: 8080, NginxSSLPort: 8443}); err != nil {
		t.Fatalf("writePfAnchorDirect: %v", err)
	}
	data, err := os.ReadFile(sysPath(pfAnchorPath))
	if err != nil || !strings.Contains(string(data), "port 8443") {
		t.Errorf("anchor not written under the temp root: %q %v", data, err)
	}
}
