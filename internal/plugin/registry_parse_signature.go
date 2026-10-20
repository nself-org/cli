package plugin

// registry_parse_signature.go — decode and re-encode the registry's release_signature and platform_signatures.
//
// Purpose: carry the release_signature object (hq PLUG D7) and the entry-level platform_signatures map (contract v1.1, P7-PLUG-84) from a registry or catalog entry onto PluginManifest and through the registry cache, split out of registry_parse.go (300 lines) for file size.
// Inputs: the raw JSON of an entry's release_signature and platform_signatures keys.
// Outputs: a *signature.Release, or nil when the entry has none; a map platform -> *signature.Release, or nil when the entry has none.
// Constraints: absent, null and an empty value are "no signature" (nil). Any other value that is not an object of three strings decodes to a non-nil EMPTY Release, never to nil, so a mangled signature refuses the install (E121) instead of reading as unsigned. platform_signatures: a value that is not an object, or a platform value that is null or malformed, becomes present-but-invalid (empty Release) for the affected platform names (all five when the whole value is unreadable); a key that is not one of the five platform names is kept but never consulted (the installer only asks for its own platform). The cache round trip keeps empty values empty.

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
)

// pluginEntrySignatures is embedded in pluginEntry so encoding/json promotes
// both keys without adding a line to registry_parse.go.
type pluginEntrySignatures struct {
	ReleaseSignature   json.RawMessage `json:"release_signature,omitempty"`
	PlatformSignatures json.RawMessage `json:"platform_signatures,omitempty"`
}

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
		return json.RawMessage(`{}`) // stays present-but-invalid (E121), never "absent"
	}
	return b
}

// platformSignaturesFromEntry decodes an entry's platform_signatures. It never
// returns a nil value inside the map and never turns a problem into "absent".
func platformSignaturesFromEntry(raw json.RawMessage) map[string]*signature.Release {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &entries); err != nil || entries == nil {
		out := make(map[string]*signature.Release, len(signature.Platforms))
		for _, p := range signature.Platforms {
			out[p] = &signature.Release{}
		}
		return out
	}
	out := map[string]*signature.Release{}
	for p, v := range entries {
		rel, err := parseReleaseSignature(v)
		if err != nil || rel == nil {
			rel = &signature.Release{}
		}
		out[p] = rel
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// platformSignaturesRaw is the inverse, for Registry.MarshalJSON (the cache).
func platformSignaturesRaw(sigs map[string]*signature.Release) json.RawMessage {
	if len(sigs) == 0 {
		return nil
	}
	b, err := json.Marshal(sigs)
	if err != nil {
		return json.RawMessage(`"unreadable"`) // reads back as unreadable: invalid for every platform
	}
	return b
}
