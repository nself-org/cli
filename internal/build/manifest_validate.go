package build

// Purpose: validate nself.yaml against ProjectManifest and report every
//          deviation with its line (E436 unknown key, E435 syntax, type or
//          duplicate-key error). Unreadable or unparsable files are findings,
//          never "valid". Findings warn in v1.4 and fail in v1.5.
// Constraints: checks are derived by reflection from the yaml tags, follow
//          what LoadProjectManifest accepts (merge keys resolved, first YAML
//          document only), and skip x- extension keys at every depth. The JSON
//          Schema (tools/schemagen) cannot see merge keys: for such a file this
//          validator is authoritative. SPORT: contract:config.nself-yaml.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/compat"
)

// Finding codes (internal/errs/codes_nself_yaml.go), the extension-key prefix
// nself ignores, and the severities (only SeverityError fails validation).
const (
	CodeManifestType    = "E435"
	CodeManifestUnknown = "E436"
	ExtensionPrefix     = "x-"
	SeverityWarning     = "warning"
	SeverityError       = "error"
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
	return fmt.Sprintf("%s:%d:%d: %s: %s: [%s] %s (%s)",
		f.File, f.Line, f.Column, f.Severity, f.Path, f.Code, f.Message, f.Fix)
}

// ManifestPath returns the manifest LoadProjectManifest reads in workdir, or "".
func ManifestPath(workdir string) string {
	for _, name := range manifestFilenames {
		p := filepath.Join(workdir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ValidateManifestFile validates the file at path; unreadable is a finding.
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
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil // empty file: a zero manifest, as in build
		}
		c.add(1, 1, "(file)", CodeManifestType, "invalid YAML: "+err.Error(), "fix the YAML syntax; nself build cannot read this file")
		return c.out
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) { // always a warning: build ignores them
		line := extra.Line
		if len(extra.Content) > 0 {
			line = extra.Content[0].Line
		}
		c.out = append(c.out, Finding{File: name, Line: line, Column: 1, Path: "(document 2)", Code: CodeManifestType, Severity: SeverityWarning,
			Message: "later YAML documents are ignored: nself reads only the first", Fix: "move its keys into the first document or remove it"})
	}
	if len(doc.Content) > 0 {
		c.walk(doc.Content[0], reflect.TypeOf(ProjectManifest{}), "")
	}
	sort.SliceStable(c.out, func(i, j int) bool { return c.out[i].Line < c.out[j].Line })
	return c.out
}

type collector struct {
	file string
	out  []Finding
}

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
	c.add(n.Line, n.Column, path, CodeManifestType, "expected "+want+", found "+describe(n), "change the value to "+want)
}

// describe names the kind of a YAML node for messages.
func describe(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		return "a map"
	case yaml.SequenceNode:
		return "a list"
	}
	return "a " + strings.TrimPrefix(n.ShortTag(), "!!") + " (" + n.Value + ")"
}

var pluginsType = reflect.TypeOf(ManifestPlugins{})

// fields maps YAML key to Go type; ManifestPlugins.Flat is the list form, not a key.
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

// walk checks n against Go type t; null is the zero value except as a list item.
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
		if n.Kind == yaml.ScalarNode && n.ShortTag() != "!!str" {
			// build coerces these to a string (no -> "false"), which is rarely meant.
			c.add(n.Line, n.Column, path, CodeManifestType,
				fmt.Sprintf("expected a string, found %s: YAML reads it as a %s, not text", describe(n), strings.TrimPrefix(n.ShortTag(), "!!")),
				fmt.Sprintf("quote it: %q", n.Value))
		} else if n.Kind != yaml.ScalarNode {
			c.typeErr(n, path, "a string")
		}
	}
}

// pairs returns mapping n's key/value pairs as build sees them: explicit keys,
// then keys merged through "<<" (alias or list of aliases) that no explicit key
// overrides. "<<" is never a key. Duplicate explicit keys are reported.
func (c *collector) pairs(n *yaml.Node, path string) [][2]*yaml.Node {
	var out [][2]*yaml.Node
	var merges []*yaml.Node
	seen := map[string]*yaml.Node{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.ShortTag() == "!!merge" {
			merges = append(merges, v)
			continue
		}
		if first, dup := seen[k.Value]; dup {
			c.add(k.Line, k.Column, joinPath(path, k.Value), CodeManifestType,
				fmt.Sprintf("duplicate key %q (first at line %d)", k.Value, first.Line), "remove one of the two; nself build rejects duplicate keys")
			continue
		}
		seen[k.Value] = k
		out = append(out, [2]*yaml.Node{k, v})
	}
	for _, m := range merges {
		srcs := []*yaml.Node{m}
		if m.Kind == yaml.SequenceNode {
			srcs = m.Content
		}
		for _, src := range srcs {
			for src.Kind == yaml.AliasNode && src.Alias != nil {
				src = src.Alias
			}
			if src.Kind != yaml.MappingNode {
				c.typeErr(src, path, "a map (or a list of maps) after <<")
				continue
			}
			for _, kv := range c.pairs(src, path) {
				if _, over := seen[kv[0].Value]; !over {
					seen[kv[0].Value] = kv[0]
					out = append(out, kv)
				}
			}
		}
	}
	return out
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// object checks the keys of mapping n against the struct type t.
func (c *collector) object(n *yaml.Node, t reflect.Type, path string) {
	known := fields(t)
	for _, kv := range c.pairs(n, path) {
		k, v := kv[0], kv[1]
		if strings.HasPrefix(k.Value, ExtensionPrefix) {
			continue
		}
		kp := joinPath(path, k.Value)
		ft, ok := known[k.Value]
		if !ok {
			c.add(k.Line, k.Column, kp, CodeManifestUnknown, fmt.Sprintf("unknown key %q: nself does not read it", k.Value), unknownFix(k.Value, known))
			continue
		}
		c.walk(v, ft, kp)
	}
}

// unknownFix suggests a known key (plural slip, or project for app) and the x- rename.
func unknownFix(key string, known map[string]reflect.Type) string {
	base := "rename to " + ExtensionPrefix + key + " if it is app metadata, or remove it; nself does not read it"
	lk := strings.ToLower(key)
	names := make([]string, 0, len(known))
	for k := range known {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if lk == k+"s" || lk+"s" == k || (key == "project" && k == "app") {
			return fmt.Sprintf("did you mean %q? Otherwise %s", k, base)
		}
	}
	return base
}

// ManifestFields returns the YAML keys read for t (ProjectManifest or ManifestPlugins).
func ManifestFields(t reflect.Type) map[string]reflect.Type { return fields(t) }
