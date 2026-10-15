package license

// revocation_verify.go — canonical JSON and signature verification for the revocation list.
//
// Purpose: produce the canonical JSON byte form the server signs and verify the ed25519 signature over it, used by RefreshRevocationList in revocation.go, split out for file size.
// Inputs: a RevocationList and its detached signature.
// Outputs: canonical JSON bytes, or a verification error.
// Constraints: wire format matches web/backend/services/ping_api's revocation-list.ts;
// sdk/go/signing rejects malformed and small-order verification keys.

import (
	"encoding/base64"
	"encoding/json"

	"sort"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/signing"
)

// canonicalJSON produces the same byte sequence the server signed:
// objects with sorted keys at every depth, arrays in declared order.
func canonicalJSON(v any) ([]byte, error) {
	var b strings.Builder
	if err := writeCanonical(&b, v); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func writeCanonical(b *strings.Builder, v any) error {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		out, err := json.Marshal(t)
		if err != nil {
			return err
		}
		b.Write(out)
	case float64:
		out, err := json.Marshal(t)
		if err != nil {
			return err
		}
		b.Write(out)
	case json.Number:
		b.WriteString(string(t))
	case []any:
		b.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeCanonical(b, item); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			kj, err := json.Marshal(k)
			if err != nil {
				return err
			}
			b.Write(kj)
			b.WriteByte(':')
			if err := writeCanonical(b, t[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		// Fall back to standard JSON encoding for other types — re-decode
		// through json.Number so nested structures are also canonicalised.
		buf, err := json.Marshal(t)
		if err != nil {
			return err
		}
		dec := json.NewDecoder(strings.NewReader(string(buf)))
		dec.UseNumber()
		var generic any
		if err := dec.Decode(&generic); err != nil {
			return err
		}
		return writeCanonical(b, generic)
	}
	return nil
}

// canonicalForVerify reproduces the bytes the server signed: the full
// payload object minus the signature field.
func canonicalForVerify(list *RevocationList) ([]byte, error) {
	// Marshal then re-decode through json.Number so the canonicalizer
	// preserves numeric precision and key ordering at every depth.
	raw, err := json.Marshal(list)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var generic map[string]any
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}
	delete(generic, "signature")
	return canonicalJSON(generic)
}

// ─── Signature verification ──────────────────────────────────────────────────

// VerifyRevocationSignature checks the Ed25519 signature on `list` against
// the bundled license public keys.  Returns true on a valid signature.
func VerifyRevocationSignature(list *RevocationList) bool {
	if list == nil || list.Signature == "" {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(list.Signature)
	if err != nil {
		return false
	}
	canonical, err := canonicalForVerify(list)
	if err != nil {
		return false
	}
	keys := GetPublicKeys()
	for _, pk := range keys {
		if signing.VerifyEd25519(pk.Key, canonical, sig) == nil {
			return true
		}
	}
	return false
}
