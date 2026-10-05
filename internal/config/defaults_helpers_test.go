package config

// defaults_helpers_test.go — tests for the scoped secret source (P7-LIVE-21).
//
// Purpose: UseRandSource makes generateSecureRandom deterministic inside one
// scope and returns to crypto/rand afterwards.
// Constraints: tests in this file must not run in parallel with each other.

import (
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
