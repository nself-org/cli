package catalog

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentCachePairs(t *testing.T) {
	dir := t.TempDir()
	writeCache(dir, sampleCatalog(0), `"0"`)
	var wg sync.WaitGroup
	for n := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 40 {
				writeCache(dir, sampleCatalog(n), fmt.Sprintf(`"%d"`, n))
				c, _, ok := readCache(dir)
				if !ok {
					t.Error("concurrent write lost a valid cache")
					return
				}
				if c.etag == "" {
					continue
				}
				var version int
				if _, err := fmt.Sscanf(c.etag, `"%d"`, &version); err != nil || !bytes.Equal(c.body, sampleCatalog(version)) {
					t.Errorf("cache body and ETag %q belong to different responses", c.etag)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestLegacyCacheDiscardsUnboundETag(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, bodyFile), sampleCatalog(1), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "catalog.etag"), []byte(`"2"`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, _, ok := readCache(dir)
	if !ok || c.etag != "" || !bytes.Equal(c.body, sampleCatalog(1)) {
		t.Fatalf("legacy cache must remain usable without its unbound ETag: %v", c)
	}
	writeCache(dir, sampleCatalog(2), `"2"`)
	c, _, ok = readCache(dir)
	if !ok || c.etag != `"2"` || !bytes.Equal(c.body, sampleCatalog(2)) {
		t.Fatalf("combined cache did not replace legacy cache: %v", c)
	}
	info, err := os.Stat(filepath.Join(dir, recordFile))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("cache record permissions: %v, %v", info, err)
	}
}
