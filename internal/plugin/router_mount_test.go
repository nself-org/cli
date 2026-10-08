package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestProxyBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".nself", "plugins", "bin")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "nself-demo")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 3\n"), 0755); err != nil {
		t.Fatal(err)
	}
	var exit *ExitCodeError
	if err := ProxyBinary("nself-demo", "demo", []string{"sub", "--flag", "x"}); !errors.As(err, &exit) || exit.Code != 3 {
		t.Fatalf("exit: %v", err)
	}
	for _, bad := range []string{"../outside", "/bin/sh", "nself-../../bin/sh"} {
		var ce *errs.CLIError
		if err := ProxyBinary(bad, "demo", nil); !errors.As(err, &ce) || ce.Code != "E406" {
			t.Fatalf("%q: %v", bad, err)
		}
	}
	out := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(out, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, path); err != nil {
		t.Fatal(err)
	}
	var ce *errs.CLIError
	if err := ProxyBinary("nself-demo", "demo", nil); !errors.As(err, &ce) || ce.Code != "E406" {
		t.Fatalf("escape: %v", err)
	}
	// The candidate is changed after discovery would have accepted it. The
	// runtime lookup must still refuse the target outside the plugin root.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, path); err != nil {
		t.Fatal(err)
	}
	if err := ProxyBinary("nself-demo", "demo", nil); !errors.As(err, &ce) || ce.Code != "E406" {
		t.Fatalf("post-discovery swap: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	got := filepath.Join(home, "argv.txt")
	t.Setenv("ARGV_PROOF", got)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" \"$2\" > \"$ARGV_PROOF\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	pwned := filepath.Join(home, "pwned")
	args := []string{"$(touch " + pwned + ")", "a; exit 9"}
	if err := ProxyBinary("nself-demo", "demo", args); err != nil {
		t.Fatalf("argv exec: %v", err)
	}
	contents, err := os.ReadFile(got)
	if err != nil || strings.TrimSpace(string(contents)) != strings.Join(args, "\n") {
		t.Fatalf("argv = %q, err = %v", contents, err)
	}
	if _, err := os.Stat(pwned); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("argv was interpreted as a shell command: %v", err)
	}
}
