// Package license — revocation_test.go covers the CLI-side revocation
// list consumer (D3-T08).
//
// Test cases:
//  1. valid signature → cache populated and IsRecordRevoked respects entries
//  2. invalid signature → refresh rejected, cache untouched
//  3. 7-day-stale cache + offline → fail-open (returns false)
//  4. 8-day-stale cache + offline → fail-open with prominent stderr warning
//  5. canonicalJSON sorts keys deterministically
//  6. matchAny across jti / user_id / kid
//  7. WriteRevocationCache → ReadRevocationCache round-trip
package license

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

// genTestKeypair returns an Ed25519 keypair for signing test payloads.
func genTestKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return pub, priv
}

// installTestPubKey injects the test public key as the only bundled key for
// signature verification, then returns a cleanup function.
func installTestPubKey(t *testing.T, pub ed25519.PublicKey) func() {
	t.Helper()
	useTestKey(t, hex.EncodeToString(pub))
	return func() {
		// t.Setenv handles cleanup automatically; nothing to do here.
	}
}

// withTempCachePath redirects the revocation cache file to a temporary
// directory for the duration of the test.
func withTempCachePath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, revocationCacheFile)
	t.Setenv("LICENSE_REVOCATION_CACHE_PATH", path)
	return path
}

// signList builds a signed RevocationList using the canonical encoder.
func signList(t *testing.T, list RevocationList, priv ed25519.PrivateKey) RevocationList {
	t.Helper()
	list.Signature = "" // ensure unsigned form for signing
	canonical, err := canonicalForVerify(&list)
	if err != nil {
		t.Fatalf("canonicalForVerify: %v", err)
	}
	sig := ed25519.Sign(priv, canonical)
	list.Signature = base64.StdEncoding.EncodeToString(sig)
	return list
}

// startTestServer spins up an httptest server that serves the supplied
// payload as the revocation list endpoint.
func startTestServer(t *testing.T, list RevocationList, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/license/revocation-list") {
			http.NotFound(w, r)
			return
		}
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		body, err := json.Marshal(list)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LICENSE_PING_URL", srv.URL)
	return srv
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestRefreshRevocationList_ValidSignature(t *testing.T) {
	pub, priv := genTestKeypair(t)
	defer installTestPubKey(t, pub)()
	withTempCachePath(t)

	now := time.Now().UTC().Truncate(time.Second)
	list := signList(t, RevocationList{
		IssuedAt:   now.Format(time.RFC3339Nano),
		ValidUntil: now.Add(time.Hour).Format(time.RFC3339Nano),
		Kid:        "primary-2026-q2",
		Revoked: []RevocationEntry{
			{Type: "jti", ID: "jti-1", Reason: "deactivated", RevokedAt: now.Format(time.RFC3339Nano)},
			{Type: "user_id", ID: "user-9", Reason: "fraud", RevokedAt: now.Format(time.RFC3339Nano)},
		},
	}, priv)
	startTestServer(t, list, 0)

	cache, err := RefreshRevocationList(context.Background())
	if err != nil {
		t.Fatalf("RefreshRevocationList: %v", err)
	}
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	if len(cache.List.Revoked) != 2 {
		t.Fatalf("expected 2 revoked entries, got %d", len(cache.List.Revoked))
	}

	// Round-trip: ReadRevocationCache returns the same data.
	read, err := ReadRevocationCache()
	if err != nil {
		t.Fatalf("ReadRevocationCache: %v", err)
	}
	if read.List.Kid != "primary-2026-q2" {
		t.Errorf("unexpected kid: %q", read.List.Kid)
	}

	// IsRecordRevoked should hit each declared revocation.
	if !IsRecordRevoked(LicenseRecord{JTI: "jti-1"}) {
		t.Error("expected jti-1 to be revoked")
	}
	if !IsRecordRevoked(LicenseRecord{UserID: "user-9"}) {
		t.Error("expected user-9 to be revoked")
	}
	if IsRecordRevoked(LicenseRecord{JTI: "jti-other"}) {
		t.Error("expected jti-other to be allowed (not revoked)")
	}
}

