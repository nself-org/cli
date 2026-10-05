package main

// output.go: canonical JSON bytes and the write or check step.
//
// Purpose: every output file is rendered the same way (Constitution 4.4): keys
// sorted at every level, 2-space indent, trailing newline, no HTML escaping, so
// identical inputs give byte-identical files.
// Inputs: a value to render, an output directory, the check flag.
// Outputs: file bytes; files written, or the names of files that differ.
// Constraints: -check never writes.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// canonical renders v with sorted keys. It round-trips through a generic value
// because Go marshals maps in key order but structs in field order.
func canonical(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(generic); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// outputs maps a file name to its bytes.
type outputs map[string][]byte

func (o outputs) add(name string, v any) error {
	b, err := canonical(v)
	if err != nil {
		return fmt.Errorf("render %s: %w", name, err)
	}
	o[name] = b
	return nil
}

// names returns the file names in sorted order.
func (o outputs) names() []string {
	out := make([]string, 0, len(o))
	for n := range o {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// write stores every output under dir.
func (o outputs) write(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, n := range o.names() {
		if err := os.WriteFile(filepath.Join(dir, n), o[n], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// differing lists the outputs whose file under dir is missing or not equal.
func (o outputs) differing(dir string) []string {
	var out []string
	for _, n := range o.names() {
		have, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil || !bytes.Equal(have, o[n]) {
			out = append(out, n)
		}
	}
	return out
}
