//go:build !windows

package acme

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestACMEInstallOwnerBeforeReload(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing uid requires root")
	}
	dir, _ := fixture(t)
	cert, key := testPair(t, []string{"api.example.org"}, time.Now().Add(time.Hour))
	req := InstallReq{SSLDir: dir, Targets: []string{tgt}, Cert: cert, Key: key,
		Access: &InstallAccess{DirMode: 0o711, ParentMode: 0o751, UID: 1001, GID: 1002}}
	req.Reloader = &fakeReloader{check: func() {
		for _, generation := range []string{tgt, "certificates/.api-example-org.gen-0"} {
			for _, name := range []string{"", "privkey.pem", "fullchain.pem", "cert.pem"} {
				path := filepath.Join(dir, generation, name)
				fi, err := os.Stat(path)
				if os.IsNotExist(err) && name == "cert.pem" {
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				st := fi.Sys().(*syscall.Stat_t)
				if st.Uid != 1001 || st.Gid != 1002 {
					t.Errorf("%s owner = %d:%d", path, st.Uid, st.Gid)
				}
			}
		}
	}}
	if _, err := Install(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}

func TestACMEInstallAccessFailureDoesNotSwitch(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can assign arbitrary groups")
	}
	dir, original := fixture(t)
	cert, key := testPair(t, []string{"api.example.org"}, time.Now().Add(time.Hour))
	rel := &fakeReloader{}
	_, err := Install(context.Background(), InstallReq{SSLDir: dir, Targets: []string{tgt}, Cert: cert, Key: key,
		Access: &InstallAccess{DirMode: 0o711, ParentMode: 0o751, UID: os.Getuid(), GID: 1 << 30}, Reloader: rel})
	if err == nil || rel.calls != 0 {
		t.Fatalf("err=%v reloads=%d", err, rel.calls)
	}
	if string(served(t, dir, tgt)) != string(original) {
		t.Fatal("failed access policy switched the certificate")
	}
}
