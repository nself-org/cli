package build

// Purpose: validate a project's nself.yaml against the shape of
//          ProjectManifest and report every deviation with its line, so a
//          silently ignored key or a wrong-typed value becomes a visible finding.
// Inputs:  a manifest path (ValidateManifestFile) or bytes (ValidateManifestBytes).
// Outputs: []Finding, ordered by line. Code E436 = unknown key, E435 = syntax
//          or type error. A clean file returns nil. A file that cannot be read
//          or parsed is itself a finding, never "valid".
// Constraints: the checks are derived by reflection from the yaml tags of
//          ProjectManifest and ManifestPlugins, so they follow the type.
//          Keys starting with "x-" are extension keys and are skipped at every
//          depth. Parsing in LoadProjectManifest is not changed by this file.
//          tools/schemagen builds schemas/nself-yaml.v1.schema.json from the
//          same type; TestManifestValidateAgreesWithSchema keeps both honest.
// SPORT: contract:config.nself-yaml.

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/compat"
)

// Finding codes (registered in internal/errs/codes_nself_yaml.go).
const (
	CodeManifestType    = "E435"
	CodeManifestUnknown = "E436"
)

// ExtensionPrefix marks a key nself ignores: app metadata that nself does not read.
const ExtensionPrefix = "x-"

// Finding severities. Both are reported; only SeverityError fails validation.
const (
	SeverityWarning = "warning"
	SeverityError   = "error"
)

// Finding is one deviation in nself.yaml.
type Finding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Path     string `json:"path"`
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Fix      string `json:"fix"`
}

// String renders "file:line:col: severity: path: [code] message (fix)".
func (f Finding) String() string {
	return fmt.Sprintf("%s:%d:%d: %s: %s: [%s] %s (%s)", f.File, f.Line, f.Column, f.Severity, f.Path, f.Code, f.Message, f.Fix)
}

