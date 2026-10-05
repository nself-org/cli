package signing_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/cli/sdk/go/v2/signing/signingtest"
)

var msg = []byte("nself-signing-test-v1\nhello")

// A signature by a key of each purpose fails under every other purpose.
func TestCrossPurposeTable(t *testing.T) {
	for _, a := range allPurposes {
		k, s := signingtest.NewKey(t, a)
		sg := sign(t, s, msg)
		for _, b := range allPurposes {
			v := newVerifier(t, b, []signing.Key{k}, nil)
			want := signing.ErrWrongPurpose
			if a == b {
				want = nil
			}
			t.Run(fmt.Sprintf("%s_under_%s", a, b), func(t *testing.T) {
				only(t, v.Verify(msg, sg), want)
				if v.Purpose() != b {
					t.Fatalf("Purpose() = %q", v.Purpose())
				}
			})
		}
	}
}

func TestScope(t *testing.T) {
	k, s := signingtest.NewKey(t, signing.PurposePlugins)
	free, paid, none := k, k, k
	free.Scope, paid.Scope, none.Scope = "free", "licensed", ""
	sg := sign(t, s, msg)
	cases := []struct {
		name string
		key  signing.Key
		opts []signing.Option
		want error
	}{
		{"match", free, []signing.Option{signing.WithScope("free")}, nil},
		{"other scope", paid, []signing.Option{signing.WithScope("free")}, signing.ErrWrongScope},
		{"unscoped key under scoped verifier", none, []signing.Option{signing.WithScope("free")}, signing.ErrWrongScope},
		{"scope is case sensitive", free, []signing.Option{signing.WithScope("Free")}, signing.ErrWrongScope},
		{"unscoped verifier accepts scoped key", paid, nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			only(t, newVerifier(t, signing.PurposePlugins, []signing.Key{c.key}, nil, c.opts...).Verify(msg, sg), c.want)
		})
	}
}

func TestRevokedUnknownAndBadSignature(t *testing.T) {
	k, s := signingtest.NewKey(t, signing.PurposeCIRelease)
	sg := sign(t, s, msg)

	only(t, newVerifier(t, signing.PurposeCIRelease, []signing.Key{k}, []string{k.ID}).Verify(msg, sg), signing.ErrRevoked)
	only(t, newVerifier(t, signing.PurposeCIRelease, []signing.Key{k}, nil,
		signing.WithRevoked(func(id string) bool { return id == k.ID })).Verify(msg, sg), signing.ErrRevoked)
	only(t, newVerifier(t, signing.PurposeCIRelease, []signing.Key{k}, nil,
		signing.WithRevoked(func(string) bool { return false })).Verify(msg, sg), nil)
	only(t, newVerifier(t, signing.PurposeCIRelease, nil, []string{k.ID}).Verify(msg, sg), signing.ErrRevoked)
	only(t, newVerifier(t, signing.PurposeCIRelease, nil, nil).Verify(msg, sg), signing.ErrUnknownKey)
	only(t, newVerifier(t, signing.PurposeCIRelease, []signing.Key{k}, nil).Verify(msg, sg), nil)

	v := newVerifier(t, signing.PurposeCIRelease, []signing.Key{k}, nil)
	other := signing.Signature{KeyID: k.ID + "x", Sig: sg.Sig}
	only(t, v.Verify(msg, other), signing.ErrUnknownKey)

	for i := range msg {
		m := bytes.Clone(msg)
		m[i] ^= 1
		only(t, v.Verify(m, sg), signing.ErrBadSignature)
	}
	only(t, v.Verify(msg[:len(msg)-1], sg), signing.ErrBadSignature)
	only(t, v.Verify(append(bytes.Clone(msg), 0), sg), signing.ErrBadSignature)
	only(t, v.Verify(nil, sg), signing.ErrBadSignature)
	for i := range sg.Sig {
		bad := bytes.Clone(sg.Sig)
		bad[i] ^= 0x80
		only(t, v.Verify(msg, signing.Signature{KeyID: k.ID, Sig: bad}), signing.ErrBadSignature)
	}
}

