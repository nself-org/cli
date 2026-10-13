package testwatch

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestMain runs the package normally, and under a 300 ms watchdog when the test
// binary is re-executed as the child of the subprocess tests below.
func TestMain(m *testing.M) {
	if os.Getenv("TESTWATCH_CHILD") == "1" {
		Main(m, 300*time.Millisecond)
		return
	}
	Main(m, time.Minute)
}

func TestChildHang(t *testing.T) {
	if os.Getenv("TESTWATCH_CHILD") != "1" {
		t.Skip("child of TestHangPastLimitCrashesWithDump")
	}
	time.Sleep(10 * time.Second)
}

func TestChildQuick(t *testing.T) {
	if os.Getenv("TESTWATCH_CHILD") != "1" {
		t.Skip("child of TestFinishingPackageExitsZero")
	}
}

func runChild(t *testing.T, run string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+run+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), "TESTWATCH_CHILD=1")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestHangPastLimitCrashesWithDump(t *testing.T) {
	out, err := runChild(t, "TestChildHang")
	if err == nil {
		t.Fatalf("a hung package must fail; output:\n%s", out)
	}
	for _, want := range []string{"testwatch: the package did not finish within 300ms", "goroutine "} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%.600s", want, out)
		}
	}
}

func TestFinishingPackageExitsZero(t *testing.T) {
	if out, err := runChild(t, "TestChildQuick"); err != nil {
		t.Fatalf("a package that finishes must exit 0: %v\n%s", err, out)
	}
}
