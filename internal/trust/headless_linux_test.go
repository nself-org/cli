package trust

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/trust/sysexec"
)

// Linux steps under the fake runner and a temp system root. Nothing executes
// and nothing under /etc is touched.

func TestSetupDNSLinux_HeadlessMakesNoChange(t *testing.T) {
	for _, env := range []string{"NSELF_HEADLESS", "CI"} {
		t.Run(env, func(t *testing.T) {
			fake := hermetic(t)
			t.Setenv(env, "1")
			stub := sysPath("/run/systemd/resolve/stub-resolv.conf")
			if err := os.MkdirAll(filepath.Dir(stub), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(stub, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			already, err := SetupDNSLinux(TrustConfig{BaseDomain: "h.local"})
			if !isSkipped(err) || already {
				t.Fatalf("got (%v, %v), want (false, ErrSkippedHeadless)", already, err)
			}
			if len(fake.Calls()) != 0 {
				t.Errorf("headless run issued commands: %v", fake.Calls())
			}
			if _, statErr := os.Stat(sysPath("/etc/systemd/resolved.conf.d/nself.conf")); !os.IsNotExist(statErr) {
				t.Error("headless run wrote the resolved drop-in")
			}
		})
	}
}

func TestSetupDNSLinux_HeadlessAlreadyConfigured(t *testing.T) {
	hermetic(t)
	t.Setenv("CI", "1")
	for _, p := range []string{"/run/systemd/resolve/stub-resolv.conf", "/etc/systemd/resolved.conf.d/nself.conf"} {
		if err := os.MkdirAll(filepath.Dir(sysPath(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sysPath(p), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	already, err := SetupDNSLinux(TrustConfig{BaseDomain: "h.local"})
	if err != nil || !already {
		t.Fatalf("got (%v, %v), want (true, nil)", already, err)
	}
}

func TestSetupPortsLinux_HeadlessMakesNoChange(t *testing.T) {
	fake := hermetic(t)
	t.Setenv("NSELF_HEADLESS", "1")
	fake.Reply = func(c sysexec.Call) ([]byte, error) {
		if c.Name == "iptables" {
			return nil, errors.New("rule absent") // iptables -C says "not configured"
		}
		return nil, nil
	}
	already, err := SetupPortsLinux(TrustConfig{NginxHTTPPort: 8080, NginxSSLPort: 8443})
	if !isSkipped(err) || already {
		t.Fatalf("got (%v, %v), want (false, ErrSkippedHeadless)", already, err)
	}
	for _, c := range fake.Calls() {
		if sysexec.Privileged(c.Name, c.Args) != "" {
			t.Errorf("headless run issued a system-changing command: %s", c)
		}
	}
}