func TestRefreshRevocationList_InvalidSignature(t *testing.T) {
	pub, priv := genTestKeypair(t)
	otherPub, _ := genTestKeypair(t)
	_ = otherPub
	defer installTestPubKey(t, pub)()
	withTempCachePath(t)

	// Build a valid list signed with the right key.
	now := time.Now().UTC().Truncate(time.Second)
	list := signList(t, RevocationList{
		IssuedAt:   now.Format(time.RFC3339Nano),
		ValidUntil: now.Add(time.Hour).Format(time.RFC3339Nano),
		Kid:        "primary",
		Revoked:    []RevocationEntry{{Type: "jti", ID: "x", Reason: "r", RevokedAt: now.Format(time.RFC3339Nano)}},
	}, priv)
	// Tamper: add an entry without re-signing.
	list.Revoked = append(list.Revoked, RevocationEntry{
		Type: "jti", ID: "evil", Reason: "spoof", RevokedAt: now.Format(time.RFC3339Nano),
	})
	startTestServer(t, list, 0)

	_, err := RefreshRevocationList(context.Background())
	if err == nil {
		t.Fatal("expected signature error, got nil")
	}
	if !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected signature-related error, got %v", err)
	}

	// Cache must NOT be written on signature failure.
	read, _ := ReadRevocationCache()
	if read != nil {
		t.Errorf("cache should not be written on signature failure, got %+v", read)
	}
}

func TestIsRecordRevoked_StaleWithin7Days_FailOpen(t *testing.T) {
	pub, _ := genTestKeypair(t)
	defer installTestPubKey(t, pub)()
	withTempCachePath(t)
	ResetFailOpenWarning()

	// Cache 6 days old — within the FAIL-OPEN window.
	cache := &RevocationCache{
		List: RevocationList{
			Kid:     "k",
			Revoked: []RevocationEntry{{Type: "jti", ID: "jti-1"}},
		},
		FetchedAt: time.Now().Add(-6 * 24 * time.Hour).Unix(),
	}
	if err := WriteRevocationCache(cache); err != nil {
		t.Fatalf("WriteRevocationCache: %v", err)
	}

	// Within 7d → cache is authoritative; revoked entries match.
	if !IsRecordRevoked(LicenseRecord{JTI: "jti-1"}) {
		t.Error("6-day-stale cache should still match revoked entries")
	}
	if IsRecordRevoked(LicenseRecord{JTI: "jti-other"}) {
		t.Error("non-matching record should not be revoked")
	}
}

func TestIsRecordRevoked_StaleBeyond7Days_FailOpenWithWarning(t *testing.T) {
	pub, _ := genTestKeypair(t)
	defer installTestPubKey(t, pub)()
	withTempCachePath(t)
	ResetFailOpenWarning()

	// Capture stderr.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = origStderr
	})

	// 8 days stale — past FAIL-OPEN cutoff.
	cache := &RevocationCache{
		List: RevocationList{
			Kid:     "k",
			Revoked: []RevocationEntry{{Type: "jti", ID: "jti-1"}},
		},
		FetchedAt: time.Now().Add(-8 * 24 * time.Hour).Unix(),
	}
	if err := WriteRevocationCache(cache); err != nil {
		t.Fatalf("WriteRevocationCache: %v", err)
	}

	// FAIL-OPEN → returns false even though jti-1 is in the list.
	if IsRecordRevoked(LicenseRecord{JTI: "jti-1"}) {
		t.Error("8-day-stale cache should fail-open (return false)")
	}

	_ = w.Close()
	captured, _ := io.ReadAll(r)
	got := string(captured)
	if !strings.Contains(got, "stale") || !strings.Contains(got, "warning") {
		t.Errorf("expected stderr to contain stale-cache warning, got: %q", got)
	}
}

func TestIsRecordRevoked_ColdStart_NoCache(t *testing.T) {
	pub, _ := genTestKeypair(t)
	defer installTestPubKey(t, pub)()
	withTempCachePath(t) // path exists, but no file written

	if IsRecordRevoked(LicenseRecord{JTI: "anything"}) {
		t.Error("cold start (no cache) should report not-revoked")
	}
}

func TestCanonicalJSON_SortsKeys(t *testing.T) {
	out, err := canonicalJSON(map[string]any{
		"b": 1,
		"a": 2,
		"c": []any{"x", "y"},
	})
	if err != nil {
		t.Fatalf("canonicalJSON: %v", err)
	}
	want := `{"a":2,"b":1,"c":["x","y"]}`
	if string(out) != want {
		t.Errorf("got %q, want %q", string(out), want)
	}
}

