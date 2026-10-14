//go:build unix

package licensing

import (
	"context"
	"errors"
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
