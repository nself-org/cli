package license

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestIsZeroPubKey_* verify IsZeroPubKey reports "no usable key" from the
// committed key set.
func TestIsZeroPubKey_EmptyString(t *testing.T) {
	setPingKeys(t)
	if !IsZeroPubKey() {
		t.Error("an empty key set should read as zero")
	}
}

func TestIsZeroPubKey_AllZeroHex(t *testing.T) {
	setPingKeys(t, PingKey{ID: "1", Public: pubKeyFromHex(strings.Repeat("00", 32))})
	if !IsZeroPubKey() {
		t.Error("an all-zero key should read as zero")
	}
}

func TestIsZeroPubKey_ValidNonZeroHex(t *testing.T) {
	useTestKey(t, "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")
	if IsZeroPubKey() {
		t.Error("a non-zero key should not read as zero")
	}
}

func TestIsZeroPubKey_SingleNonZeroChar(t *testing.T) {
	useTestKey(t, strings.Repeat("00", 31)+"01")
	if IsZeroPubKey() {
		t.Error("a key with one non-zero byte should not read as zero")
	}
}

// TestWriteCache_AtomicTmpfileNoLeftover ensures the atomic tmpfile + rename
// pattern leaves no stray ".tmp" files in the cache directory after success
// (D3-T10).
func TestWriteCache_AtomicTmpfileNoLeftover(t *testing.T) {
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)

	entry := &CacheEntry{
		KeyHash: HashKey("nself_pro_testkey1234567890abcdef12345"),
		Tier:    "pro",
	}
	if err := WriteCache(entry); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	dir := filepath.Dir(cachePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("leftover tmpfile after atomic write: %s", e.Name())
		}
	}

	// Verify the final file is parseable + has 0600 perms (Unix only).
	if runtime.GOOS != "windows" {
		info, err := os.Stat(cachePath)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("cache file perms = %o, want 0600", perm)
		}
	}
	data, _ := os.ReadFile(cachePath)
	var got CacheEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("cache not parseable: %v", err)
	}
	if got.KeyHash != entry.KeyHash {
		t.Errorf("cache content drift: got %q, want %q", got.KeyHash, entry.KeyHash)
	}
}

// TestWriteCache_AtomicOverwritePreservesContent ensures that overwriting an
// existing cache file with a new entry succeeds and the resulting file is
// fully replaced (no torn write of merged content).
func TestWriteCache_AtomicOverwritePreservesContent(t *testing.T) {
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "license.json")
	t.Setenv("LICENSE_CACHE_PATH", cachePath)

	first := &CacheEntry{KeyHash: HashKey("k1"), Tier: "pro"}
	if err := WriteCache(first); err != nil {
		t.Fatalf("WriteCache 1: %v", err)
	}
	second := &CacheEntry{KeyHash: HashKey("k2"), Tier: "enterprise"}
	if err := WriteCache(second); err != nil {
		t.Fatalf("WriteCache 2: %v", err)
	}

	data, _ := os.ReadFile(cachePath)
	var got CacheEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("cache parse: %v", err)
	}
	if got.Tier != "enterprise" || got.KeyHash != HashKey("k2") {
		t.Errorf("overwrite did not take effect: got %+v", got)
	}
}

// newTestKeyPair generates a fresh Ed25519 keypair for use in signature tests.
func newTestKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	return pub, priv
}

// signEntry signs the entry's canonical payload with the given private key and
// stores the hex-encoded signature + keyID=1 on the entry.
func signEntry(t *testing.T, entry *CacheEntry, priv ed25519.PrivateKey) {
	t.Helper()
	sig := ed25519.Sign(priv, entry.signablePayload())
	entry.Signature = hex.EncodeToString(sig)
	entry.SignatureKeyID = 1
}

// TestSignablePayload_MutatedPluginsAllowed_FailsVerification verifies that
// mutating PluginsAllowed after signing causes VerifySignature to return false.
// This is the regression test for SIEGE V03-F01: previously PluginsAllowed was
// excluded from the signed payload, allowing injection of arbitrary plugin
// names without invalidating the Ed25519 signature.
func TestSignablePayload_MutatedPluginsAllowed_FailsVerification(t *testing.T) {
	pub, priv := newTestKeyPair(t)
	useTestKey(t, hex.EncodeToString(pub))

	entry := &CacheEntry{
		KeyHash:        HashKey("nself_pro_testkey_v03f01"),
		Tier:           "pro",
		PluginsAllowed: []string{"ai", "claw", "notify"},
		FetchedAt:      1_700_000_000,
		ExpiresAt:      1_700_086_400,
	}
	signEntry(t, entry, priv)

	// Sanity: valid entry must verify.
	if !entry.VerifySignature() {
		t.Fatal("expected valid signature before mutation; got false")
	}

	// Inject an extra plugin — simulates an attacker editing ~/.cache/nself/license.json.
	entry.PluginsAllowed = append(entry.PluginsAllowed, "media-processing")

	// After mutation the signature must NOT verify.
	if entry.VerifySignature() {
		t.Error("SIEGE V03-F01: VerifySignature returned true after PluginsAllowed mutation — payload does not cover plugins_allowed")
	}
}

// TestSignablePayload_ValidCacheVerifies confirms that a correctly signed cache
// entry still passes VerifySignature after the plugins_allowed field is
// included in the signed payload (no regression on the happy path).
func TestSignablePayload_ValidCacheVerifies(t *testing.T) {
	pub, priv := newTestKeyPair(t)
	useTestKey(t, hex.EncodeToString(pub))

	entry := &CacheEntry{
		KeyHash:        HashKey("nself_pro_happypath_key"),
		Tier:           "enterprise",
		PluginsAllowed: []string{"voice", "google", "ai", "mcp"},
		FetchedAt:      1_700_100_000,
		ExpiresAt:      1_700_186_400,
	}
	signEntry(t, entry, priv)

	if !entry.VerifySignature() {
		t.Error("VerifySignature returned false for a correctly signed entry — regression in happy path")
	}
}