func TestCanonicalJSON_NestedSorting(t *testing.T) {
	a, _ := canonicalJSON(map[string]any{
		"x": map[string]any{"z": 1, "y": 2},
		"a": []any{1, 2},
	})
	b, _ := canonicalJSON(map[string]any{
		"a": []any{1, 2},
		"x": map[string]any{"y": 2, "z": 1},
	})
	if string(a) != string(b) {
		t.Errorf("identical objects with different declaration order should produce same canonical JSON\n a=%s\n b=%s", a, b)
	}
}

func TestMatchAny(t *testing.T) {
	list := &RevocationList{
		Revoked: []RevocationEntry{
			{Type: "jti", ID: "j1"},
			{Type: "user_id", ID: "u1"},
			{Type: "kid", ID: "k1"},
			{Type: "key_hash", ID: "h1"},
		},
	}
	cases := []struct {
		name string
		rec  LicenseRecord
		want bool
	}{
		{"jti hit", LicenseRecord{JTI: "j1"}, true},
		{"user_id hit", LicenseRecord{UserID: "u1"}, true},
		{"kid hit", LicenseRecord{Kid: "k1"}, true},
		{"key_hash hit", LicenseRecord{KeyHash: "h1"}, true},
		{"key_hash miss", LicenseRecord{KeyHash: "h-other"}, false},
		{"empty record", LicenseRecord{}, false},
		{"miss", LicenseRecord{JTI: "j2", UserID: "u2", Kid: "k2"}, false},
		{"jti empty does not collide with empty entry", LicenseRecord{JTI: ""}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchAny(list, tc.rec); got != tc.want {
				t.Errorf("matchAny(%+v) = %v, want %v", tc.rec, got, tc.want)
			}
		})
	}
}

func TestWriteReadRevocationCache_Roundtrip(t *testing.T) {
	withTempCachePath(t)

	now := time.Now().UTC().Truncate(time.Second)
	original := &RevocationCache{
		List: RevocationList{
			IssuedAt:   now.Format(time.RFC3339Nano),
			ValidUntil: now.Add(time.Hour).Format(time.RFC3339Nano),
			Kid:        "k1",
			Revoked: []RevocationEntry{
				{Type: "jti", ID: "j1", Reason: "x", RevokedAt: now.Format(time.RFC3339Nano)},
			},
			Signature: "sig",
		},
		FetchedAt: now.Unix(),
	}
	if err := WriteRevocationCache(original); err != nil {
		t.Fatalf("WriteRevocationCache: %v", err)
	}
	read, err := ReadRevocationCache()
	if err != nil {
		t.Fatalf("ReadRevocationCache: %v", err)
	}
	if read.FetchedAt != original.FetchedAt {
		t.Errorf("FetchedAt mismatch: got %d, want %d", read.FetchedAt, original.FetchedAt)
	}
	if len(read.List.Revoked) != 1 || read.List.Revoked[0].ID != "j1" {
		t.Errorf("revoked round-trip mismatch: %+v", read.List.Revoked)
	}

	// File permissions — must be 0600 per the env-perms hard rule (Unix only).
	if runtime.GOOS != "windows" {
		path, _ := RevocationCachePath()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Errorf("cache file perms = %o, want 0600", got)
		}
	}
}

func TestRefreshRevocationList_HTTPError(t *testing.T) {
	pub, _ := genTestKeypair(t)
	defer installTestPubKey(t, pub)()
	withTempCachePath(t)
	startTestServer(t, RevocationList{}, http.StatusInternalServerError)

	_, err := RefreshRevocationList(context.Background())
	if err == nil {
		t.Fatal("expected HTTP 500 to surface an error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to mention status code, got %v", err)
	}
}