// ManifestPath returns the manifest in workdir that LoadProjectManifest reads
// (the first of nself.yaml, nself.yml that exists), or "" when there is none.
func ManifestPath(workdir string) string {
	for _, name := range manifestFilenames {
		p := filepath.Join(workdir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ValidateManifestFile validates the manifest at path. An unreadable file is a
// finding, never a pass.
func ValidateManifestFile(path string) []Finding {
	data, err := os.ReadFile(path)
	if err != nil {
		c := &collector{file: filepath.Base(path)}
		c.add(1, 1, "(file)", CodeManifestType, "cannot read "+filepath.Base(path)+": "+err.Error(), "check the file permissions and that it is a regular file")
		return c.out
	}
	return ValidateManifestBytes(filepath.Base(path), data)
}

// ValidateManifestBytes validates manifest bytes; name is used in findings.
func ValidateManifestBytes(name string, data []byte) []Finding {
	c := &collector{file: name}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		c.add(1, 1, "(file)", CodeManifestType, "invalid YAML: "+err.Error(), "fix the YAML syntax; nself build cannot read this file")
		return c.out
	}
	if doc.Kind == 0 || len(doc.Content) == 0 { // empty file: a zero manifest
		return nil
	}
	c.walk(doc.Content[0], reflect.TypeOf(ProjectManifest{}), "")
	sort.SliceStable(c.out, func(i, j int) bool { return c.out[i].Line < c.out[j].Line })
	return c.out
}

type collector struct {
	file string
	out  []Finding
}

// severity is the mode switch for every finding.
func severity() string {
	// compat.V15(P7-SURF-07): nself.yaml type errors and unknown keys warn -> fail (E435/E436)
	if compat.V15() {
		return SeverityError
	}
	return SeverityWarning
}

func (c *collector) add(line, col int, path, code, msg, fix string) {
	c.out = append(c.out, Finding{File: c.file, Line: line, Column: col, Path: path, Code: code, Severity: severity(), Message: msg, Fix: fix})
}

func (c *collector) typeErr(n *yaml.Node, path, want string) {
	c.add(n.Line, n.Column, path, CodeManifestType, fmt.Sprintf("expected %s, found %s", want, describe(n)), "change the value to "+want)
}

// describe names the kind of a YAML node for messages.
func describe(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		return "a map"
	case yaml.SequenceNode:
		return "a list"
	case yaml.ScalarNode:
		return "a " + strings.TrimPrefix(n.ShortTag(), "!!") + " (" + n.Value + ")"
	}
	return "a value of unknown kind"
}

var pluginsType = reflect.TypeOf(ManifestPlugins{})

// fields maps YAML key name to Go type for a struct. ManifestPlugins is the
// map form of plugins: its Flat field is the list form, so it is not a key.
func fields(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() || (t == pluginsType && f.Name == "Flat") {
			continue
		}
		name := strings.Split(f.Tag.Get("yaml"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		out[name] = f.Type
	}
	return out
}

func isNull(n *yaml.Node) bool { return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null" }

// walk checks node n against Go type t at path. A null value is the zero
// value, except as a list item, where it is a type error.
func (c *collector) walk(n *yaml.Node, t reflect.Type, path string) {
	for n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	if isNull(n) {
		return
	}
	switch {
	case t == pluginsType:
		switch n.Kind {
		case yaml.SequenceNode:
			c.walk(n, reflect.TypeOf([]string(nil)), path)
		case yaml.MappingNode:
			c.object(n, t, path)
		default:
			c.typeErr(n, path, "a list of plugin names or a map with free and pro lists")
		}
	case t.Kind() == reflect.Struct:
		if n.Kind != yaml.MappingNode {
			c.typeErr(n, path, "a map")
			return
		}
		c.object(n, t, path)
	case t.Kind() == reflect.Slice:
		if n.Kind != yaml.SequenceNode {
			c.typeErr(n, path, "a list")
			return
		}
		for i, item := range n.Content {
			ip := fmt.Sprintf("%s[%d]", path, i)
			if isNull(item) {
				c.typeErr(item, ip, "a string")
				continue
			}
			c.walk(item, t.Elem(), ip)
		}
	case t.Kind() == reflect.String:
		if n.Kind != yaml.ScalarNode || n.ShortTag() != "!!str" {
			c.typeErr(n, path, "a string")
		}
	}
}

// object checks the keys of mapping n against the struct type t.
func (c *collector) object(n *yaml.Node, t reflect.Type, path string) {
	known := fields(t)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if strings.HasPrefix(k.Value, ExtensionPrefix) {
			continue
		}
		kp := k.Value
		if path != "" {
			kp = path + "." + k.Value
		}
		ft, ok := known[k.Value]
		if !ok {
			c.add(k.Line, k.Column, kp, CodeManifestUnknown, fmt.Sprintf("unknown key %q: nself does not read it", k.Value), unknownFix(k.Value, known))
			continue
		}
		c.walk(v, ft, kp)
	}
}

// unknownFix is the fix text: a near-miss suggestion when one exists.
func unknownFix(key string, known map[string]reflect.Type) string {
	base := "rename to " + ExtensionPrefix + key + " if it is app metadata, or remove it; nself does not read it"
	best, bestD := "", 3
	names := make([]string, 0, len(known))
	for k := range known {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		d := editDistance(strings.ToLower(key), k)
		if key == "project" && k == "app" {
			d = 0 // the reference apps write project: where nself reads app:
		}
		if d < bestD {
			best, bestD = k, d
		}
	}
	if best != "" {
		return fmt.Sprintf("did you mean %q? Otherwise %s", best, base)
	}
	return base
}

// editDistance is the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// ManifestFields returns the YAML keys the CLI reads for struct type t (use
// ProjectManifest or ManifestPlugins) with their Go types, for generators.
func ManifestFields(t reflect.Type) map[string]reflect.Type { return fields(t) }