func TestMalformedInputs(t *testing.T) {
	k, s := signingtest.NewKey(t, signing.PurposeAgent)
	sg := sign(t, s, msg)
	v := newVerifier(t, signing.PurposeAgent, []signing.Key{k}, nil)
	for _, n := range []int{0, 1, 63, 65, 128} {
		only(t, v.Verify(msg, signing.Signature{KeyID: k.ID, Sig: make([]byte, n)}), signing.ErrMalformed)
	}
	only(t, v.Verify(msg, signing.Signature{KeyID: k.ID}), signing.ErrMalformed)
	for _, id := range []string{"", strings.Repeat("a", 129), "a b", "a/b", "é", "a\x00b", "a\nb", "a:b"} {
		only(t, v.Verify(msg, signing.Signature{KeyID: id, Sig: sg.Sig}), signing.ErrMalformed)
	}
	// 128 characters is a legal id (so unknown, not malformed).
	only(t, v.Verify(msg, signing.Signature{KeyID: strings.Repeat("A", 128), Sig: sg.Sig}), signing.ErrUnknownKey)
	only(t, v.Verify(msg, signing.Signature{KeyID: "a.B_c-9", Sig: sg.Sig}), signing.ErrUnknownKey)
	// Hostile text is clipped and quoted in errors.
	err := v.Verify(msg, signing.Signature{KeyID: strings.Repeat("x\n", 500), Sig: sg.Sig})
	if err == nil || len(err.Error()) > 200 || strings.Contains(err.Error(), "\n") {
		t.Fatalf("error text not bounded/quoted: %q", err)
	}
	var zero *signing.Verifier
	only(t, zero.Verify(msg, sg), signing.ErrMalformed)
	only(t, (&signing.Verifier{}).Verify(msg, sg), signing.ErrMalformed)
}

func TestKeyValidity(t *testing.T) {
	k, s := signingtest.NewKey(t, signing.PurposeCINode)
	sg := sign(t, s, msg)
	t0 := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	k.NotBefore, k.NotAfter = t0, t0.Add(time.Hour)
	at := func(now time.Time) error {
		return newVerifier(t, signing.PurposeCINode, []signing.Key{k}, nil,
			signing.WithClock(func() time.Time { return now })).Verify(msg, sg)
	}
	only(t, at(t0.Add(-time.Nanosecond)), signing.ErrNotYetValid)
	only(t, at(t0), nil)
	only(t, at(t0.Add(time.Hour-time.Nanosecond)), nil)
	only(t, at(t0.Add(time.Hour)), signing.ErrExpired)
	only(t, at(t0.Add(48*time.Hour)), signing.ErrExpired)
	// Unbounded sides.
	k.NotBefore = time.Time{}
	only(t, at(t0.Add(-1000*time.Hour)), nil)
	k.NotBefore, k.NotAfter = t0, time.Time{}
	only(t, at(t0.Add(1000*time.Hour)), nil)
	// Default clock is the real one: a key that ended in the past is expired.
	k.NotBefore, k.NotAfter = time.Time{}, time.Unix(1, 0)
	only(t, newVerifier(t, signing.PurposeCINode, []signing.Key{k}, nil).Verify(msg, sg), signing.ErrExpired)
	k.NotAfter = time.Now().Add(time.Hour)
	only(t, newVerifier(t, signing.PurposeCINode, []signing.Key{k}, nil).Verify(msg, sg), nil)
}

