package ssl

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/sysexec"
)

// hermeticSSL installs a recording fake runner, a temp hosts file and a clean
// headless environment for one test.
func hermeticSSL(t *testing.T) (*sysexec.Fake, string) {
	t.Helper()
	t.Setenv("NSELF_HEADLESS", "")
	t.Setenv("CI", "")
	fake := &sysexec.Fake{}
	t.Cleanup(sysexec.Set(fake))
	prev := hostsFile
	hostsFile = filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(hostsFile, []byte("127.0.0.1\tlocalhost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { hostsFile = prev })
	return fake, hostsFile
}

func TestAddHosts_HeadlessMakesNoChange(t *testing.T) {
	for _, env := range []string{"NSELF_HEADLESS", "CI"} {
		t.Run(env, func(t *testing.T) {
			_, path := hermeticSSL(t)
			t.Setenv(env, "1")
			n, err := AddHosts([]string{"app.local.test"})
			if !errors.Is(err, sysexec.ErrSkippedHeadless) || n != 0 {
				t.Fatalf("got (%d, %v), want (0, ErrSkippedHeadless)", n, err)
			}
			data, _ := os.ReadFile(path)
			if string(data) != "127.0.0.1\tlocalhost\n" {
				t.Errorf("headless run edited the hosts file: %q", data)
			}
		})
	}
}

// Positive control: interactive, the same call writes the (temp) hosts file.
func TestAddHosts_InteractiveWritesTempFile(t *testing.T) {
	_, path := hermeticSSL(t)
	n, err := AddHosts([]string{"app.local.test"})
	if err != nil || n != 1 {
		t.Fatalf("got (%d, %v), want (1, nil)", n, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "app.local.test\t# nself-managed") {
		t.Errorf("entry missing: %q", data)
	}
}

func TestAddHosts_HeadlessAlreadyPresentIsSuccess(t *testing.T) {
	_, path := hermeticSSL(t)
	if err := os.WriteFile(path, []byte("127.0.0.1\tapp.local.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_HEADLESS", "1")
	n, err := AddHosts([]string{"app.local.test"})
	if err != nil || n != 0 {
		t.Fatalf("got (%d, %v), want (0, nil)", n, err)
	}
}

func TestInstallCA_HeadlessNeverTrustsTheCA(t *testing.T) {
	fake, _ := hermeticSSL(t)
	ca := filepath.Join(t.TempDir(), "rootCA.pem")
	if err := os.WriteFile(ca, []byte("not a real cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake.Reply = func(c sysexec.Call) ([]byte, error) {
		if c.Name == "security" || c.Name == "update-ca-certificates" || c.Name == "certutil" {
			return nil, errors.New("not trusted")
		}
		return nil, nil
	}
	t.Setenv("CI", "1")
	if err := installCA(ca); !errors.Is(err, ErrSudoRequired) {
		t.Fatalf("headless installCA = %v, want ErrSudoRequired (manual command fallback)", err)
	}
	for _, c := range fake.Calls() {
		if sysexec.Privileged(c.Name, c.Args) != "" {
			t.Errorf("headless installCA issued a system-changing command: %s", c)
		}
	}
}
