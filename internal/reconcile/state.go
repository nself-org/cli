package reconcile

// state.go — the generated-state record: what nself last wrote
// (EPIC P7-LIVE D5, Ticket P7-LIVE-04).
//
// Purpose: one file under the project, .nself/state/generated.json, holds the
// sha256 of the exact bytes nself last wrote for every file it wrote and the
// host-level entries it created, so a later build can tell a generated change
// from a human one. Plan marks a file hand_edited when it is recorded and its
// current bytes hash differently; Apply then shows the diff and refuses to
// overwrite or remove it without --force (reconcile.Confirm).
// Inputs: the project directory, and the planned build of a successful apply.
// Outputs: LoadGeneratedState and GeneratedState.Save; RecordPlanned folds a
// successful apply's render into a record; PlannedRecord is that fold, pure,
// so the plan can report the record itself as an artifact (kind state) with
// the exact bytes the apply writes.
// Constraints: the record is written only after the whole apply succeeded, and
// atomically (temp file + rename), so an interrupted apply leaves the previous
// record intact. Any byte change counts — the hash is of the exact bytes the
// write was held to; line-ending churn is a change (documented in
// Safe-On-Live.md, not normalised here). Recording is bookkeeping both compat
// modes do; only the refusal that reads the record is v1.5 behaviour (ADR 0021,
// gated in confirm.go). A first run on a project without a record records it
// and warns once (apply.go); it never blocks.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	nbuild "github.com/nself-org/cli/internal/build"
)

// generatedStateSchema is the record's schema_version.
const generatedStateSchema = "1"

// GeneratedStateDisplayPath is the record's location as a plan display path
// (project-relative, "/" separators): the artifact path the plan reports for
// the record itself.
const GeneratedStateDisplayPath = ".nself/state/generated.json"

// GeneratedState is what nself last wrote in a project: the sha256 of every
// file it wrote, keyed by the plan's display path (project-relative for
// project files, "@fronting/<rel>", "@plugins/<rel>" or "@abs/<path>" for
// files outside the project), and the host-level entries it created by effect
// kind and target.
type GeneratedState struct {
	SchemaVersion string               `json:"schema_version"`
	Files         map[string]string    `json:"files"`
	Host          []GeneratedHostEntry `json:"host"`
}

// GeneratedHostEntry is one host-level entry nself created (an /etc/hosts
// block, a trust-store entry).
type GeneratedHostEntry struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

// GeneratedStatePath is the record's location in a project.
func GeneratedStatePath(projectDir string) string {
	return filepath.Join(projectDir, filepath.FromSlash(GeneratedStateDisplayPath))
}

// LoadGeneratedState reads the record. A missing record is not an error: it
// returns (nil, nil), the no-state-yet case of D5. A record that exists but
// cannot be parsed is an error: detection would silently answer "never
// hand-edited" over an unknown file, so the operator is told to remove it (the
// next successful apply re-records).
func LoadGeneratedState(projectDir string) (*GeneratedState, error) {
	s, _, err := loadGeneratedStateRaw(projectDir)
	return s, err
}

// loadGeneratedStateRaw is LoadGeneratedState that also returns the record's
// bytes as read, so the planner shows exactly those bytes as the record's
// "before" side without a second read that could race the first.
func loadGeneratedStateRaw(projectDir string) (*GeneratedState, []byte, error) {
	raw, err := os.ReadFile(GeneratedStatePath(projectDir))
	if err != nil {
		if os.IsNotExist(err) {
			// A dangling symlink in the record's place is not "no record":
			// treating it as a first run would drop every hand-edit guard.
			if _, lerr := os.Lstat(GeneratedStatePath(projectDir)); lerr == nil {
				return nil, nil, fmt.Errorf("the generated-state record %s is a dangling link"+
					"; delete it and run nself build, which re-records the files it writes",
					GeneratedStatePath(projectDir))
			}
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("reading the generated-state record: %w", err)
	}
	var s GeneratedState
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, nil, fmt.Errorf("the generated-state record %s cannot be parsed: %v"+
			"; delete the file and run nself build, which re-records the files it writes",
			GeneratedStatePath(projectDir), err)
	}
	if s.SchemaVersion != generatedStateSchema { // a missing schema is a record nself did not write: refuse
		return nil, nil, fmt.Errorf("the generated-state record %s has schema %q, this CLI understands %q"+
			"; delete the file and run nself build, which re-records the files it writes",
			GeneratedStatePath(projectDir), s.SchemaVersion, generatedStateSchema)
	}
	return &s, raw, nil
}

