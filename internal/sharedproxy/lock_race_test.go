package sharedproxy

// lock_race_test.go: concurrent first-time creation of the owner lock file.

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestLockCreateRace: two callers creating .lock at once both get the lock (one waits).
// Found as a flaky TestConcurrentAttachesNeverOverlap: os.Root reported ENOENT for the loser.
func TestLockCreateRace(t *testing.T) {
	for i := 0; i < 300; i++ {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "nginx", "shared"), 0o755); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for g := 0; g < 2; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rel, err := Lock(context.Background(), dir)
				if err != nil {
					ents, _ := os.ReadDir(filepath.Join(dir, "nginx"))
					names := ""
					for _, e := range ents {
						names += e.Name() + " "
					}
					t.Errorf("iter %d: %v; nginx has: %s", i, err, names)
					return
				}
				rel()
			}()
		}
		wg.Wait()
	}
}