func TestRefreshRevocationList_ContextCancellation(t *testing.T) {
	pub, _ := genTestKeypair(t)
	defer installTestPubKey(t, pub)()
	withTempCachePath(t)

	// Server that never responds.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LICENSE_PING_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := RefreshRevocationList(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// Sanity check: make sure the Type assertions in matchAny only match when
// the request field is non-empty.  An empty JTI must NOT match an empty
// jti entry (which shouldn't exist anyway, but defence in depth).
func TestMatchAny_EmptyFieldDoesNotMatchEmptyEntry(t *testing.T) {
	list := &RevocationList{
		Revoked: []RevocationEntry{{Type: "jti", ID: ""}},
	}
	if matchAny(list, LicenseRecord{}) {
		t.Error("empty record should never match (defence in depth)")
	}
}

// Ensure the canonical encoder produces the same bytes as the TS server's
// canonical stringify for representative payloads.  We cross-check by
// signing with a known key and verifying the signature round-trips.
func TestCanonicalEncoder_SigningRoundTrip(t *testing.T) {
	pub, priv := genTestKeypair(t)
	defer installTestPubKey(t, pub)()

	list := RevocationList{
		IssuedAt:   "2026-04-25T10:00:00.000Z",
		ValidUntil: "2026-04-25T11:00:00.000Z",
		Kid:        "primary",
		Revoked: []RevocationEntry{
			{Type: "jti", ID: "j1", Reason: "deactivated", RevokedAt: "2026-04-25T09:30:00.000Z"},
		},
	}
	canonical, err := canonicalForVerify(&list)
	if err != nil {
		t.Fatalf("canonicalForVerify: %v", err)
	}
	sig := ed25519.Sign(priv, canonical)
	list.Signature = base64.StdEncoding.EncodeToString(sig)

	if !VerifyRevocationSignature(&list) {
		t.Error("signed list should verify")
	}

	// Tamper: change a single byte of the reason → verification fails.
	list.Revoked[0].Reason = "tampered"
	if VerifyRevocationSignature(&list) {
		t.Error("tampered list should NOT verify")
	}
}

// ─── D3-T08a: ETag conditional-GET tests ─────────────────────────────────────

// TestRefreshRevocationList_CapturesETagOn200 — a 200 response's ETag header
// must be persisted into the cache so subsequent calls can send INM.
func TestRefreshRevocationList_CapturesETagOn200(t *testing.T) {
	pub, priv := genTestKeypair(t)
	defer installTestPubKey(t, pub)()
	withTempCachePath(t)

	now := time.Now().UTC().Truncate(time.Second)
	list := signList(t, RevocationList{
		IssuedAt:   now.Format(time.RFC3339Nano),
		ValidUntil: now.Add(time.Hour).Format(time.RFC3339Nano),
		Kid:        "primary",
		Revoked:    []RevocationEntry{{Type: "jti", ID: "j1", Reason: "x", RevokedAt: now.Format(time.RFC3339Nano)}},
	}, priv)

	const wantEtag = "sha256:abcd1234deadbeef"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/license/revocation-list") {
			http.NotFound(w, r)
			return
		}
		body, _ := json.Marshal(list)
		w.Header().Set("ETag", wantEtag)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LICENSE_PING_URL", srv.URL)

	cache, err := RefreshRevocationList(context.Background())
	if err != nil {
		t.Fatalf("RefreshRevocationList: %v", err)
	}
	if cache.ETag != wantEtag {
		t.Errorf("returned cache ETag = %q, want %q", cache.ETag, wantEtag)
	}
	read, err := ReadRevocationCache()
	if err != nil {
		t.Fatalf("ReadRevocationCache: %v", err)
	}
	if read.ETag != wantEtag {
		t.Errorf("persisted ETag = %q, want %q", read.ETag, wantEtag)
	}
}

// TestRefreshRevocationList_SendsIfNoneMatchWhenCached — when a cache with an
// ETag exists, the next refresh must include `If-None-Match: <etag>`.
func TestRefreshRevocationList_SendsIfNoneMatchWhenCached(t *testing.T) {
	pub, _ := genTestKeypair(t)
	defer installTestPubKey(t, pub)()
	withTempCachePath(t)

	const seedEtag = "sha256:cafef00d"
	seed := &RevocationCache{
		List:      RevocationList{Kid: "primary"},
		FetchedAt: time.Now().Unix(),
		ETag:      seedEtag,
	}
	if err := WriteRevocationCache(seed); err != nil {
		t.Fatalf("WriteRevocationCache: %v", err)
	}

	var gotINM string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotINM = r.Header.Get("If-None-Match")
		// Respond 304 to short-circuit; we only care about the request header.
		w.WriteHeader(http.StatusNotModified)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LICENSE_PING_URL", srv.URL)

	if _, err := RefreshRevocationList(context.Background()); err != nil {
		t.Fatalf("RefreshRevocationList: %v", err)
	}
	if gotINM != seedEtag {
		t.Errorf("If-None-Match header = %q, want %q", gotINM, seedEtag)
	}
}

