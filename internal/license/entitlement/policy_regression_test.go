package entitlement

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
)

func policySigner(t *testing.T) func(string, []string, int64) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("USERPROFILE", home)
	for i := 1; i <= 10; i++ {
		t.Setenv("NSELF_LICENSE_KEY_"+strconv.Itoa(i), "")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old := license.PingKeys
	license.PingKeys = []license.PingKey{{ID: "policy-test", Public: pub}}
	t.Cleanup(func() { license.PingKeys = old })
	return func(key string, plugins []string, exp int64) string {
		body, err := json.Marshal(map[string]any{"sub": license.HashKey(key), "plugins": plugins, "exp": exp, "iss": "ping.nself.org", "aud": "nself-cli"})
		if err != nil {
			t.Fatal(err)
		}
		enc := base64.RawURLEncoding
		unsigned := enc.EncodeToString([]byte(`{"alg":"EdDSA","kid":"policy-test"} `)) + "." + enc.EncodeToString(body)
		return unsigned + "." + enc.EncodeToString(ed25519.Sign(priv, []byte(unsigned)))
	}
}

func TestSelectTokenOrderingAndEligibility(t *testing.T) {
	sign := policySigner(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "accepted")
	now := time.Now().Unix()
	first := sign("accepted", []string{"chat"}, now+3600)
	tie := sign("accepted", []string{"chat", "claw"}, now+3600)
	later := sign("accepted", []string{"chat"}, now+7200)
	uncovered := sign("accepted", []string{"claw"}, now+7200)
	for _, tc := range []struct {
		name       string
		candidates []string
		names      []string
		rejected   map[string]bool
		want       string
		wantErr    bool
	}{
		{"covering-tie-keeps-first", []string{first, tie}, []string{"chat"}, nil, first, false},
		{"fallback-tie-keeps-first", []string{first, tie}, []string{"missing"}, nil, first, true},
		{"later-covering-wins", []string{first, later}, []string{"chat"}, nil, later, false},
		{"older-covering-before-uncovered", []string{first, uncovered}, []string{"chat"}, nil, first, false},
		{"older-covering-after-uncovered", []string{uncovered, first}, []string{"chat"}, nil, first, false},
		{"later-fallback-wins", []string{first, later}, []string{"missing"}, nil, later, true},
		{"rejected-subject", []string{first}, nil, map[string]bool{license.HashKey("accepted"): true}, "", true},
		{"unknown-subject", []string{sign("other", []string{"*"}, now+7200)}, nil, nil, "", true},
		{"zero-expiry", []string{sign("accepted", []string{"*"}, 0)}, nil, nil, "", true},
		{"negative-expiry", []string{sign("accepted", []string{"*"}, -1)}, nil, nil, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectToken(tc.candidates, tc.names, tc.rejected)
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("selected %q, error %v; want %q, error=%v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestRetainedTokenGraceSubjectAndReadLimit(t *testing.T) {
	sign := policySigner(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "accepted")
	path := filepath.Join(t.TempDir(), "entitlement")
	now := time.Now()
	base := sign("accepted", []string{""}, now.Add(time.Hour).Unix())
	payload, err := base64.RawURLEncoding.DecodeString(strings.Split(base, ".")[1])
	if err != nil {
		t.Fatal(err)
	}
	// A valid token exactly at the byte cap distinguishes a size violation
	// from a signature failure and verifies that the final byte is read.
	maxToken := sign("accepted", []string{strings.Repeat("x", 786330-len(payload))}, now.Add(time.Hour).Unix())
	if len(maxToken) != 1<<20 {
		t.Fatalf("boundary fixture is %d bytes", len(maxToken))
	}
	for _, tc := range []struct {
		name, token string
		want        bool
	}{
		{"within-grace", sign("accepted", []string{"chat"}, now.Add(-time.Hour).Unix()), true},
		{"grace-ended", sign("accepted", []string{"chat"}, now.Add(-license.PostExpiryGraceWindow-time.Hour).Unix()), false},
		{"other-subject", sign("other", []string{"chat"}, now.Add(time.Hour).Unix()), false},
		{"zero-expiry", sign("accepted", []string{"chat"}, 0), false},
		{"exact-read-limit", maxToken, true},
		{"over-limit", string(bytes.Repeat([]byte("x"), 1<<20+1)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.token), 0600); err != nil {
				t.Fatal(err)
			}
			got := retainedToken(path)
			if (got != "") != tc.want || tc.want && got != tc.token {
				t.Fatalf("retained=%v, want=%v", got != "", tc.want)
			}
		})
	}
}

func TestManifestNameFallbackAndInstallationDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", "  "+dir+"  ")
	if got, err := installationDirectory(""); err != nil || got != dir {
		t.Fatalf("configured directory: %q, %v", got, err)
	}
	if got, err := installationDirectory("explicit"); err != nil || got != "explicit" {
		t.Fatalf("explicit directory: %q, %v", got, err)
	}
	for _, tc := range []struct{ body, want string }{{`{}`, "legacy"}, {`{"name":"canonical"}`, "canonical"}} {
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		if got, err := manifestName(dir, "legacy"); err != nil || got != tc.want {
			t.Fatalf("manifest name: %q, %v; want %q", got, err, tc.want)
		}
	}
}

