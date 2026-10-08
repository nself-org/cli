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
	// A file can change after discovery. The accepted check-to-exec window
	// belongs to the plugins directory owner; an outside symlink is refused
	// when this call checks it immediately before exec.
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

func TestProxyFallbackEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := filepath.Join(home, ".nself", "plugins", "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(out, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, filepath.Join(bin, "nself-demo")); err != nil {
		t.Fatal(err)
	}
	var ce *errs.CLIError
	if err := ProxyCommandWithHint("demo", nil, ""); !errors.As(err, &ce) || ce.Code != "E406" {
		t.Fatalf("fallback escape: %v", err)
	}
}

func TestProxyBinaryIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	pluginDir := t.TempDir()
	bin := filepath.Join(pluginDir, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bin, "nself-demo")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s' \"$0\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })
	if err := ProxyBinaryAt(path, pluginDir, "demo", nil); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	if string(buf[:n]) != path {
		t.Fatalf("argv0=%q want %q", buf[:n], path)
	}
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".mount-") {
			t.Fatalf("stale hard link: %s", e.Name())
		}
	}
}