// Save writes the record deterministically and atomically: file keys are
// sorted by encoding/json, host entries are sorted and deduped here, the body
// goes to a temp file in the state directory and one rename puts it in place,
// so a reader (or a crash) never sees a partial record.
func (s *GeneratedState) Save(projectDir string) error {
	body, err := s.body()
	if err != nil {
		return err
	}
	return writeRecord(projectDir, body)
}

// PlannedRecord returns what a successful apply of pb leaves in the record,
// and the exact bytes of it: a copy of the current record with pb folded in
// (RecordPlanned), serialised the way Save writes it. Pure, so the plan can
// report the record as an artifact (kind state) whose bytes are exactly what
// the apply then writes — plan == apply stays exact. The record never holds an
// entry for itself: the folded set is the build's render, not the record, so
// there is no self-reference to resolve.
func PlannedRecord(state *GeneratedState, pb *nbuild.PlannedBuild) (*GeneratedState, []byte, error) {
	next := &GeneratedState{}
	if state != nil {
		next.Files = make(map[string]string, len(state.Files))
		for k, v := range state.Files {
			next.Files[k] = v
		}
		next.Host = append([]GeneratedHostEntry(nil), state.Host...)
	}
	next.RecordPlanned(pb)
	body, err := next.body()
	if err != nil {
		return nil, nil, err
	}
	return next, body, nil
}

// body is the record's canonical bytes: schema pinned, files map allocated,
// host entries sorted and deduped, MarshalIndent with a trailing newline.
func (s *GeneratedState) body() ([]byte, error) {
	s.SchemaVersion = generatedStateSchema
	if s.Files == nil {
		s.Files = map[string]string{}
	}
	s.Host = sortedHostEntries(s.Host)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding the generated-state record: %w", err)
	}
	return append(b, '\n'), nil
}

// writeRecord puts body in place atomically: a temp file in the state
// directory, 0644, then one rename, so a reader (or a crash) never sees a
// partial record. Callers hold the bytes they were shown in the plan.
func writeRecord(projectDir string, body []byte) error {
	dir := filepath.Dir(GeneratedStatePath(projectDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "generated.*.tmp")
	if err != nil {
		return fmt.Errorf("creating a temp record in %s: %w", dir, err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }() // a no-op once the rename below succeeded
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", name, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", name, err)
	}
	if err := os.Rename(name, GeneratedStatePath(projectDir)); err != nil {
		return fmt.Errorf("putting the generated-state record in place: %w", err)
	}
	return nil
}

// RecordPlanned folds one successful apply into the record: every file the
// write was held to is (re)recorded with the sha256 of those exact bytes,
// every removal drops its entry, and the host effects nself creates (hosts
// blocks, trust-store entries) are recorded by kind and target. Other effect
// kinds act outside the project tree or are bookkeeping and stay unrecorded.
func (s *GeneratedState) RecordPlanned(pb *nbuild.PlannedBuild) {
	if s.Files == nil {
		s.Files = map[string]string{}
	}
	for key, f := range pb.Files {
		sum := sha256.Sum256(f.Data)
		s.Files[displayPath(key)] = hex.EncodeToString(sum[:])
	}
	for _, key := range pb.Removed {
		delete(s.Files, displayPath(key))
	}
	for _, e := range pb.Effects {
		if e.Kind == string(EffectHosts) || e.Kind == string(EffectTrustStore) {
			s.Host = append(s.Host, GeneratedHostEntry{Kind: e.Kind, Target: e.Target})
		}
	}
	s.Host = sortedHostEntries(s.Host)
}

// sortedHostEntries returns the entries sorted by kind then target, without
// duplicates, so two applies of the same plan write the same record.
func sortedHostEntries(in []GeneratedHostEntry) []GeneratedHostEntry {
	sort.Slice(in, func(i, j int) bool {
		if in[i].Kind != in[j].Kind {
			return in[i].Kind < in[j].Kind
		}
		return in[i].Target < in[j].Target
	})
	out := in[:0]
	for i, e := range in {
		if i > 0 && e == in[i-1] {
			continue
		}
		out = append(out, e)
	}
	return out
}
