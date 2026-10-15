package commands

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type failingKeyTemp struct {
	*os.File
	failWrite bool
	failure   error
}

func (f failingKeyTemp) WriteString(s string) (int, error) {
	if f.failWrite {
		return 0, f.failure
	}
	return f.File.WriteString(s)
}
func (f failingKeyTemp) Sync() error { return f.failure }

func TestD0435JWTStageFailureDoesNotLog(t *testing.T) {
	for _, write := range []bool{true, false} {
		t.Run(map[bool]string{true: "write", false: "sync"}[write], func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "rotation.log")
			t.Setenv("NSELF_JWT_ROTATION_LOG", log)
			target := filepath.Join(dir, "jwt.key")
			if err := os.WriteFile(target, []byte("previous\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			kf, err := openKeyFile(target)
			if err != nil {
				t.Fatal(err)
			}
			defer kf.discard()
			tmp := kf.tmp.(*os.File)
			failure := errors.New("disk full")
			kf.tmp = failingKeyTemp{File: tmp, failWrite: write, failure: failure}
			if _, err := rotateJWTKeyFile("old-key", kf); !errors.Is(err, failure) {
				t.Fatalf("want staging error, got %v", err)
			}
			if _, err := os.Stat(log); !os.IsNotExist(err) {
				t.Fatalf("staging failure updated log: %v", err)
			}
			got, err := os.ReadFile(target)
			if err != nil || string(got) != "previous\n" {
				t.Fatalf("target changed: %q %v", got, err)
			}
		})
	}
}
