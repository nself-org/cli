package plugin

// registry_parse_signature.go — decode and re-encode the registry's release_signature.
//
// Purpose: carry the release_signature object (hq PLUG D7) from a registry or catalog entry onto PluginManifest and through the registry cache, split out of registry_parse.go (299 lines) for file size.
// Inputs: the raw JSON of an entry's release_signature key.
// Outputs: a *signature.Release, or nil when the entry has none.
// Constraints: absent, null and an empty value are "no signature" (nil). Any other value that is not an object of three strings decodes to a non-nil EMPTY Release, never to nil, so a mangled signature refuses the install (E121) instead of reading as unsigned. The cache round trip keeps the empty value empty.

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
)

// parseReleaseSignature decodes one release_signature value. nil, an empty
// value and JSON null give (nil, nil); an object gives its three fields; any
// other JSON, or an object with a non-string field, is an error.
func parseReleaseSignature(raw json.RawMessage) (*signature.Release, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	var rel signature.Release
	if err := json.Unmarshal(trimmed, &rel); err != nil {
		return nil, fmt.Errorf("release_signature: %w", err)
	}
	return &rel, nil
}

// releaseSignatureFromEntry is parseReleaseSignature for entryToManifest: a
// decode error becomes a non-nil empty Release (see the file constraints).
func releaseSignatureFromEntry(raw json.RawMessage) *signature.Release {
	rel, err := parseReleaseSignature(raw)
	if err != nil {
		return &signature.Release{}
	}
	return rel
}

// releaseSignatureRaw is the inverse, for Registry.MarshalJSON (the cache).
func releaseSignatureRaw(rel *signature.Release) json.RawMessage {
	if rel == nil {
		return nil
	}
	b, err := json.Marshal(rel)
	if err != nil {
		return nil
	}
	return b
}
