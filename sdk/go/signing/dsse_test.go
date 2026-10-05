package signing_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/cli/sdk/go/v2/signing/signingtest"
)

const pt = "application/vnd.in-toto+json"

func TestPAEGoldenVectors(t *testing.T) {
	// DSSE v1 specification example.
	if got := string(signing.PAE("http://example.com/HelloWorld", []byte("hello world"))); got != "DSSEv1 29 http://example.com/HelloWorld 11 hello world" {
		t.Fatalf("PAE = %q", got)
	}
	if got := string(signing.PAE("text", nil)); got != "DSSEv1 4 text 0 " {
		t.Fatalf("empty payload PAE = %q", got)
	}
	// Lengths count bytes and carry no leading zeros; the payload is verbatim.
	if got := string(signing.PAE("é", []byte("a b\n\x00"))); got != "DSSEv1 2 é 5 a b\n\x00" {
		t.Fatalf("PAE = %q", got)
	}
	big := make([]byte, 1234)
	if got := string(signing.PAE("t", big)); !strings.HasPrefix(got, "DSSEv1 1 t 1234 ") || len(got) != len("DSSEv1 1 t 1234 ")+1234 {
		t.Fatalf("PAE long = %q", got[:20])
	}
}

func fail(t *testing.T, v *signing.Verifier, env signing.Envelope, want error) {
	t.Helper()
	typ, payload, ids, err := signing.VerifyEnvelope(v, env)
	only(t, err, want)
	if typ != "" || payload != nil || ids != nil {
		t.Fatalf("VerifyEnvelope returned data on failure: %q %q %v", typ, payload, ids)
	}
}

