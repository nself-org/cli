package signing

import (
	"encoding/base64"
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
	sig, err := s.Sign(PAE(payloadType, payload))
	if err != nil {
		return Envelope{}, err
	}
	if len(sig) != 64 || !validKeyID(s.KeyID()) {
		return Envelope{}, fmt.Errorf("%w: signer %q produced an invalid signature", ErrMalformed, clip(s.KeyID()))
	}
	return Envelope{
		PayloadType: payloadType,
		Payload:     base64.StdEncoding.EncodeToString(payload),
		Signatures:  []EnvelopeSig{{KeyID: s.KeyID(), Sig: EncodeSig(sig)}},
	}, nil
}

// VerifyEnvelope accepts env when at least one signature verifies under v over
// PAE(payloadType, payload). It returns the payload type and decoded payload
// only on success; on any failure both are zero.
func VerifyEnvelope(v *Verifier, env Envelope) (string, []byte, error) {
	if env.PayloadType == "" {
		return "", nil, fmt.Errorf("%w: empty payloadType", ErrMalformed)
	}
	n := len(env.Signatures)
	if n == 0 || n > maxEnvelopeSigs {
		return "", nil, fmt.Errorf("%w: %d signatures", ErrMalformed, n)
	}
	payload, err := decodeStrict(env.Payload)
	if err != nil {
		return "", nil, fmt.Errorf("%w: payload base64", ErrMalformed)
	}
	seen := make(map[string]bool, n)
	for _, s := range env.Signatures {
		if seen[s.KeyID] {
			return "", nil, fmt.Errorf("%w: duplicate signature key id %q", ErrMalformed, clip(s.KeyID))
		}
		seen[s.KeyID] = true
	}
	pae := PAE(env.PayloadType, payload)
	var first error
	for _, s := range env.Signatures {
		raw, derr := DecodeSig(s.Sig)
		if derr == nil {
			derr = v.Verify(pae, Signature{KeyID: s.KeyID, Sig: raw})
		}
		if derr == nil {
			return env.PayloadType, payload, nil
		}
		if first == nil {
			first = derr
		}
	}
	return "", nil, first
}
