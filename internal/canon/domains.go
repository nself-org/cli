package canon

// Fragment loading and validation (contract:cli.canon-fragments v1).
//
// Purpose:     read canon.yaml (schema_version, verbs) and every
//              domains/<name>.yaml, merge them into one raw File and check every
//              rule of the contract, reporting each problem with the fragment
//              file and the offending path.
// Inputs:      an fs.FS holding canon.yaml and domains/*.yaml.
// Outputs:     the merged, validated *File, or a *ValidationError.
// Constraints: strict decoding; deterministic messages; no I/O beyond the FS.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ModeV14 is the only value of an entry's `mode`: the entry exists only in v1.4 mode.
const ModeV14 = "v1.4"

// RequiredFragments are the fragments the embedded canon must contain (EPIC
// P7-CANON contract table). Later Tickets add fragments; none may remove these.
var RequiredFragments = []string{
	"lifecycle", "build", "io", "observe", "config", "account", "data",
	"deploy", "services", "extend", "mcp", "breakouts", "admin",
}

var (
	fragmentName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	versionRe    = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)
	pluginRe     = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// Fragment is the decoded form of one domains/<name>.yaml.
type Fragment struct {
	SchemaVersion int              `yaml:"schema_version"`
	Commands      map[string]Entry `yaml:"commands"`
	Rows          `yaml:",inline"`
}

// rootDoc is the decoded canon.yaml: only the schema version and the verbs.
type rootDoc struct {
	SchemaVersion int      `yaml:"schema_version"`
	Verbs         []string `yaml:"verbs"`
}

func decodeStrict(data []byte, v any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("empty document")
		}
		return err
	}
	return nil
}

// LoadFS loads canon.yaml and every domains/*.yaml of fsys (lexical order),
// merges them and validates the result. It is the loader behind LoadRaw and the
// entry point for fixtures. A failure returns one aggregated *ValidationError
// (or a decode error for canon.yaml itself) naming file and path.
func LoadFS(fsys fs.FS) (*File, error) {
	data, err := fs.ReadFile(fsys, "canon.yaml")
	if err != nil {
		return nil, fmt.Errorf("canon: read canon.yaml: %w", err)
	}
	var r rootDoc
	if err := decodeStrict(data, &r); err != nil {
		return nil, fmt.Errorf("canon: canon.yaml: decode: %w", err)
	}
	f := &File{SchemaVersion: r.SchemaVersion, Verbs: r.Verbs, Commands: map[string]Entry{}, origin: map[string]string{}}
	p := f.validateHeader()
	names, err := fs.Glob(fsys, "domains/*.yaml")
	if err != nil {
		return nil, fmt.Errorf("canon: list fragments: %w", err)
	}
	sort.Strings(names)
	verbs := map[string]bool{}
	for _, v := range f.Verbs {
		verbs[v] = true
	}
	for _, name := range names {
		p = append(p, f.mergeFragment(fsys, name, verbs)...)
	}
	if len(p) == 0 {
		for _, v15 := range []bool{false, true} {
			if _, err := f.View(v15); err != nil {
				p = append(p, err.(*ValidationError).Problems...)
			}
		}
	}
	if err := NewValidationError(p); err != nil {
		return nil, err
	}
	return f, nil
}

// mergeFragment decodes one fragment, validates it and merges it into f.
func (f *File) mergeFragment(fsys fs.FS, name string, verbs map[string]bool) []string {
	base := strings.TrimSuffix(path.Base(name), ".yaml")
	var p []string
	if !fragmentName.MatchString(base) {
		p = append(p, fmt.Sprintf("%s: fragment name %q must match ^[a-z][a-z0-9_]*$", name, base))
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return append(p, fmt.Sprintf("%s: read: %v", name, err))
	}
	var fr Fragment
	if err := decodeStrict(data, &fr); err != nil {
		return append(p, fmt.Sprintf("%s: decode: %v", name, err))
	}
	if fr.SchemaVersion != SchemaVersion {
		p = append(p, fmt.Sprintf("%s: schema_version: %d is not supported (want %d)", name, fr.SchemaVersion, SchemaVersion))
	}
	keys := make([]string, 0, len(fr.Commands))
	for k := range fr.Commands {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		e := fr.Commands[k]
		for _, m := range validateEntry(k, e, verbs) {
			p = append(p, name+": "+m)
		}
		if e.Mode != "" && e.Mode != ModeV14 {
			p = append(p, fmt.Sprintf("%s: commands[%q]: mode %q is not v1.4 (omit it for entries present in both modes)", name, k, e.Mode))
		}
		if prev, dup := f.origin["commands:"+k]; dup {
			p = append(p, fmt.Sprintf("commands[%q] is defined in both %s and %s", k, prev, name))
			continue
		}
		f.origin["commands:"+k] = name
		f.Commands[k] = e
	}
	p = append(p, fr.validate(name+": ")...)
	p = append(p, f.mergeRows(name, fr.Rows)...)
	return p
}

// mergeRows appends the rows of one fragment to f, reporting a `from` (or hub
// path or builtin) already claimed by another fragment.
func (f *File) mergeRows(name string, r Rows) []string {
	var p []string
	claim := func(kind, key, list string) bool {
		id := kind + ":" + key
		if prev, ok := f.origin[id]; ok {
			p = append(p, fmt.Sprintf("%s %q is declared in both %s and %s (%s)", kind, key, prev, name, list))
			return false
		}
		f.origin[id] = name
		return true
	}
	for _, h := range r.Hubs {
		if claim("hub", h.Path, "hubs") {
			f.Hubs = append(f.Hubs, h)
		}
	}
	for _, b := range r.Builtins {
		if claim("builtin", b, "builtins") {
			f.Builtins = append(f.Builtins, b)
		}
	}
	for _, x := range r.Moves {
		if claim("from", x.From, "moves") {
			f.Moves = append(f.Moves, x)
		}
	}
	for _, x := range r.Shims {
		if claim("from", x.From, "shims") {
			f.Shims = append(f.Shims, x)
		}
	}
	for _, x := range r.RetiredHubs {
		if claim("from", x.From, "retired_hubs") {
			f.RetiredHubs = append(f.RetiredHubs, x)
		}
	}
	for _, x := range r.Breakouts {
		if claim("from", x.From, "breakouts") {
			f.Breakouts = append(f.Breakouts, x)
		}
	}
	for _, x := range r.Removed {
		if claim("from", x.From, "removed") {
			f.Removed = append(f.Removed, x)
		}
	}
	return p
}

// where names the file that declared the given key ("" when unknown, as for a
// single document parsed with Parse).
func (f *File) where(kind, key string) string {
	if n := f.origin[kind+":"+key]; n != "" {
		return n + ": "
	}
	return ""
}

// checkRequiredFragments reports every contract fragment missing from fsys.
func checkRequiredFragments(fsys fs.FS) []string {
	var p []string
	for _, n := range RequiredFragments {
		if _, err := fs.Stat(fsys, "domains/"+n+".yaml"); err != nil {
			p = append(p, fmt.Sprintf("domains/%s.yaml: missing (contract:cli.canon-fragments v1 requires every owner fragment)", n))
		}
	}
	return p
}
