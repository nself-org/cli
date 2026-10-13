package ssl

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/trust/sysexec"
)

// TestMain makes the package's tests hermetic: commands go to a fake runner
// (mkcert, security, certutil and friends never execute), and /etc/hosts is
// replaced by a temp file. Without this a test that reaches the CA-trust or
// hosts step would prompt for admin rights or edit the real machine.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "nself-ssl-hermetic-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain:", err)
		os.Exit(1)
	}
	hostsFile = filepath.Join(dir, "hosts")
	if err := os.WriteFile(hostsFile, []byte("127.0.0.1\tlocalhost\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "TestMain:", err)
		os.Exit(1)
	}
	// A bare host: every command fails, nothing is trusted, nothing is installed.
	sysexec.Set(&sysexec.Fake{
		Reply: func(c sysexec.Call) ([]byte, error) {
			return nil, errors.New("fake runner: " + c.Name + " unavailable in unit tests")
		},
	})
	_ = os.Unsetenv("NSELF_HEADLESS")
	_ = os.Unsetenv("CI")
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
