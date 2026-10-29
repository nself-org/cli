package secrets

import (
	"path/filepath"
	"testing"
)

// Every supported platform must provide a working store lock; the "other"
// stub returning an error turned every recipient change into E401.
func TestLockRecipientStoreExcludesSecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ci.age.lock")
	unlock, err := lockRecipientStore(path)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if second, err := lockRecipientStore(path); err == nil {
		second()
		t.Fatal("second lock acquired while first held")
	}
	unlock()
	again, err := lockRecipientStore(path)
	if err != nil {
		t.Fatalf("relock after unlock: %v", err)
	}
	again()
}
