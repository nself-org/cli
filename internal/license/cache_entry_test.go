package license

import (
	"encoding/json"
	"os"
	"runtime"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// realShape is a response as ping sends it today: plugins_allowed, never plugins.
func realShape(t *testing.T) *ValidateResponse {
	t.Helper()
	var vr ValidateResponse
	if err := json.Unmarshal([]byte(`{"valid":true,"tier":"free","plugins_allowed":["bundle:chat","ai"]}`), &vr); err != nil {
		t.Fatal(err)
	}
	return &vr
}

// pluginsFor and the cache entry: v1.4 keeps the always-empty legacy field
// (base binary behaviour), v1.5 uses what ping sent.
func TestPluginsForGatedByCompat(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		got := pluginsFor(realShape(t))
		if compatMode15() && len(got) != 2 {
			t.Errorf("v1.5 plugins = %v, want plugins_allowed", got)
		}
		if !compatMode15() && len(got) != 0 {
			t.Errorf("v1.4 plugins = %v, want the legacy empty list", got)
		}
	})
}

// TestCheckerRealPluginsAllowed: the checker's cacheCoversBundle reads
// entry.PluginsAllowed; with a real response only v1.5 sees the bundle.
func TestCheckerRealPluginsAllowed(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		entry := responseToCache(testLicenseKey, realShape(t))
		if got := cacheCoversBundle(entry, "chat"); got != compatMode15() {
			t.Errorf("cacheCoversBundle(chat) = %v, want %v for plugins_allowed [bundle:chat]", got, compatMode15())
		}
		if cacheCoversBundle(entry, "claw") {
			t.Error("a bundle ping did not list must never be covered on a free tier")
		}
	})
}

// TestBundleInfoPluginsAllowed: `bundle info` (cmd/commands/bundle_info.go)
// reads cache.PluginsAllowed. This asserts the cache it reads, from a real
// response, in both modes: base-binary output in v1.4, real list in v1.5.
func TestBundleInfoPluginsAllowed(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		entry := responseToCache(testLicenseKey, realShape(t))
		want := 0
		if compatMode15() {
			want = 2
		}
		if len(entry.PluginsAllowed) != want {
			t.Errorf("cache PluginsAllowed = %v, want %d entries", entry.PluginsAllowed, want)
		}
	})
}

func TestCacheEntryRoundTripKeepsSignedFields(t *testing.T) {
	t.Setenv("LICENSE_CACHE_PATH", t.TempDir()+"/license.json")
	in := &CacheEntry{KeyHash: "k", Tier: "plus", FetchedAt: 5, RawBody: `{"valid":true}`, BodySig: "ab", JWT: "a.b.c", JWTKid: "1"}
	if err := WriteCache(in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadCache()
	if err != nil || out.RawBody != in.RawBody || out.BodySig != "ab" || out.JWT != "a.b.c" || out.JWTKid != "1" {
		t.Fatalf("round trip lost signed fields: %+v, %v", out, err)
	}
	if runtime.GOOS != "windows" {
		path, _ := CachePath()
		if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
			t.Errorf("cache file mode = %v, %v; want 0600", st.Mode().Perm(), err)
		}
	}
}

func TestVerifyResponseSig(t *testing.T) {
	pub, priv := testKeypairHex(t)
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", pub)
	body := []byte(`{"valid":true}`)
	good := signHex(priv, body)
	if err := verifyResponseSig(body, good); err != nil {
		t.Errorf("good signature rejected: %v", err)
	}
	for name, sig := range map[string]string{"missing": "", "malformed": "xyz", "wrong": signHex(priv, []byte("other"))} {
		if err := verifyResponseSig(body, sig); err == nil {
			t.Errorf("%s signature accepted", name)
		}
	}
}
