package pathmap

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestWindowsDrivePath(t *testing.T) {
	for p, want := range map[string]bool{`C:\k`: true, "d:/k": true, "C:k": false, "/k": false, "": false, "1:/k": false} {
		if got := WindowsDrivePath(p); got != want {
			t.Errorf("WindowsDrivePath(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestSSHKeyPath(t *testing.T) {
	if got := SSHKeyPath(`C:\keys\id`); got != "C:/keys/id" {
		t.Errorf("SSHKeyPath = %q", got)
	}
	if got := SSHKeyPath("/home/a/.ssh/id"); got != "/home/a/.ssh/id" {
		t.Errorf("SSHKeyPath = %q", got)
	}
}

func TestLocalOperand(t *testing.T) {
	if got, err := LocalOperand("/work/e"); err != nil || got != "/work/e" {
		t.Errorf("LocalOperand abs = %q, %v", got, err)
	}
	if got, err := LocalOperand("host:x.env"); err != nil || !filepath.IsAbs(got) {
		t.Errorf("LocalOperand rel = %q, %v", got, err)
	}
	for _, bad := range []string{"", "a\nb", "a\x00b"} {
		if _, err := LocalOperand(bad); err == nil {
			t.Errorf("LocalOperand(%q) accepted", bad)
		}
		if _, err := RsyncLocalOperand(bad); err == nil {
			t.Errorf("RsyncLocalOperand(%q) accepted", bad)
		}
	}
	if runtime.GOOS != "windows" {
		if got, err := RsyncLocalOperand("/work/src/"); err != nil || got != "/work/src/" {
			t.Errorf("RsyncLocalOperand posix = %q, %v", got, err)
		}
	}
}
