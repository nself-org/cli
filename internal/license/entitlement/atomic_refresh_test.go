package entitlement

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUnmountedRefreshReplacesInode(t *testing.T) {
	previous := entitlementMounted
	entitlementMounted = func(string) bool { return false }
	defer func() { entitlementMounted = previous }()
	path := filepath.Join(t.TempDir(), "entitlement.jwt")
	if err := writeAtomic(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	if err := writeAtomic(path, []byte("second")); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if os.SameFile(before, after) {
		t.Fatal("unmounted refresh rewrote the live inode")
	}
}

func TestConcurrentEntitlementWritesSerialize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entitlement.jwt")
	if err := writeAtomic(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(filepath.Join(filepath.Dir(path), ".entitlement-write.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := lockRejected(lock); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- writeAtomic(path, []byte("second")) }()
	select {
	case err := <-done:
		t.Fatalf("writer bypassed held publication lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	_ = lock.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("writer did not resume after publication lock released")
	}
}

func TestMountProbePolicy(t *testing.T) {
	mountProbeMu.RLock()
	previous := mountProbe
	mountProbeMu.RUnlock()
	defer SetMountProbe(previous)
	for _, tc := range []struct {
		name  string
		probe func(context.Context, string) (bool, error)
		want  bool
	}{
		{"unconfigured", nil, true},
		{"unmounted", func(context.Context, string) (bool, error) { return false, nil }, false},
		{"mounted", func(context.Context, string) (bool, error) { return true, nil }, true},
		{"unavailable", func(context.Context, string) (bool, error) { return false, errors.New("offline") }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			SetMountProbe(tc.probe)
			if got := entitlementMounted("token"); got != tc.want {
				t.Fatalf("mounted = %v, want %v", got, tc.want)
			}
		})
	}
}
