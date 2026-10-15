package acme

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestACMEInstallAccessBeforeReload(t *testing.T) {
	posixOnly(t)
	for _, custom := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "caller"}[custom], func(t *testing.T) {
			dir, _ := fixture(t)
			cert, key := testPair(t, []string{"api.example.org"}, time.Now().Add(time.Hour))
			req := InstallReq{SSLDir: dir, Targets: []string{tgt}, Cert: cert, Key: key}
			mode := os.FileMode(0o700)
			if custom {
				mode = 0o711
				req.Access = &InstallAccess{DirMode: mode, ParentMode: 0o751, UID: os.Getuid(), GID: os.Getgid()}
			}
			req.Reloader = &fakeReloader{check: func() {
				paths := []string{filepath.Join(dir, tgt)}
				if custom {
					paths = append(paths, filepath.Join(dir, "certificates/.api-example-org.gen-0"))
				}
				for _, path := range paths {
					fi, err := os.Stat(path)
					if err != nil {
						t.Fatal(err)
					}
					if fi.Mode().Perm() != mode {
						t.Errorf("%s mode = %o, want %o", path, fi.Mode().Perm(), mode)
					}
					keyInfo, err := os.Stat(filepath.Join(path, "privkey.pem"))
					if err != nil {
						t.Fatal(err)
					}
					if keyInfo.Mode().Perm() != 0o600 {
						t.Error("private key permissions widened")
					}
				}
				if custom {
					fi, err := os.Stat(filepath.Join(dir, "certificates"))
					if err != nil {
						t.Fatal(err)
					}
					if fi.Mode().Perm() != 0o751 {
						t.Errorf("parent mode = %o", fi.Mode().Perm())
					}
				}
			}}
			if _, err := Install(context.Background(), req); err != nil {
				t.Fatal(err)
			}
		})
	}
}
