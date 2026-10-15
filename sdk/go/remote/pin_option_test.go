package remote

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGlobalKnownHostsFileOption(t *testing.T) {
	for _, path := range []string{"pins", "a b/pins", "a'b/pins", filepath.Join(t.TempDir(), "pins")} {
		got, err := GlobalKnownHostsFileOption(path)
		if err != nil {
			t.Fatal(err)
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			t.Fatal(err)
		}
		if want := `GlobalKnownHostsFile="` + filepath.ToSlash(abs) + `"`; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
	bad := []string{"", `a"b`, "a\nb", "a\tb", "a\x00b", "a\x7fb", "a\u0085b"}
	if runtime.GOOS != "windows" {
		bad = append(bad, `a\b`)
	}
	for _, path := range bad {
		if _, err := GlobalKnownHostsFileOption(path); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
}

func TestGlobalKnownHostsFileOptionRefusesUnsafeCWD(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX filename with a quote")
	}
	dir := filepath.Join(t.TempDir(), `unsafe"cwd`)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if _, err := GlobalKnownHostsFileOption("pins"); err == nil {
		t.Fatal("relative pin file under unquotable cwd accepted")
	}
}
