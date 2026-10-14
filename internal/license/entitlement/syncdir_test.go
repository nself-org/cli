package entitlement

import "testing"

func TestSyncDirectorySupportsHostPlatform(t *testing.T) {
	if err := syncDirectory(t.TempDir()); err != nil {
		t.Fatalf("directory sync must support host filesystem semantics: %v", err)
	}
	if err := syncDirectory(t.TempDir() + "/missing"); err == nil {
		t.Fatal("directory sync must preserve open failures")
	}
}