func TestEnvelopeRoundTripAndTamper(t *testing.T) {
	for _, p := range allPurposes {
		k, s := signingtest.NewKey(t, p)
		v := newVerifier(t, p, []signing.Key{k}, nil)
		payload := []byte(`{"hello":"world"}`)
		env, err := signing.SignEnvelope(s, pt, payload)
		if err != nil {
			t.Fatal(err)
		}
		typ, got, ids, err := signing.VerifyEnvelope(v, env)
		if err != nil || typ != pt || string(got) != string(payload) || len(ids) != 1 || ids[0] != k.ID {
			t.Fatalf("%s: %q %q %v %v", p, typ, got, ids, err)
		}
		// Purposes: any other verifier refuses.
		for _, q := range allPurposes {
			if q != p {
				fail(t, newVerifier(t, q, []signing.Key{k}, nil), env, signing.ErrWrongPurpose)
			}
		}
	}
	k, s := signingtest.NewKey(t, signing.PurposeCIRelease)
	v := newVerifier(t, signing.PurposeCIRelease, []signing.Key{k}, nil)
	env, _ := signing.SignEnvelope(s, pt, []byte("payload"))

	a := env
	a.PayloadType = pt + "x"
	fail(t, v, a, signing.ErrBadSignature)
	a = env
	a.Payload = base64.StdEncoding.EncodeToString([]byte("payloaD"))
	fail(t, v, a, signing.ErrBadSignature)
	a = env
	a.Payload = ""
	fail(t, v, a, signing.ErrBadSignature)
	// The signature does not verify as a bare signature over the payload (no PAE).
	raw, _ := s.Sign([]byte("payload"))
	a = env
	a.Signatures = []signing.EnvelopeSig{{KeyID: k.ID, Sig: signing.EncodeSig(raw)}}
	fail(t, v, a, signing.ErrBadSignature)
	// Moving bytes between type and payload changes PAE.
	e2, _ := signing.SignEnvelope(s, "ab", []byte("c"))
	e2.PayloadType, e2.Payload = "a", base64.StdEncoding.EncodeToString([]byte("bc"))
	fail(t, v, e2, signing.ErrBadSignature)
	// Malformed pieces.
	a = env
	a.Payload = "!!!"
	fail(t, v, a, signing.ErrMalformed)
	a = env
	a.Payload = env.Payload + "\n"
	fail(t, v, a, signing.ErrMalformed)
	a = env
	a.Payload = strings.TrimRight(env.Payload, "=")
	fail(t, v, a, signing.ErrMalformed)
	a = env
	a.PayloadType = ""
	fail(t, v, a, signing.ErrMalformed)
	a = env
	a.Signatures = nil
	fail(t, v, a, signing.ErrMalformed)
	a = env
	a.Signatures = []signing.EnvelopeSig{{KeyID: k.ID, Sig: "AAAA"}}
	fail(t, v, a, signing.ErrMalformed)
	a = env
	a.Signatures = []signing.EnvelopeSig{{KeyID: "bad id", Sig: env.Signatures[0].Sig}}
	fail(t, v, a, signing.ErrMalformed)
	a = env
	a.Signatures = []signing.EnvelopeSig{{KeyID: "someone", Sig: env.Signatures[0].Sig}}
	fail(t, v, a, signing.ErrUnknownKey)
	// Revoked key.
	fail(t, newVerifier(t, signing.PurposeCIRelease, []signing.Key{k}, []string{k.ID}), env, signing.ErrRevoked)
	// Nil verifier fails closed.
	fail(t, nil, env, signing.ErrMalformed)
	// Empty payload is legal.
	e0, err := signing.SignEnvelope(s, pt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if typ, p, _, err := signing.VerifyEnvelope(v, e0); err != nil || typ != pt || len(p) != 0 || e0.Payload != "" {
		t.Fatalf("empty payload: %q %q %v", typ, p, err)
	}
}

func TestEnvelopeSignatureSets(t *testing.T) {
	k, s := signingtest.NewKey(t, signing.PurposeCIRelease)
	k2, s2 := signingtest.NewKey(t, signing.PurposeCIRelease)
	other, os := signingtest.NewKey(t, signing.PurposeAgent)
	v := newVerifier(t, signing.PurposeCIRelease, []signing.Key{k, k2, other}, nil)
	env, _ := signing.SignEnvelope(s, pt, []byte("p"))
	good := env.Signatures[0]
	e2, _ := signing.SignEnvelope(s2, pt, []byte("p"))
	good2 := e2.Signatures[0]
	wrongPurpose, _ := signing.SignEnvelope(os, pt, []byte("p"))
	ghost := signing.EnvelopeSig{KeyID: "ghost", Sig: good.Sig}
	badSig := signing.EnvelopeSig{KeyID: k2.ID, Sig: signing.EncodeSig(make([]byte, 64))}
	with := func(sigs ...signing.EnvelopeSig) signing.Envelope { a := env; a.Signatures = sigs; return a }

	// Two valid signatures: both key ids are returned, in envelope order.
	_, _, ids, err := signing.VerifyEnvelope(v, with(good2, good))
	if err != nil || len(ids) != 2 || ids[0] != k2.ID || ids[1] != k.ID {
		t.Fatalf("ids = %v, err = %v", ids, err)
	}
	// An unknown key id is ignored when another signature verifies, in either order.
	for _, sigs := range [][]signing.EnvelopeSig{{ghost, good}, {good, ghost}} {
		_, _, ids, err := signing.VerifyEnvelope(v, with(sigs...))
		if err != nil || len(ids) != 1 || ids[0] != k.ID {
			t.Fatalf("ids = %v, err = %v", ids, err)
		}
	}
	// A signature naming a known key that fails fails the envelope, even beside a good one.
	fail(t, v, with(good, wrongPurpose.Signatures[0]), signing.ErrWrongPurpose)
	fail(t, v, with(wrongPurpose.Signatures[0], good), signing.ErrWrongPurpose)
	fail(t, v, with(good, badSig), signing.ErrBadSignature)
	fail(t, v, with(good, signing.EnvelopeSig{KeyID: k2.ID, Sig: "AAAA"}), signing.ErrMalformed)
	fail(t, v, with(good, signing.EnvelopeSig{KeyID: "bad id", Sig: good.Sig}), signing.ErrMalformed)
	fail(t, newVerifier(t, signing.PurposeCIRelease, []signing.Key{k, k2}, []string{k2.ID}), with(good, good2), signing.ErrRevoked)
	// Only unknown key ids: nothing verified.
	fail(t, v, with(ghost), signing.ErrUnknownKey)
	fail(t, v, with(ghost, signing.EnvelopeSig{KeyID: "ghost2", Sig: good.Sig}), signing.ErrUnknownKey)
	// Deterministic most-specific error, independent of order.
	rv := newVerifier(t, signing.PurposeCIRelease, []signing.Key{k, k2, other}, []string{k2.ID})
	for _, sigs := range [][]signing.EnvelopeSig{
		{wrongPurpose.Signatures[0], good2}, {good2, wrongPurpose.Signatures[0]},
		{good, good2, wrongPurpose.Signatures[0]},
	} {
		fail(t, rv, with(sigs...), signing.ErrRevoked)
	}
	fail(t, v, with(badSig, wrongPurpose.Signatures[0]), signing.ErrWrongPurpose)
	fail(t, v, with(wrongPurpose.Signatures[0], badSig), signing.ErrWrongPurpose)
	fail(t, v, with(signing.EnvelopeSig{KeyID: k.ID, Sig: "AAAA"}, badSig), signing.ErrBadSignature)
	fail(t, v, with(badSig, signing.EnvelopeSig{KeyID: k2.ID + "x", Sig: "AAAA"}), signing.ErrBadSignature)
	// Ties go to the earlier signature.
	badSig1 := signing.EnvelopeSig{KeyID: k.ID, Sig: signing.EncodeSig(make([]byte, 64))}
	for _, c := range []struct{ first, second signing.EnvelopeSig }{{badSig1, badSig}, {badSig, badSig1}} {
		_, _, _, err := signing.VerifyEnvelope(v, with(c.first, c.second))
		if err == nil || !strings.Contains(err.Error(), c.first.KeyID) {
			t.Fatalf("tie not resolved to the first signature (%s): %v", c.first.KeyID, err)
		}
	}
	// Duplicate key ids are ambiguous.
	fail(t, v, with(good, good), signing.ErrMalformed)
	// 16 signatures are allowed, 17 are not.
	sigs := []signing.EnvelopeSig{good}
	for i := 1; i < 16; i++ {
		sigs = append(sigs, signing.EnvelopeSig{KeyID: "x" + strings.Repeat("a", i), Sig: good.Sig})
	}
	if _, _, _, err := signing.VerifyEnvelope(v, with(sigs...)); err != nil {
		t.Fatal(err)
	}
	fail(t, v, with(append(sigs, signing.EnvelopeSig{KeyID: "extra", Sig: good.Sig})...), signing.ErrMalformed)
}

func TestSignEnvelopeErrors(t *testing.T) {
	_, s := signingtest.NewKey(t, signing.PurposeAgent)
	good := signing.KeyID(signing.PurposeAgent, make([]byte, 32))
	for name, f := range map[string]func() error{
		"nil signer": func() error { _, e := signing.SignEnvelope(nil, pt, nil); return e },
		"empty type": func() error { _, e := signing.SignEnvelope(s, "", nil); return e },
		"bad signer": func() error {
			_, e := signing.SignEnvelope(signing.NewEd25519Signer(signing.PurposeAgent, nil), pt, nil)
			return e
		},
		"short sig": func() error {
			_, e := signing.SignEnvelope(fakeSigner{id: good, sig: make([]byte, 10)}, pt, nil)
			return e
		},
		"long sig": func() error {
			_, e := signing.SignEnvelope(fakeSigner{id: good, sig: make([]byte, 65)}, pt, nil)
			return e
		},
		"syntactically bad id": func() error {
			_, e := signing.SignEnvelope(fakeSigner{id: "a b", sig: make([]byte, 64)}, pt, nil)
			return e
		},
		"chosen (non-derived) id": func() error {
			_, e := signing.SignEnvelope(fakeSigner{id: "release-2026", sig: make([]byte, 64)}, pt, nil)
			return e
		},
		"upper-case derived id": func() error {
			_, e := signing.SignEnvelope(fakeSigner{id: strings.ToUpper(good), sig: make([]byte, 64)}, pt, nil)
			return e
		},
	} {
		t.Run(name, func(t *testing.T) { only(t, f(), signing.ErrMalformed) })
	}
	// A derived id with a 64-byte signature is accepted.
	if _, err := signing.SignEnvelope(fakeSigner{id: good, sig: make([]byte, 64)}, pt, nil); err != nil {
		t.Fatal(err)
	}
}

// Codex round 3: unknown key ids are ignored even with an undecodable
// signature; known key ids are classified before decoding, so the ranked error
// does not depend on whether the signature decodes.
func TestEnvelopeKeyClassificationBeforeDecoding(t *testing.T) {
	k, s := signingtest.NewKey(t, signing.PurposeCIRelease)
	k2, _ := signingtest.NewKey(t, signing.PurposeCIRelease)
	other, _ := signingtest.NewKey(t, signing.PurposeAgent)
	env, _ := signing.SignEnvelope(s, pt, []byte("p"))
	good := env.Signatures[0]
	with := func(sigs ...signing.EnvelopeSig) signing.Envelope { a := env; a.Signatures = sigs; return a }
	junk := []string{"AAAA", "!!!", "", good.Sig + "\n", strings.TrimRight(good.Sig, "="), signing.EncodeSig(make([]byte, 63))}
	v := newVerifier(t, signing.PurposeCIRelease, []signing.Key{k, k2, other}, []string{k2.ID})
	ghostID := signing.KeyID(signing.PurposeCIRelease, make([]byte, 32))

	for _, j := range junk {
		// 1. Unknown id with a malformed signature is ignored beside a good one.
		_, _, ids, err := signing.VerifyEnvelope(v, with(good, signing.EnvelopeSig{KeyID: ghostID, Sig: j}))
		if err != nil || len(ids) != 1 || ids[0] != k.ID {
			t.Fatalf("junk %q: ids %v err %v", j, ids, err)
		}
		// ... and alone it is only "unknown".
		fail(t, v, with(signing.EnvelopeSig{KeyID: ghostID, Sig: j}), signing.ErrUnknownKey)
		// 2. A revoked known id outranks malformed whatever its signature looks like.
		fail(t, v, with(good, signing.EnvelopeSig{KeyID: k2.ID, Sig: j}), signing.ErrRevoked)
		fail(t, v, with(signing.EnvelopeSig{KeyID: k2.ID, Sig: j}, good), signing.ErrRevoked)
		// A wrong-purpose known id outranks malformed likewise.
		fail(t, v, with(good, signing.EnvelopeSig{KeyID: other.ID, Sig: j}), signing.ErrWrongPurpose)
		// A usable known key with an undecodable signature is malformed.
		fail(t, v, with(signing.EnvelopeSig{KeyID: k.ID, Sig: j}), signing.ErrMalformed)
	}
	// Direct Verify: key-level errors precede a short signature.
	sh := func(id string) error { return v.Verify(msg, signing.Signature{KeyID: id, Sig: []byte{1}}) }
	only(t, sh(k2.ID), signing.ErrRevoked)
	only(t, sh(other.ID), signing.ErrWrongPurpose)
	only(t, sh(ghostID), signing.ErrUnknownKey)
	only(t, sh(k.ID), signing.ErrMalformed)
}

// The documented ranking holds for every pair of failure classes, in either
// envelope order.
func TestEnvelopeErrorRankingEveryPair(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(p signing.Purpose, mut func(*signing.Key)) (signing.Key, signing.EnvelopeSig) {
		k, s := signingtest.NewKey(t, p)
		mut(&k)
		e, _ := signing.SignEnvelope(s, pt, []byte("p"))
		return k, e.Signatures[0]
	}
	type class struct {
		name string
		want error
		key  signing.Key
		sig  signing.EnvelopeSig
		rev  bool
	}
	var cs []class
	add := func(name string, want error, p signing.Purpose, mut func(*signing.Key), rev bool, sigmod func(*signing.EnvelopeSig)) {
		k, sg := mk(p, mut)
		if sigmod != nil {
			sigmod(&sg)
		}
		cs = append(cs, class{name, want, k, sg, rev})
	}
	add("revoked", signing.ErrRevoked, signing.PurposeCIRelease, func(*signing.Key) {}, true, func(e *signing.EnvelopeSig) { e.Sig = "AAAA" })
	add("expired", signing.ErrExpired, signing.PurposeCIRelease, func(k *signing.Key) { k.NotAfter = now.Add(-time.Hour) }, false, func(e *signing.EnvelopeSig) { e.Sig = "AAAA" })
	add("notyet", signing.ErrNotYetValid, signing.PurposeCIRelease, func(k *signing.Key) { k.NotBefore = now.Add(time.Hour) }, false, nil)
	add("purpose", signing.ErrWrongPurpose, signing.PurposeAgent, func(*signing.Key) {}, false, nil)
	add("scope", signing.ErrWrongScope, signing.PurposeCIRelease, func(k *signing.Key) { k.Scope = "other" }, false, nil)
	add("badsig", signing.ErrBadSignature, signing.PurposeCIRelease, func(*signing.Key) {}, false, func(e *signing.EnvelopeSig) { e.Sig = signing.EncodeSig(make([]byte, 64)) })
	add("malformed", signing.ErrMalformed, signing.PurposeCIRelease, func(*signing.Key) {}, false, func(e *signing.EnvelopeSig) { e.Sig = "AAAA" })

	var keys []signing.Key
	var revoked []string
	for _, c := range cs {
		keys = append(keys, c.key)
		if c.rev {
			revoked = append(revoked, c.key.ID)
		}
	}
	keys2 := make([]signing.Key, len(keys))
	for i, c := range cs {
		keys2[i] = c.key
		if c.name != "scope" {
			keys2[i].Scope = "main"
		}
	}
	v := newVerifier(t, signing.PurposeCIRelease, keys2, revoked, signing.WithScope("main"),
		signing.WithClock(func() time.Time { return now }))
	// Each class alone yields its own error (guards the table itself).
	env := signing.Envelope{PayloadType: pt, Payload: base64.StdEncoding.EncodeToString([]byte("p"))}
	for _, c := range cs {
		e := env
		e.Signatures = []signing.EnvelopeSig{c.sig}
		fail(t, v, e, c.want)
	}
	for i := range cs {
		for j := range cs {
			if i == j {
				continue
			}
			want := cs[i].want // lower index = more specific
			if j < i {
				want = cs[j].want
			}
			e := env
			e.Signatures = []signing.EnvelopeSig{cs[i].sig, cs[j].sig}
			fail(t, v, e, want)
		}
	}
}

type fakeSigner struct {
	id  string
	sig []byte
}

func (f fakeSigner) KeyID() string               { return f.id }
func (f fakeSigner) Sign([]byte) ([]byte, error) { return f.sig, nil }

func TestEnvelopeJSONShape(t *testing.T) {
	_, s := signingtest.NewKey(t, signing.PurposeAgent)
	env, _ := signing.SignEnvelope(s, "t", []byte("p"))
	b, _ := json.Marshal(env)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["payloadType"] != "t" || m["payload"] != "cA==" {
		t.Fatalf("json = %s", b)
	}
	sigs := m["signatures"].([]any)[0].(map[string]any)
	if sigs["keyid"] != s.KeyID() || len(sigs["sig"].(string)) != 88 {
		t.Fatalf("json = %s", b)
	}
}
