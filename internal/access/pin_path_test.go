package access

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestSSHPinPathWithSpace(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	home := filepath.Join(t.TempDir(), "a b")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	args, err := (&SSHTransport{Host: "host.test"}).sshArgs()
	if err != nil {
		t.Fatal(err)
	}
	want := `GlobalKnownHostsFile="` + filepath.ToSlash(filepath.Join(home, ".config", "nself", "deploy_known_hosts")) + `"`
	for _, arg := range args {
		if arg == want {
			return
		}
	}
	t.Fatalf("SSH argv %q lacks quoted pin file %q", args, want)
}

func TestSSHPinPathRefusesUnsafeHOME(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX path characters")
	}
	t.Setenv("NSELF_V15", "1")
	for _, name := range []string{`a"b`, `a\b`, "a\nb", "a\tb"} {
		t.Setenv("HOME", filepath.Join(t.TempDir(), name))
		if _, err := (&SSHTransport{Host: "host.test"}).sshArgs(); err == nil {
			t.Fatalf("accepted unsafe HOME %q", name)
		}
	}
}
