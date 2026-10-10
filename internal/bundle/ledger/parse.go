// Parse: read and strictly validate ledger bytes.
//
// Purpose: turn a ledger file into a Ledger, refusing anything the schema or the
// contract refuses: a first key other than _generated, duplicate keys at any
// depth, unknown keys, missing or null required keys, trailing data, oversize
// input, and every Validate failure. A damaged ledger is reported, never
// replaced by a plausible default.
// Inputs: the file bytes. Outputs: a Ledger or an error.
// Constraints: pure; no file access (Store reads and caps the file).
package ledger

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Parse reads and validates ledger bytes. Unknown keys, a first key other than
// _generated, trailing data and every Validate failure are errors: a damaged
// ledger is reported, never replaced by a plausible default.
func Parse(data []byte) (Ledger, error) {
	if len(data) > MaxBytes {
		return Ledger{}, fmt.Errorf("ledger: file is larger than %d bytes", MaxBytes)
	}
	if err := checkStructure(data); err != nil {
		return Ledger{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var l Ledger
	if err := dec.Decode(&l); err != nil {
		return Ledger{}, fmt.Errorf("ledger: decode: %w", err)
	}
	if dec.More() {
		return Ledger{}, fmt.Errorf("ledger: trailing data after the document")
	}
	if err := checkRequired(data); err != nil {
		return Ledger{}, err
	}
	if err := l.Validate(); err != nil {
		return Ledger{}, err
	}
	l.normalize()
	return l, nil
}

// requireValue fails when a required key is absent or JSON null (null decodes
// as a no-op, so a null "explicit" would read false).
func requireValue(raw json.RawMessage, where, key string) error {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		if where != "" {
			return fmt.Errorf("ledger: %s: %q is missing or null", where, key)
		}
		return fmt.Errorf("ledger: %q is missing or null", key)
	}
	return nil
}

// checkRequired mirrors the schema's required lists: a missing or null key must
// not decode silently to its zero value (a missing "explicit" would read false).
func checkRequired(data []byte) error {
	var doc struct {
		Bundles map[string]map[string]json.RawMessage `json:"bundles"`
		Plugins map[string]map[string]json.RawMessage `json:"plugins"`
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("ledger: decode: %w", err)
	}
	for _, k := range []string{"_generated", "schema_version", "bundles", "plugins"} {
		if err := requireValue(root[k], "", k); err != nil {
			return err
		}
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("ledger: decode: %w", err)
	}
	for slug, rec := range doc.Bundles {
		if err := requireValue(rec["installed_at"], "bundle "+slug, "installed_at"); err != nil {
			return err
		}
	}
	for slug, rec := range doc.Plugins {
		for _, k := range []string{"installed_by", "explicit", "tier", "version", "checksum"} {
			if err := requireValue(rec[k], "plugin "+slug, k); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkStructure walks every token of the document. The first key must be
// _generated, and no object, at any depth, may repeat a key (encoding/json
// would silently keep the last one).
func checkStructure(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return fmt.Errorf("ledger: not a JSON object")
	}
	if err := walkObject(dec, nil); err != nil {
		return err
	}
	return nil
}

// Exact, case-sensitive key sets of the struct-shaped levels. encoding/json
// matches struct fields case-insensitively, so "EXPLICIT" would silently
// override "explicit"; the schema rejects such a key and so must Parse.
var (
	rootKeys   = keySet("_generated", "schema_version", "bundles", "plugins")
	bundleKeys = keySet("installed_at")
	pluginKeys = keySet("installed_by", "explicit", "tier", "version", "checksum")
)

func keySet(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// allowedKeys returns the permitted keys of the object at path, or nil when the
// level is a free-form map (bundle and plugin slugs) or not part of the schema.
func allowedKeys(path []string) map[string]bool {
	switch {
	case len(path) == 0:
		return rootKeys
	case len(path) == 2 && path[0] == "bundles":
		return bundleKeys
	case len(path) == 2 && path[0] == "plugins":
		return pluginKeys
	}
	return nil
}

// walkObject reads object members up to and including the closing brace. The
// opening brace is already consumed. path holds the keys leading here.
func walkObject(dec *json.Decoder, path []string) error {
	seen := map[string]bool{}
	allowed := allowedKeys(path)
	for first := true; dec.More(); first = false {
		kt, err := dec.Token()
		if err != nil {
			return fmt.Errorf("ledger: decode: %w", err)
		}
		key, _ := kt.(string)
		if len(path) == 0 && first && key != "_generated" {
			return fmt.Errorf("ledger: first key must be _generated")
		}
		if allowed != nil && !allowed[key] {
			return fmt.Errorf("ledger: unknown key %q (keys are exact and case-sensitive)", key)
		}
		if seen[key] {
			return fmt.Errorf("ledger: duplicate key %q", key)
		}
		seen[key] = true
		if err := walkValue(dec, append(append([]string(nil), path...), key)); err != nil {
			return err
		}
	}
	if len(path) == 0 && len(seen) == 0 {
		return fmt.Errorf("ledger: first key must be _generated")
	}
	_, err := dec.Token() // closing brace
	return err
}

// walkValue reads one value, recursing into objects and arrays.
func walkValue(dec *json.Decoder, path []string) error {
	t, err := dec.Token()
	if err != nil {
		return fmt.Errorf("ledger: decode: %w", err)
	}
	switch t {
	case json.Delim('{'):
		return walkObject(dec, path)
	case json.Delim('['):
		for dec.More() {
			if err := walkValue(dec, append(append([]string(nil), path...), "[]")); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	}
	return nil
}
