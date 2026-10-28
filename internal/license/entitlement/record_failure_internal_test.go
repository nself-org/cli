package entitlement

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license"
)

// A server rejection that cannot be remembered must fail the scheduled run with
// ClassWriteFailed, not the plain ClassRejected. Pins the caller guard around
// recordVerdict in revalidateClassified: without the error assignment the run would
// report a rejection as recorded and publish nothing, hiding the persistence failure.
func TestScheduledRefreshReportsRejectionThatCannotBeRecorded(t *testing.T) {
	compattest.Set(t, true)
	p := newArPing(t, time.Now(), "chat")
	p.seedCache(t)
	fakeTimer(t)
	cache, err := license.CachePath()
	if err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(filepath.Dir(cache), rejectedFile)
	marker := filepath.Join(filepath.Dir(cache), markerPrefix+license.HashKey(p.key)+markerSuffix)
	for _, path := range []string{record, marker} { // a directory at each path: neither can be written
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	p.set("revoked", time.Now())
	_, err = RunScheduled(context.Background(), t.TempDir())
	var failed *RefreshError
	if !errors.As(err, &failed) || failed.Class != ClassWriteFailed {
		t.Fatalf("err = %v, want class %s", err, ClassWriteFailed)
	}
}
