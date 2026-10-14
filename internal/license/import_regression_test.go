package license

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestImportCacheV15RejectsForcedUnsignedCache(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "1")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "1")
	path := filepath.Join(t.TempDir(), "license.json")
	t.Setenv("LICENSE_CACHE_PATH", path)
	previous := []byte(`{"key_hash":"existing","tier":"plus"}`)
	if err := os.WriteFile(path, previous, 0600); err != nil {
		t.Fatal(err)
	}
	err := ImportCache(makeFakeEntryJSON(t))
	if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("forced unsigned import must fail signature verification: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, previous) {
		t.Fatalf("rejected import changed the existing cache: %q, %v", got, err)
	}
}

func TestImportCacheV15AcceptsSignedCacheWithSkipVerify(t *testing.T) {
	compattest.Set(t, true)
	newPingStub(t, nil)
	_, entry := validateAndRead(t)
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "1")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "1")
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "imported.json"))
	if err := ImportCache(data); err != nil {
		t.Fatal(err)
	}
	got, err := ReadCache()
	if err != nil || got == nil || !got.VerifySignature() {
		t.Fatalf("signed import must remain usable: %+v, %v", got, err)
	}
}