func TestConstructorErrors(t *testing.T) {
	k, _ := signingtest.NewKey(t, signing.PurposePlugins)
	bad := func(mut func(*signing.Key)) []signing.Key { c := k; mut(&c); return []signing.Key{c} }
	cases := map[string]func() error{
		"bad purpose":   func() error { _, e := signing.NewVerifier("nope", nil, nil); return e },
		"empty purpose": func() error { _, e := signing.NewVerifier("", nil, nil); return e },
		"dup id":        func() error { _, e := signing.NewVerifier(signing.PurposePlugins, []signing.Key{k, k}, nil); return e },
		"empty key id": func() error {
			_, e := signing.NewVerifier(signing.PurposePlugins, bad(func(c *signing.Key) { c.ID = "" }), nil)
			return e
		},
		"bad key id": func() error {
			_, e := signing.NewVerifier(signing.PurposePlugins, bad(func(c *signing.Key) { c.ID = "a b" }), nil)
			return e
		},
		"key bad purpose": func() error {
			_, e := signing.NewVerifier(signing.PurposePlugins, bad(func(c *signing.Key) { c.Purpose = "x" }), nil)
			return e
		},
		"short public": func() error {
			_, e := signing.NewVerifier(signing.PurposePlugins, bad(func(c *signing.Key) { c.Public = c.Public[:31] }), nil)
			return e
		},
		"long public": func() error {
			_, e := signing.NewVerifier(signing.PurposePlugins, bad(func(c *signing.Key) { c.Public = append(append([]byte{}, c.Public...), 0) }), nil)
			return e
		},
		"nil public": func() error {
			_, e := signing.NewVerifier(signing.PurposePlugins, bad(func(c *signing.Key) { c.Public = nil }), nil)
			return e
		},
		"bad revoked id": func() error { _, e := signing.NewVerifier(signing.PurposePlugins, nil, []string{"a b"}); return e },
		"empty scope": func() error {
			_, e := signing.NewVerifier(signing.PurposePlugins, nil, nil, signing.WithScope(""))
			return e
		},
		"nil clock": func() error {
			_, e := signing.NewVerifier(signing.PurposePlugins, nil, nil, signing.WithClock(nil))
			return e
		},
		"nil lookup": func() error { _, e := signing.NewLookupVerifier(signing.PurposePlugins, nil); return e },
		"lookup bad purpose": func() error {
			_, e := signing.NewLookupVerifier("zzz", func(context.Context, string) (signing.Key, bool, error) { return signing.Key{}, false, nil })
			return e
		},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) { only(t, f(), signing.ErrMalformed) })
	}
	// nil option is skipped.
	if _, err := signing.NewVerifier(signing.PurposePlugins, []signing.Key{k}, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestKeysAreCopied(t *testing.T) {
	k, s := signingtest.NewKey(t, signing.PurposePlugins)
	sg := sign(t, s, msg)
	v := newVerifier(t, signing.PurposePlugins, []signing.Key{k}, nil)
	for i := range k.Public {
		k.Public[i] ^= 0xff
	}
	only(t, v.Verify(msg, sg), nil)
}

func TestKeyIDDerivation(t *testing.T) {
	pub := make(ed25519.PublicKey, 32)
	got := signing.KeyID(signing.PurposeCIRelease, pub)
	// sha256 of 32 zero bytes starts 66687aadf862bd77.
	if got != "ci-release-66687aadf862bd77" {
		t.Fatalf("KeyID = %q", got)
	}
	if signing.KeyID(signing.PurposeAgent, pub) == got {
		t.Fatal("purpose is not part of the id")
	}
}

func TestLookupVerifier(t *testing.T) {
	boom := errors.New("registry down")
	for _, a := range allPurposes {
		k, s := signingtest.NewKey(t, a)
		sg := sign(t, s, msg)
		for _, b := range allPurposes {
			var calls []string
			lv, err := signing.NewLookupVerifier(b, func(_ context.Context, id string) (signing.Key, bool, error) {
				calls = append(calls, id)
				if id == k.ID {
					return k, true, nil
				}
				return signing.Key{}, false, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			fixed := newVerifier(t, b, []signing.Key{k}, nil)
			e1, e2 := lv.Verify(msg, sg), fixed.Verify(msg, sg)
			if (e1 == nil) != (e2 == nil) || (e1 != nil && !errors.Is(e1, signing.ErrWrongPurpose)) != (e2 != nil && !errors.Is(e2, signing.ErrWrongPurpose)) {
				t.Fatalf("lookup %v vs fixed %v", e1, e2)
			}
			if a != b {
				only(t, e1, signing.ErrWrongPurpose)
			}
			if len(calls) != 1 || calls[0] != k.ID {
				t.Fatalf("lookup calls = %v", calls)
			}
		}
	}

	k, s := signingtest.NewKey(t, signing.PurposeCINode)
	sg := sign(t, s, msg)
	mk := func(f signing.KeyLookup, opts ...signing.Option) *signing.Verifier {
		v, err := signing.NewLookupVerifier(signing.PurposeCINode, f, opts...)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	// lookup error is wrapped and not a success or a sentinel.
	err := mk(func(context.Context, string) (signing.Key, bool, error) { return k, true, boom }).Verify(msg, sg)
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("lookup error not propagated: %v", err)
	}
	for _, s := range allSentinels {
		if errors.Is(err, s) {
			t.Fatalf("lookup error matched sentinel %v", s)
		}
	}
	// not found, found with other id.
	only(t, mk(func(context.Context, string) (signing.Key, bool, error) { return signing.Key{}, false, nil }).Verify(msg, sg), signing.ErrUnknownKey)
	other := k
	other.ID = "someone-else"
	only(t, mk(func(context.Context, string) (signing.Key, bool, error) { return other, true, nil }).Verify(msg, sg), signing.ErrUnknownKey)
	// returned key with a bad public key length.
	short := k
	short.Public = k.Public[:31]
	only(t, mk(func(context.Context, string) (signing.Key, bool, error) { return short, true, nil }).Verify(msg, sg), signing.ErrMalformed)
	// scope, validity and revocation apply to looked-up keys; a revoked id is not looked up.
	scoped := k
	scoped.Scope = "a"
	only(t, mk(func(context.Context, string) (signing.Key, bool, error) { return scoped, true, nil }, signing.WithScope("b")).Verify(msg, sg), signing.ErrWrongScope)
	called := false
	only(t, mk(func(context.Context, string) (signing.Key, bool, error) { called = true; return k, true, nil },
		signing.WithRevoked(func(string) bool { return true })).Verify(msg, sg), signing.ErrRevoked)
	if called {
		t.Fatal("revoked id reached the lookup")
	}
	// malformed input never reaches the lookup.
	only(t, mk(func(context.Context, string) (signing.Key, bool, error) { called = true; return k, true, nil }).Verify(msg, signing.Signature{KeyID: "bad id", Sig: sg.Sig}), signing.ErrMalformed)
	if called {
		t.Fatal("malformed input reached the lookup")
	}
	// the context reaches the lookup.
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "v")
	var got any
	only(t, mk(func(c context.Context, _ string) (signing.Key, bool, error) {
		got = c.Value(ctxKey{})
		return k, true, nil
	}).VerifyContext(ctx, msg, sg), nil)
	if got != "v" {
		t.Fatal("context not passed")
	}
}

func TestErrorsCarryNoKeyMaterial(t *testing.T) {
	k, s := signingtest.NewKey(t, signing.PurposePlugins)
	sg := sign(t, s, msg)
	sg.Sig[0] ^= 1
	err := newVerifier(t, signing.PurposePlugins, []signing.Key{k}, nil).Verify(msg, sg)
	only(t, err, signing.ErrBadSignature)
	for _, secret := range []string{signing.EncodeSig(sg.Sig), signing.EncodeSig(k.Public), fmt.Sprintf("%x", []byte(k.Public))} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaks %q", secret)
		}
	}
	if !strings.Contains(err.Error(), k.ID) {
		t.Fatal("error should name the key id")
	}
}

