package trust

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/nself-org/cli/internal/trust/sysexec"
)

// TestMain makes every test in this package hermetic by default: a fake
// command runner (nothing executes), a temp system root (nothing under /etc,
// /Library or /opt is read or written), and a clean headless environment.
// Tests that need to script command output or inspect calls take their own
// fake with hermetic(t).
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "nself-trust-sysroot-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain:", err)
		os.Exit(1)
	}
	sysRoot = root
	sysexec.Set(bareHost())
	_ = os.Unsetenv("NSELF_HEADLESS")
	_ = os.Unsetenv("CI")
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}

// bareHost is a recording fake on which every command fails and nothing is
// installed or trusted, like a fresh CI box. Tests that need a command to
// succeed script it through Reply.
func bareHost() *sysexec.Fake {
	return &sysexec.Fake{Reply: func(c sysexec.Call) ([]byte, error) {
		return nil, errors.New("fake runner: " + c.Name + " unavailable")
	}}
}

// hermetic gives the test its own recording fake runner, its own temp system
// root and a clean headless environment (NSELF_HEADLESS and CI unset), and
// restores everything on cleanup. Nothing the test runs can reach the machine.
func hermetic(t *testing.T) *sysexec.Fake {
	t.Helper()
	t.Setenv("NSELF_HEADLESS", "")
	t.Setenv("CI", "")
	fake := bareHost()
	restore := sysexec.Set(fake)
	prevRoot := sysRoot
	sysRoot = t.TempDir()
	t.Cleanup(func() {
		sysRoot = prevRoot
		restore()
	})
	return fake
}