func TestInstallationDirectoryHomeDefaultAndFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("USERPROFILE", home)
	if got, err := installationDirectory(""); err != nil || got != filepath.Join(home, ".nself", "plugins") {
		t.Fatalf("home default: %q, %v", got, err)
	}
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("USERPROFILE", "")
	if got, err := installationDirectory(""); err == nil || got != "" {
		t.Fatalf("unavailable home must fail: %q, %v", got, err)
	}
}

func TestStartMayContinueDistinguishesFailureKinds(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{{nil, true}, {errs.New("E125", "licensed service unavailable"), true}, {errs.New("E124", "different failure"), false}, {errors.New("ordinary failure"), false}} {
		var warnings bytes.Buffer
		if got := StartMayContinue(tc.err, &warnings); got != tc.want {
			t.Fatalf("continuation for %v = %v", tc.err, got)
		}
		if tc.err == nil && warnings.Len() != 0 {
			t.Fatal("successful refresh emitted a warning")
		}
		if tc.err != nil && tc.want && warnings.String() != tc.err.Error()+"\n" {
			t.Fatalf("missing actionable warning: %q", warnings.String())
		}
	}
}

// Boundary tests for mutation survivors at lifecycle.go:78:30 (CONDITIONALS_BOUNDARY
// on After) and selection.go:27:31 (CONDITIONALS_BOUNDARY on Exp <= 0).
// retainedToken uses time.Now().After(exp.Add(grace)). We compute exp relative to
// time.Now() inside each subtest so the window between signing and checking is
// negligible; the "past" case uses an extra 2s margin to avoid flakiness.
func TestRetainedTokenExactGraceBoundary(t *testing.T) {
	sign := policySigner(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "accepted")
	path := filepath.Join(t.TempDir(), "entitlement")
	for _, tc := range []struct {
		name   string
		offset time.Duration
		want   bool
	}{
		{"one-second-before-grace", -license.PostExpiryGraceWindow + time.Second, true},
		{"one-second-past-grace", -license.PostExpiryGraceWindow - 2*time.Second, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			exp := now.Add(tc.offset)
			token := sign("accepted", []string{"chat"}, exp.Unix())
			if err := os.WriteFile(path, []byte(token), 0600); err != nil {
				t.Fatal(err)
			}
			got := retainedToken(path)
			if (got != "") != tc.want {
				t.Fatalf("retained=%v, want=%v (offset=%v)", got != "", tc.want, tc.offset)
			}
		})
	}
}