// Every byte value is classified: only [A-Za-z0-9._-] is legal in a key id.
func TestKeyIDCharacterClass(t *testing.T) {
	k, s := signingtest.NewKey(t, signing.PurposeAgent)
	sg := sign(t, s, msg)
	v := newVerifier(t, signing.PurposeAgent, []signing.Key{k}, nil)
	for b := 0; b < 256; b++ {
		c := byte(b)
		legal := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-'
		want := signing.ErrMalformed
		if legal {
			want = signing.ErrUnknownKey
		}
		for _, id := range []string{string([]byte{c}), "a" + string([]byte{c}) + "b"} {
			err := v.Verify(msg, signing.Signature{KeyID: id, Sig: sg.Sig})
			if !errors.Is(err, want) {
				t.Fatalf("byte %#x in %q: %v, want %v", b, id, err, want)
			}
		}
	}
}

// Hostile text in errors is cut at 64 bytes; exactly 64 is kept whole.
func TestErrorTextClipped(t *testing.T) {
	_, s := signingtest.NewKey(t, signing.PurposeAgent)
	sg := sign(t, s, msg)
	v := newVerifier(t, signing.PurposeAgent, nil, nil)
	for _, n := range []int{63, 64, 65, 200} {
		id := "/" + strings.Repeat("a", n-1)
		err := v.Verify(msg, signing.Signature{KeyID: id, Sig: sg.Sig})
		only(t, err, signing.ErrMalformed)
		want := fmt.Sprintf("%q", id)
		if n > 64 {
			want = fmt.Sprintf("%q", id[:64]+"...")
		}
		if !strings.HasSuffix(err.Error(), want) {
			t.Fatalf("n=%d: %q does not end with %s", n, err, want)
		}
	}
}
