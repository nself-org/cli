package signing

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
)

const maxEnvelopeSigs = 16

// EnvelopeSig is one DSSE signature; Sig is base64 standard.
type EnvelopeSig struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}

// Envelope is a DSSE v1 envelope; Payload is base64 standard.
type Envelope struct {
	PayloadType string        `json:"payloadType"`
	Payload     string        `json:"payload"`
	Signatures  []EnvelopeSig `json:"signatures"`
}

// PAE is the DSSE v1 pre-authentication encoding:
// "DSSEv1 <len(type)> <type> <len(payload)> <payload>".
func PAE(payloadType string, payload []byte) []byte {
	out := make([]byte, 0, 24+len(payloadType)+len(payload))
	out = append(out, "DSSEv1 "...)
	out = strconv.AppendInt(out, int64(len(payloadType)), 10)
	out = append(out, ' ')
	out = append(out, payloadType...)
	out = append(out, ' ')
	out = strconv.AppendInt(out, int64(len(payload)), 10)
	out = append(out, ' ')
	return append(out, payload...)
}

// SignEnvelope signs payload under payloadType with s.
func SignEnvelope(s Signer, payloadType string, payload []byte) (Envelope, error) {
	if s == nil {
		return Envelope{}, fmt.Errorf("%w: nil signer", ErrMalformed)
	}
	if payloadType == "" {
		return Envelope{}, fmt.Errorf("%w: empty payloadType", ErrMalformed)
	}
	if !derivedIDShape(s.KeyID()) {
		return Envelope{}, fmt.Errorf("%w: signer key id %q is not a derived id", ErrMalformed, clip(s.KeyID()))
	}
	sig, err := s.Sign(PAE(payloadType, payload))
	if err != nil {
		return Envelope{}, err
	}
	if len(sig) != 64 {
		return Envelope{}, fmt.Errorf("%w: signer %q produced a %d-byte signature", ErrMalformed, s.KeyID(), len(sig))
	}
	return Envelope{
		PayloadType: payloadType,
		Payload:     base64.StdEncoding.EncodeToString(payload),
		Signatures:  []EnvelopeSig{{KeyID: s.KeyID(), Sig: EncodeSig(sig)}},
	}, nil
}

// errRank orders failure classes, most specific first. Unknown key ids are not
// failures (see VerifyEnvelope); anything not listed ranks last.
var errRank = []error{ErrRevoked, ErrExpired, ErrNotYetValid, ErrWrongPurpose, ErrWrongScope, ErrBadSignature, ErrMalformed}

func rank(err error) int {
	for i, s := range errRank {
		if errors.Is(err, s) {
			return i
		}
	}
	return len(errRank)
}

// VerifyEnvelope accepts env only when at least one signature verifies under a
// trusted key of the verifier's purpose AND no signature that names a known key
// id fails. Signatures by unknown key ids are ignored (key rotation: an old
// envelope may carry a signature this verifier has no key for). A signature
// that is malformed, or names a known key that is revoked, expired, of another
// purpose or scope, or does not verify, fails the whole envelope.
//
// On success it returns the payload type, the decoded payload and the key ids
// that verified, in envelope order. On any failure it returns nothing but the
// error: the most specific among the failures (revoked, expired, not yet
// valid, wrong purpose, wrong scope, bad signature, malformed, anything else;
// ties go to the earlier signature), or ErrUnknownKey when every signature
// named an unknown key.
func VerifyEnvelope(v *Verifier, env Envelope) (string, []byte, []string, error) {
	if env.PayloadType == "" {
		return "", nil, nil, fmt.Errorf("%w: empty payloadType", ErrMalformed)
	}
	n := len(env.Signatures)
	if n == 0 || n > maxEnvelopeSigs {
		return "", nil, nil, fmt.Errorf("%w: %d signatures", ErrMalformed, n)
	}
	payload, err := decodeStrict(env.Payload)
	if err != nil {
		return "", nil, nil, fmt.Errorf("%w: payload base64", ErrMalformed)
	}
	seen := make(map[string]bool, n)
	for _, s := range env.Signatures {
		if seen[s.KeyID] {
			return "", nil, nil, fmt.Errorf("%w: duplicate signature key id %q", ErrMalformed, clip(s.KeyID))
		}
		seen[s.KeyID] = true
	}
	pae := PAE(env.PayloadType, payload)
	var ok []string
	var worst error
	for _, s := range env.Signatures {
		// A signature that does not decode is verified as empty bytes: the
		// verifier then reports key-level errors first (unknown key ignored,
		// revoked and the rest ranked) and ErrMalformed only for a usable key.
		raw, derr := DecodeSig(s.Sig)
		if derr != nil {
			raw = nil
		}
		derr = v.Verify(pae, Signature{KeyID: s.KeyID, Sig: raw})
		if derr == nil {
			ok = append(ok, s.KeyID)
			continue
		}
		if errors.Is(derr, ErrUnknownKey) {
			continue
		}
		if worst == nil || rank(derr) < rank(worst) {
			worst = derr
		}
	}
	if worst != nil {
		return "", nil, nil, worst
	}
	if len(ok) == 0 {
		// Every signature (there is at least one) named an unknown key.
		return "", nil, nil, fmt.Errorf("%w: no signature by a known key", ErrUnknownKey)
	}
	return env.PayloadType, payload, ok, nil
}