// selectToken rejects claims.Exp <= 0. A future expiry (now+1h) must be accepted;
// zero and negative must be rejected. Each subtest gets its own LICENSE_CACHE_PATH
// so loadRejected never sees a stale or blocked record from another subtest.
func TestSelectTokenExpZeroBoundary(t *testing.T) {
	sign := policySigner(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "boundary-key")
	now := time.Now().Unix()
	for _, tc := range []struct {
		name    string
		exp     int64
		wantErr bool
	}{
		{"exp-zero-rejected", 0, true},
		{"exp-future-accepted", now + 3600, false},
		{"exp-negative-rejected", -1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "license.json"))
			token := sign("boundary-key", []string{"*"}, tc.exp)
			got, err := selectToken([]string{token}, []string{"*"}, nil)
			if (err != nil) != tc.wantErr || (!tc.wantErr && got != token) {
				t.Fatalf("got=%q err=%v; wantErr=%v", got, err, tc.wantErr)
			}
		})
	}
}

// ARITHMETIC_BASE at rejected.go:83:49 mutates len(name)-len(markerSuffix) to
// len(name)+len(markerSuffix), producing a wrong slice index. A marker with a
// known hash must yield exactly that hash in the set, not a truncated or
// extended key.
func TestLoadRejectedExtractsExactHashFromMarkerName(t *testing.T) {
	dir := useRejectedDir(t)
	hash := "abcdef0123456789"
	marker := filepath.Join(dir, markerPrefix+hash+markerSuffix)
	if err := os.WriteFile(marker, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	set, err := loadRejected()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if !set[hash] {
		t.Fatalf("hash %q not found in set %v", hash, set)
	}
	for k := range set {
		if k != hash {
			t.Fatalf("unexpected key %q in set (want only %q)", k, hash)
		}
	}
}

// TestSelectTokenExpZeroBoundaryKillsMutant verifies that claims.Exp <= 0 at
// selection.go:27:31 is pinned. The mutant flips <= to <, accepting exp=0.
// We use a token with exp=0 that is valid in every other respect (correct subject,
// covering installed plugins, not rejected) so that the Exp check is the ONLY
// rejection reason. Under the mutant, selectToken would accept it as a fallback
// (exp=0 > fallbackExp=-1 initially), but tokenPolicy would reject it with
// "entitlement grace ended". We assert the error message contains "grace ended"
// only when the token passes the Exp filter — which happens under the mutant.
// On original code, exp=0 is filtered before reaching tokenPolicy, so the error
// is the generic "no verified entitlement".
func TestSelectTokenExpZeroBoundaryKillsMutant(t *testing.T) {
	sign := policySigner(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "boundary-key")
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "license.json"))
	token := sign("boundary-key", []string{"*"}, 0)
	got, err := selectToken([]string{token}, []string{"chat"}, nil)
	if got != "" {
		t.Fatalf("exp=0 token was accepted: %q", got)
	}
	if err == nil {
		t.Fatal("exp=0 token returned no error")
	}
	// Original: "no verified entitlement belongs to an accepted configured licence"
	// Mutant (<= to <): token passes Exp<0 check, reaches tokenPolicy, gets "entitlement grace ended"
	if strings.Contains(err.Error(), "grace ended") {
		t.Fatalf("MUTANT DETECTED: exp=0 passed Exp filter and reached tokenPolicy: %v", err)
	}
	if !strings.Contains(err.Error(), "no verified entitlement") {
		t.Fatalf("unexpected error for exp=0: %v", err)
	}
}

// TestRetainedTokenGraceBoundaryKillsMutant pins lifecycle.go:78:30 where
// time.Now().After(exp.Add(grace)) controls retention. The mutant flips After
// to !After (or Before), retaining tokens PAST grace. We test a token exactly
// 2s past grace: original rejects it, mutant retains it.
func TestRetainedTokenGraceBoundaryKillsMutant(t *testing.T) {
	sign := policySigner(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "accepted")
	path := filepath.Join(t.TempDir(), "entitlement")
	now := time.Now()
	exp := now.Add(-license.PostExpiryGraceWindow - 2*time.Second)
	token := sign("accepted", []string{"chat"}, exp.Unix())
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	got := retainedToken(path)
	if got != "" {
		t.Fatalf("MUTANT DETECTED: token past grace was retained (len=%d)", len(got))
	}
}
