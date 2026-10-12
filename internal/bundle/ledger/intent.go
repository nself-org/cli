// intent.go — the durable remove intent of `nself bundle remove` (D-0368).
//
// Purpose: a bundle remove deletes members one by one. This sidecar file, next
// to the ledger, says which bundle is being removed and which members are not
// yet gone, so a kill between members (or a failed final ledger write) can be
// resumed by running the same command again.
// Inputs: Tx methods (they run under the writer lock only).
// Outputs: <pluginDir>/.nself-state/remove-intent.json:
//
//	{"_generated": ..., "schema_version": 1,
//	 "bundles": {"<slug>": {"started_at": "<RFC3339 UTC>", "remaining": ["a", "b"]}}}
//
// Constraints: machine-owned and strict like the ledger (unknown or repeated
// keys, bad slugs, bad times, trailing data and files over MaxBytes are
// errors that name the file, never a plausible default). A missing file means
// no intents. The ledger file format is not touched. The file is 0644.
package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	intentFileName    = "remove-intent.json"
	intentTempPattern = ".remove-intent.json.*.tmp"
)

// RemoveIntent is one bundle remove in progress.
type RemoveIntent struct {
	StartedAt time.Time
	// Remaining lists the members not yet removed (sorted, unique).
	Remaining []string
}

type intentDoc struct {
	Generated     string                  `json:"_generated"`
	SchemaVersion int                     `json:"schema_version"`
	Bundles       map[string]intentRecord `json:"bundles"`
}

type intentRecord struct {
	StartedAt *string   `json:"started_at"`
	Remaining *[]string `json:"remaining"`
}

// IntentPath is the remove-intent file.
func (s *Store) IntentPath() string { return filepath.Join(s.Dir(), intentFileName) }

var (
	intentRootKeys   = keySet("_generated", "schema_version", "bundles")
	intentBundleKeys = keySet("started_at", "remaining")
)

func intentAllowedKeys(path []string) map[string]bool {
	switch {
	case len(path) == 0:
		return intentRootKeys
	case len(path) == 2 && path[0] == "bundles":
		return intentBundleKeys
	}
	return nil
}

// parseIntents reads and validates the intent file bytes.
func parseIntents(data []byte) (map[string]RemoveIntent, error) {
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("file is larger than %d bytes", MaxBytes)
	}
	if err := checkStructureWith(data, intentAllowedKeys); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var d intentDoc
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if dec.More() {
		return nil, errors.New("trailing data after the document")
	}
	if d.Generated != Generated {
		return nil, fmt.Errorf("_generated must be %q", Generated)
	}
	if d.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported schema_version %d (this nself reads %d)", d.SchemaVersion, SchemaVersion)
	}
	if d.Bundles == nil {
		return nil, errors.New("bundles is missing or null")
	}
	out := make(map[string]RemoveIntent, len(d.Bundles))
	for slug, rec := range d.Bundles {
		if !slugRE.MatchString(slug) {
			return nil, fmt.Errorf("bundle slug %q does not match %s", slug, SlugPattern)
		}
		if rec.StartedAt == nil || rec.Remaining == nil {
			return nil, fmt.Errorf("bundle %q needs started_at and remaining", slug)
		}
		at, err := time.Parse(time.RFC3339, *rec.StartedAt)
		if err != nil {
			return nil, fmt.Errorf("bundle %q started_at %q is not RFC 3339", slug, *rec.StartedAt)
		}
		seen := map[string]bool{}
		for _, m := range *rec.Remaining {
			if !slugRE.MatchString(m) {
				return nil, fmt.Errorf("bundle %q remaining %q does not match %s", slug, m, SlugPattern)
			}
			if seen[m] {
				return nil, fmt.Errorf("bundle %q lists remaining %q twice", slug, m)
			}
			seen[m] = true
		}
		out[slug] = RemoveIntent{StartedAt: at.UTC(), Remaining: sortedUnique(*rec.Remaining)}
	}
	return out, nil
}

func sortedUnique(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// readIntents loads the file; a missing file is no intents.
func (tx *Tx) readIntents() (map[string]RemoveIntent, error) {
	path := tx.s.IntentPath()
	data, err := readCapped(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]RemoveIntent{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ledger: read %s: %w", path, err)
	}
	m, err := parseIntents(data)
	if err != nil {
		return nil, fmt.Errorf("ledger: %w (file %s; remove it if no bundle remove is running)", err, path)
	}
	return m, nil
}

// writeIntents persists m, or deletes the file when m is empty.
func (tx *Tx) writeIntents(m map[string]RemoveIntent) error {
	path := tx.s.IntentPath()
	if len(m) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("ledger: delete %s: %w", path, err)
		}
		if err := syncDir(tx.s.Dir()); err != nil {
			return fmt.Errorf("ledger: flush %s: %w", tx.s.Dir(), err)
		}
		return nil
	}
	d := intentDoc{Generated: Generated, SchemaVersion: SchemaVersion, Bundles: map[string]intentRecord{}}
	for slug, in := range m {
		at := in.StartedAt.UTC().Format(time.RFC3339)
		rem := sortedUnique(in.Remaining)
		d.Bundles[slug] = intentRecord{StartedAt: &at, Remaining: &rem}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(d); err != nil {
		return fmt.Errorf("ledger: encode intent: %w", err)
	}
	if buf.Len() > MaxBytes {
		return fmt.Errorf("ledger: refusing to write %d bytes: over the %d-byte limit", buf.Len(), MaxBytes)
	}
	return tx.s.writeAtomic(path, intentTempPattern, buf.Bytes())
}

// RemoveIntent returns the recorded intent for bundle slug, if any.
func (tx *Tx) RemoveIntent(slug string) (RemoveIntent, bool, error) {
	m, err := tx.readIntents()
	if err != nil {
		return RemoveIntent{}, false, err
	}
	in, ok := m[strings.ToLower(strings.TrimSpace(slug))]
	return in, ok, nil
}

// SetRemoveIntent records (or replaces) the intent for bundle slug. Remaining
// is stored sorted and de-duplicated. Other bundles' intents are kept.
func (tx *Tx) SetRemoveIntent(slug string, in RemoveIntent) error {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !slugRE.MatchString(slug) {
		return fmt.Errorf("ledger: intent bundle slug %q does not match %s", slug, SlugPattern)
	}
	for _, m := range in.Remaining {
		if !slugRE.MatchString(m) {
			return fmt.Errorf("ledger: intent member %q does not match %s", m, SlugPattern)
		}
	}
	m, err := tx.readIntents()
	if err != nil {
		return err
	}
	m[slug] = RemoveIntent{StartedAt: in.StartedAt.UTC(), Remaining: sortedUnique(in.Remaining)}
	return tx.writeIntents(m)
}

// ClearRemoveIntent forgets the intent for bundle slug and deletes the file
// when no intent is left. A missing file or intent is not an error.
func (tx *Tx) ClearRemoveIntent(slug string) error {
	m, err := tx.readIntents()
	if err != nil {
		return err
	}
	slug = strings.ToLower(strings.TrimSpace(slug))
	if _, ok := m[slug]; !ok && len(m) > 0 {
		return nil
	}
	delete(m, slug)
	return tx.writeIntents(m)
}
