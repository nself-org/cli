package license

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

// unsignedList is a revocation list with no signature at all.
func unsignedList() RevocationList {
	now := time.Now().UTC().Truncate(time.Second)
	return RevocationList{
		IssuedAt: now.Format(time.RFC3339Nano), ValidUntil: now.Add(time.Hour).Format(time.RFC3339Nano),
		Kid: "k", Revoked: []RevocationEntry{{Type: "jti", ID: "j1", Reason: "r", RevokedAt: now.Format(time.RFC3339Nano)}},
	}
}

// TestRefreshUnsignedListNoKeysNoOverride pins that with no verification key and no override an unsigned list is accepted and cached (D-0226).
func TestRefreshUnsignedListNoKeysNoOverride(t *testing.T) {
	setPingKeys(t)
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "")
	withTempCachePath(t)
	startTestServer(t, unsignedList(), 0)
	if !IsZeroPubKey() {
		t.Fatal("test needs a build with no verification key")
	}
	cache, err := RefreshRevocationList(context.Background())
	if err != nil || cache == nil {
		t.Fatalf("got (%v, %v), want the unsigned list accepted", cache, err)
	}
	if read, err := ReadRevocationCache(); err != nil || read == nil || read.List.Kid != "k" {
		t.Errorf("cache not persisted: %v, %v", read, err)
	}
}

// TestRefreshUnsignedListNoKeysWithOverride pins that any override value forces verification, so an unsigned list is refused and not cached (D-0226).
func TestRefreshUnsignedListNoKeysWithOverride(t *testing.T) {
	setPingKeys(t)
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "not-a-hex-key") // never decodes: still no usable key
	path := withTempCachePath(t)
	startTestServer(t, unsignedList(), 0)
	if !IsZeroPubKey() {
		t.Fatal("test needs a build with no verification key")
	}
	cache, err := RefreshRevocationList(context.Background())
	if !errors.Is(err, ErrRevocationSignatureInvalid) || cache != nil {
		t.Fatalf("got (%v, %v), want ErrRevocationSignatureInvalid", cache, err)
	}
	if r, _ := ReadRevocationCache(); r != nil {
		t.Errorf("refused list was persisted at %s", path)
	}
}

// TestRefreshUnsignedListWithKey pins that once a verification key exists an unsigned list is refused (D-0226).
func TestRefreshUnsignedListWithKey(t *testing.T) {
	pub, _ := genTestKeypair(t)
	installTestPubKey(t, pub)
	t.Setenv("LICENSE_PUBLIC_KEY_OVERRIDE", "")
	withTempCachePath(t)
	startTestServer(t, unsignedList(), 0)
	cache, err := RefreshRevocationList(context.Background())
	if !errors.Is(err, ErrRevocationSignatureInvalid) || cache != nil {
		t.Fatalf("got (%v, %v), want ErrRevocationSignatureInvalid", cache, err)
	}
}

// TestIsStaleFailOpenEdges pins the exact 7-day staleness edge and that a missing cache is stale (D-0226).
func TestIsStaleFailOpenEdges(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	at := func(age time.Duration) *RevocationCache { return &RevocationCache{FetchedAt: now.Add(-age).Unix()} }
	cases := []struct {
		name  string
		cache *RevocationCache
		want  bool
	}{
		{"nil cache", nil, true},
		{"fresh", at(0), false},
		{"exactly the staleness limit", at(RevocationFailOpenStaleness), false},
		{"limit + 1s", at(RevocationFailOpenStaleness + time.Second), true},
		{"limit - 1s", at(RevocationFailOpenStaleness - time.Second), false},
	}
	for _, c := range cases {
		if got := isStaleFailOpen(c.cache, now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// TestRefreshAndWarnExact pins the warning text on failure and silence on success (D-0226).
func TestRefreshAndWarnExact(t *testing.T) {
	withTempCachePath(t)
	startTestServer(t, unsignedList(), 500)
	var w bytes.Buffer
	refreshAndWarn(context.Background(), &w, "test refresh")
	if want := "warning: test refresh failed: revocation list HTTP 500: \n"; w.String() != want {
		t.Errorf("failure: got %q, want %q", w.String(), want)
	}

	pub, priv := genTestKeypair(t)
	installTestPubKey(t, pub)
	withTempCachePath(t)
	startTestServer(t, signList(t, unsignedList(), priv), 0)
	w.Reset()
	refreshAndWarn(context.Background(), &w, "test refresh")
	if w.Len() != 0 {
		t.Errorf("success: wrote %q, want nothing", w.String())
	}
}
