//go:build unix

package licensing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestEntitlementFIFORejectedWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entitlement.jwt")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(FileEnv, path)
	if err := VerifyEntitlement("paid", time.Now()); !errors.Is(err, ErrEntitlementExpired) {
		t.Fatalf("FIFO returned %v", err)
	}
	g := Guard(context.Background(), "paid", handler())
	defer g.Close()
	if serve(g, "/app").Code != 503 {
		t.Fatal("FIFO authorized")
	}
}

// A FIFO or symlink swapped in right before the open must be rejected without
// blocking the reader.
func TestEntitlementSwapRaceRejected(t *testing.T) {
	for _, kind := range []string{"fifo", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "entitlement.jwt")
			target := filepath.Join(dir, "target.jwt")
			for _, p := range []string{path, target} {
				if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			beforeTokenOpen = func() {
				_ = os.Remove(path)
				if kind == "fifo" {
					_ = syscall.Mkfifo(path, 0600)
				} else {
					_ = os.Symlink(target, path)
				}
			}
			defer func() { beforeTokenOpen = nil }()
			done := make(chan error, 1)
			go func() { _, err := readTokenFile(path); done <- err }()
			select {
			case err := <-done:
				if !errors.Is(err, ErrEntitlementExpired) {
					t.Fatalf("swapped %s returned %v", kind, err)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("swapped %s blocked the reader", kind)
			}
		})
	}
}
