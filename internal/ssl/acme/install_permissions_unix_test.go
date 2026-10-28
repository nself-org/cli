//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package acme

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestInstallReaderPermissionsBeforeReload(t *testing.T) {
	ssl := t.TempDir()
	cert, key := testPair(t, []string{"api.example.org"}, time.Now().Add(24*time.Hour))
	rel := &fakeReloader{check: func() {
		for _, path := range []string{tgt, tgt + "/privkey.pem", tgt + "/fullchain.pem"} {
			info, err := os.Stat(filepath.Join(ssl, path))
			if err != nil {
				t.Fatal(err)
			}
			mode := os.FileMode(0600)
			if info.IsDir() {
				mode = 0711
			}
			if info.Mode().Perm() != mode {
				t.Errorf("%s: mode %o, want %o", path, info.Mode().Perm(), mode)
			}
			st := info.Sys().(*syscall.Stat_t)
			if int(st.Uid) != os.Getuid() || int(st.Gid) != os.Getgid() {
				t.Errorf("%s: wrong owner", path)
			}
		}
		parent, err := os.Stat(filepath.Join(ssl, "certificates"))
		if err != nil || parent.Mode().Perm()&001 == 0 {
			t.Errorf("parent cannot be traversed: %v", err)
		}
	}}
	_, err := Install(context.Background(), InstallReq{SSLDir: ssl, Targets: []string{tgt}, Cert: cert, Key: key,
		DirMode: 0711, Owner: &FileOwner{UID: os.Getuid(), GID: os.Getgid()}, Reloader: rel})
	if err != nil {
		t.Fatal(err)
	}
}
