package vectorsearch

// metadata.go mirrors the tracked function into on-disk Hasura metadata after
// a live `--track`. It edits a file only; it never calls Hasura and never
// applies metadata. Two layouts are written, the two that database.HasuraDiffMetadata
// (or the JSON compare) can prove clean: the sources.yaml written by
// `nself db hasura metadata export`, and the legacy hasura/metadata.json. Any
// other layout is left unchanged and reported.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// defaultSource is the Hasura source nSelf tracks tables and functions in.
const defaultSource = "default"

// ErrUnsupportedLayout means the on-disk metadata is not in a layout this
// package edits. The caller leaves disk untouched and prints a note.
var ErrUnsupportedLayout = errors.New("on-disk metadata layout is not supported for mirroring")

// metadataPaths returns the sources.yaml and metadata.json paths and the YAML
// metadata directory of a project.
func metadataPaths(projectDir string) (yamlFile, jsonFile, yamlDir string) {
	h := filepath.Join(projectDir, "hasura")
	return filepath.Join(h, "metadata", "sources.yaml"), filepath.Join(h, "metadata.json"), filepath.Join(h, "metadata")
}

// MirrorFunction adds the function and its role permissions to the on-disk
// metadata of projectDir. It returns the file it wrote and whether anything
// changed (false when the entry already holds every role). The edit is
// idempotent. ErrUnsupportedLayout means nothing was written.
func MirrorFunction(projectDir, schema, name string, roles []string) (file string, changed bool, err error) {
	yamlFile, jsonFile, yamlDir := metadataPaths(projectDir)
	if st, statErr := os.Stat(yamlDir); statErr == nil && st.IsDir() {
		if _, e := os.Stat(yamlFile); e != nil {
			return "", false, fmt.Errorf("%w: %s has no sources.yaml", ErrUnsupportedLayout, yamlDir)
		}
		changed, err = editFile(yamlFile, unmarshalYAML, marshalYAML, schema, name, roles)
		return yamlFile, changed, err
	}
	if _, e := os.Stat(jsonFile); e != nil {
		return "", false, fmt.Errorf("%w: no hasura/metadata.json", ErrUnsupportedLayout)
	}
	changed, err = editFile(jsonFile, unmarshalJSON, marshalJSON, schema, name, roles)
	return jsonFile, changed, err
}

// editFile reads path, adds the function entry to the default source and writes
// the file back only when the entry changed. In sources.yaml the document is
// the sources list; in metadata.json it is an object holding "sources".
func editFile(path string, dec func([]byte) (any, error), enc func(any) ([]byte, error), schema, name string, roles []string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	doc, err := dec(raw)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}
	sources, holder, key := sourcesOf(doc)
	changed, newSources, err := addFunction(sources, schema, name, roles)
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if !changed {
		return false, nil
	}
	if holder != nil {
		holder[key] = newSources
		doc = holder
	} else {
		doc = newSources
	}
	out, err := enc(doc)
	if err != nil {
		return false, fmt.Errorf("encode %s: %w", path, err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// sourcesOf returns the sources list of a decoded document. For an object it
// also returns the object and the key so the edited list can be put back.
func sourcesOf(doc any) (sources []any, holder map[string]any, key string) {
	switch d := doc.(type) {
	case []any:
		return d, nil, ""
	case map[string]any:
		list, _ := d["sources"].([]any)
		return list, d, "sources"
	}
	return nil, nil, ""
}

// addFunction puts the function entry into the default source of sources.
func addFunction(sources []any, schema, name string, roles []string) (bool, []any, error) {
	for _, s := range sources {
		src, ok := s.(map[string]any)
		if !ok || src["name"] != defaultSource {
			continue
		}
		funcs, _ := src["functions"].([]any)
		changed := false
		idx := -1
		for i, f := range funcs {
			if isFunction(f, schema, name) {
				idx = i
			}
		}
		if idx < 0 {
			funcs = append(funcs, map[string]any{"function": map[string]any{"name": name, "schema": schema}})
			idx = len(funcs) - 1
			changed = true
		}
		entry := funcs[idx].(map[string]any)
		perms, _ := entry["permissions"].([]any)
		have := map[string]bool{}
		for _, p := range perms {
			if pm, ok := p.(map[string]any); ok {
				if r, ok := pm["role"].(string); ok {
					have[r] = true
				}
			}
		}
		for _, r := range roles {
			if !have[r] {
				perms = append(perms, map[string]any{"role": r})
				changed = true
			}
		}
		if len(perms) > 0 {
			entry["permissions"] = perms
		}
		src["functions"] = funcs
		return changed, sources, nil
	}
	return false, nil, fmt.Errorf("%w: no %q source in the metadata", ErrUnsupportedLayout, defaultSource)
}

// isFunction reports whether f is the function entry for schema.name.
func isFunction(f any, schema, name string) bool {
	m, ok := f.(map[string]any)
	if !ok {
		return false
	}
	ref, ok := m["function"].(map[string]any)
	return ok && ref["schema"] == schema && ref["name"] == name
}

func unmarshalYAML(b []byte) (any, error) {
	var v any
	err := yaml.Unmarshal(b, &v)
	return v, err
}

func marshalYAML(v any) ([]byte, error) { return yaml.Marshal(v) }

// unmarshalJSON keeps numbers exact so a rewrite does not change them.
func unmarshalJSON(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// marshalJSON writes canonical JSON: sorted keys, two-space indent, trailing
// newline, no HTML escaping.
func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
