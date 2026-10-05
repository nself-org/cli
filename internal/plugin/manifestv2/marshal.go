package manifestv2

import (
	"bytes"
	"encoding/json"
)

// Marshal renders m as the canonical plugin.json: keys sorted at every level,
// 2-space indent, trailing newline, HTML never escaped.
func Marshal(m *Manifest) ([]byte, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return Canonical(raw)
}

// Canonical re-encodes any JSON document with sorted keys, 2-space indent and a
// trailing newline. Numbers keep their literal text.
func Canonical(data []byte) ([]byte, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// compatKeys are the JSON keys ApplyCompat owns.
var compatKeys = []string{"pluginType", "binaryName", "cliCommands", "entryPoint", "runtime", "cli",
	"minNselfVersion", "status", "isCommercial", "licenseType", "requires_license", "tier"}

// ApplyCompat returns the canonical bytes of a v2 file with only its
// compatibility keys rewritten to Projection (E112 is what a stale file fails).
// Every other key keeps its value.
func ApplyCompat(data []byte) ([]byte, error) {
	m, err := DecodeV2(data)
	if err != nil {
		return nil, err
	}
	ApplyProjection(m)
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	for _, k := range compatKeys {
		delete(doc, k)
	}
	pb, err := json.Marshal(m.Compat)
	if err != nil {
		return nil, err
	}
	var proj map[string]json.RawMessage
	if err := json.Unmarshal(pb, &proj); err != nil {
		return nil, err
	}
	for k, v := range proj {
		doc[k] = v
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return Canonical(out)
}