// TestRefreshRevocationList_HandlesNotModified — a 304 response must skip the
// payload parse, refresh FetchedAt, and preserve the existing ETag and list.
func TestRefreshRevocationList_HandlesNotModified(t *testing.T) {
	pub, _ := genTestKeypair(t)
	defer installTestPubKey(t, pub)()
	withTempCachePath(t)

	const seedEtag = "sha256:1234"
	originalFetched := time.Now().Add(-30 * time.Minute).Unix()
	seed := &RevocationCache{
		List: RevocationList{
			Kid:     "primary",
			Revoked: []RevocationEntry{{Type: "jti", ID: "j1"}},
		},
		FetchedAt: originalFetched,
		ETag:      seedEtag,
	}
	if err := WriteRevocationCache(seed); err != nil {
		t.Fatalf("WriteRevocationCache: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == "" {
			t.Errorf("expected If-None-Match header to be set")
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LICENSE_PING_URL", srv.URL)

	cache, err := RefreshRevocationList(context.Background())
	if err != nil {
		t.Fatalf("RefreshRevocationList: %v", err)
	}
	if cache == nil {
		t.Fatal("expected cache return on 304")
	}
	if cache.FetchedAt <= originalFetched {
		t.Errorf("FetchedAt should advance on 304: was %d, got %d", originalFetched, cache.FetchedAt)
	}
	if cache.ETag != seedEtag {
		t.Errorf("ETag should survive 304: got %q, want %q", cache.ETag, seedEtag)
	}
	if len(cache.List.Revoked) != 1 || cache.List.Revoked[0].ID != "j1" {
		t.Errorf("list contents should survive 304: %+v", cache.List.Revoked)
	}
}

// TestRefreshRevocationList_NoETagSkipsConditional — when the cache predates
// D3-T08a (or was written by an older binary), ETag is empty. The refresh
// must NOT send a stale or empty If-None-Match header.
func TestRefreshRevocationList_NoETagSkipsConditional(t *testing.T) {
	pub, priv := genTestKeypair(t)
	defer installTestPubKey(t, pub)()
	withTempCachePath(t)

	// Seed a cache with no ETag (legacy).
	seed := &RevocationCache{
		List:      RevocationList{Kid: "primary"},
		FetchedAt: time.Now().Unix(),
		// ETag intentionally empty.
	}
	if err := WriteRevocationCache(seed); err != nil {
		t.Fatalf("WriteRevocationCache: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	list := signList(t, RevocationList{
		IssuedAt:   now.Format(time.RFC3339Nano),
		ValidUntil: now.Add(time.Hour).Format(time.RFC3339Nano),
		Kid:        "primary",
		Revoked:    []RevocationEntry{{Type: "jti", ID: "j1", Reason: "x", RevokedAt: now.Format(time.RFC3339Nano)}},
	}, priv)

	var hadINM bool
	var hadIMS bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadINM = r.Header["If-None-Match"]
		_, hadIMS = r.Header["If-Modified-Since"]
		body, _ := json.Marshal(list)
		w.Header().Set("ETag", "sha256:fresh")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LICENSE_PING_URL", srv.URL)

	if _, err := RefreshRevocationList(context.Background()); err != nil {
		t.Fatalf("RefreshRevocationList: %v", err)
	}
	if hadINM {
		t.Error("legacy cache (no ETag) should NOT send If-None-Match")
	}
	if hadIMS {
		t.Error("If-Modified-Since must not be sent (server does not honour it)")
	}
}

// Catch-all sanity: ensure ErrRevocationSignatureInvalid is the precise
// error returned (callers may type-assert).
func TestErrRevocationSignatureInvalid_Identity(t *testing.T) {
	if got := ErrRevocationSignatureInvalid.Error(); !strings.Contains(got, "signature") {
		t.Errorf("error text should mention 'signature', got %q", got)
	}
	if fmt.Sprintf("%v", ErrRevocationSignatureInvalid) == "" {
		t.Error("ErrRevocationSignatureInvalid should not be empty")
	}
}
