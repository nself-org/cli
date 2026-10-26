package config

// defaults_helpers_test.go — tests for the scoped secret source (P7-LIVE-21).
//
// Purpose: UseRandSource makes generateSecureRandom deterministic inside one
// scope and returns to crypto/rand afterwards.
// Constraints: tests in this file must not run in parallel with each other.

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

type countReader struct{ next byte }

func (c *countReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = c.next
		c.next++
	}
	return len(p), nil
}

type failReader struct{}

func (failReader) Read([]byte) (int, error) { return 0, errors.New("source exhausted") }

func TestUseRandSourceDeterministic(t *testing.T) {
	gen := func() []string {
		restore := UseRandSource(&countReader{})
		defer restore()
		var out []string
		for _, n := range []int{24, 32, 44} {
			s, err := generateSecureRandom(n)
			if err != nil || len(s) != n {
				t.Fatalf("generateSecureRandom(%d) = %q, %v", n, s, err)
			}
			out = append(out, s)
		}
		return out
	}
	a, b := gen(), gen()
	if strings.Join(a, ",") != strings.Join(b, ",") {
		t.Errorf("same source, different secrets: %v vs %v", a, b)
	}
}

func TestUseRandSourceRestoresCryptoRand(t *testing.T) {
	restore := UseRandSource(&countReader{})
	seeded, _ := generateSecureRandom(32)
	restore()
	x, err1 := generateSecureRandom(32)
	y, err2 := generateSecureRandom(32)
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	if x == y || x == seeded || y == seeded {
		t.Error("after restore, secrets are not fresh crypto/rand output")
	}
}

func TestUseRandSourceErrorHidesSource(t *testing.T) {
	restore := UseRandSource(failReader{})
	defer restore()
	_, err := generateSecureRandom(32)
	if err == nil || !strings.Contains(err.Error(), "generating secret") {
		t.Fatalf("want generating-secret error, got %v", err)
	}
}

// fixedThenReader serves head once, then falls through to the countReader.
type fixedThenReader struct {
	head []byte
	then countReader
}

func (r *fixedThenReader) Read(p []byte) (int, error) {
	if len(r.head) > 0 {
		n := copy(p, r.head)
		r.head = r.head[n:]
		return n, nil
	}
	return r.then.Read(p)
}

// encodedBytes returns the raw bytes whose base64url form starts with prefix
// (padded with 'A' to a multiple of four), so a test can force the exact
// string generateSecureRandom first sees.
func encodedBytes(t *testing.T, prefix string, length int) []byte {
	t.Helper()
	enc := prefix + strings.Repeat("A", (4-len(prefix)%4)%4)
	for len(enc) < length*4/3 {
		enc += "AAAA"
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc[:len(enc)/4*4])
	if err != nil {
		t.Fatal(err)
	}
	return raw[:length]
}

// A random draw that spells a blocked pattern (about 1 in 10^4 per project, the
// flake in TestSurfaceIdentity) is discarded, so the secret always passes the
// staging/prod validators.
func TestGenerateSecureRandomSkipsBlockedPatterns(t *testing.T) {
	for _, bad := range []string{"xxTeStxx", "AdMiNxx", "xxhasuraxx", "12345xxx", "change-me-"} {
		for _, n := range []int{24, 44} {
			src := &fixedThenReader{head: encodedBytes(t, bad, n)}
			restore := UseRandSource(src)
			s, err := generateSecureRandom(n)
			restore()
			if err != nil || len(s) != n {
				t.Fatalf("%q len %d: %q, %v", bad, n, s, err)
			}
			if len(src.head) != 0 {
				t.Fatalf("%q len %d: the forced draw was not consumed", bad, n)
			}
			if isInsecurePassword(s) || isPlaceholderSecret(s) {
				t.Errorf("%q len %d: generated %q, which the validators reject", bad, n, s)
			}
		}
	}
}

// Plan and apply read one seeded stream: skipping a bad draw must be the same
// decision both times, so the two still produce one secret.
func TestGenerateSecureRandomSkipIsDeterministic(t *testing.T) {
	gen := func() string {
		restore := UseRandSource(&fixedThenReader{head: encodedBytes(t, "xxtestxx", 24)})
		defer restore()
		s, err := generateSecureRandom(24)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if a, b := gen(), gen(); a != b {
		t.Errorf("seeded source, different secrets: %q vs %q", a, b)
	}
}

// A source that only ever yields a blocked value fails instead of looping.
func TestGenerateSecureRandomStuckSourceErrors(t *testing.T) {
	restore := UseRandSource(&repeatReader{b: encodedBytes(t, "xxtestxx", 24)})
	defer restore()
	if s, err := generateSecureRandom(24); err == nil {
		t.Fatalf("want an error, got %q", s)
	}
}

type repeatReader struct{ b []byte }

func (r *repeatReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.b[i%len(r.b)]
	}
	return len(p), nil
}
