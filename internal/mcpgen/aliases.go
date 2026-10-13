package mcpgen

// Purpose: the legacy-name table (legacy_aliases.yaml) and its loader.
// Inputs: the embedded YAML. Outputs: validated Rows, one per legacy MCP tool name.
// Constraints: strict decoding; the table is the only place a legacy name is spelled.

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed legacy_aliases.yaml
var aliasYAML []byte

// Row kinds.
const (
	KindMerged  = "merged"
	KindAlias   = "alias"
	KindRemoved = "removed"
)

// DeprecatedPrefix starts every alias description and every legacy property.
const DeprecatedPrefix = "Deprecated: "

// Param is one legacy property of a tool.
type Param struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"` // string | integer | number | boolean
	Description string `yaml:"description"`
	Default     any    `yaml:"default"`
}

// FlagMap maps a legacy param onto a flag of the target command.
type FlagMap struct {
	Flag  string `yaml:"flag"`
	Param string `yaml:"param"`
	As    string `yaml:"as"` // "" | seconds
}

// Removed names the refusal a removed tool returns.
type Removed struct {
	Code    string   `yaml:"code"`
	Message string   `yaml:"message"`
	Use     []string `yaml:"use"`
}

// Row is one legacy tool name.
type Row struct {
	Name        string    `yaml:"name"`
	Kind        string    `yaml:"kind"`
	Target      string    `yaml:"target"`
	Plugin      string    `yaml:"plugin"`
	SideEffect  string    `yaml:"side_effect"`
	Summary     string    `yaml:"summary"`
	Params      []Param   `yaml:"params"`
	Required    []string  `yaml:"required"`
	Args        []string  `yaml:"args"`
	Flags       []FlagMap `yaml:"flags"`
	Unsupported []string  `yaml:"unsupported"`
	Removed     *Removed  `yaml:"removed"`
}

type aliasFile struct {
	RemovalAt string `yaml:"removal_at"`
	Rows      []Row  `yaml:"rows"`
}

// Table is the loaded legacy-name table.
type Table struct {
	RemovalAt string
	Rows      []Row
	byName    map[string]*Row
}

// Lookup returns the row for a legacy tool name.
func (t *Table) Lookup(name string) (*Row, bool) {
	r, ok := t.byName[name]
	return r, ok
}

// LoadTable decodes and validates the embedded table.
func LoadTable() (*Table, error) { return parseTable(aliasYAML) }

func parseTable(data []byte) (*Table, error) {
	var f aliasFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("legacy_aliases.yaml: %w", err)
	}
	t := &Table{RemovalAt: f.RemovalAt, Rows: f.Rows, byName: map[string]*Row{}}
	var problems []string
	for i := range t.Rows {
		r := &t.Rows[i]
		if _, dup := t.byName[r.Name]; dup {
			problems = append(problems, r.Name+": duplicate row")
		}
		t.byName[r.Name] = r
		problems = append(problems, r.validate()...)
	}
	if t.RemovalAt == "" {
		problems = append(problems, "removal_at is empty")
	}
	if len(problems) != 0 {
		return nil, fmt.Errorf("legacy_aliases.yaml: %s", strings.Join(problems, "; "))
	}
	return t, nil
}

var machineMembers = map[string]bool{"args": true, "flags": true, "argv": true, "confirm": true}

func (r *Row) validate() []string {
	var p []string
	bad := func(format string, a ...any) { p = append(p, r.Name+": "+fmt.Sprintf(format, a...)) }
	switch r.Kind {
	case KindMerged, KindAlias:
		if r.Target == "" {
			bad("target is empty")
		}
	case KindRemoved:
		if r.Removed == nil || r.Removed.Code == "" || r.Removed.Message == "" {
			bad("a removed row needs removed.code and removed.message")
		}
	default:
		bad("kind %q is not merged, alias or removed", r.Kind)
	}
	names := map[string]bool{}
	for _, par := range r.Params {
		switch {
		case par.Name == "" || names[par.Name]:
			bad("param %q is empty or repeated", par.Name)
		case machineMembers[par.Name] && r.Kind == KindMerged:
			bad("param %q collides with a machine request member", par.Name)
		case par.Type != "string" && par.Type != "integer" && par.Type != "number" && par.Type != "boolean":
			bad("param %q has type %q", par.Name, par.Type)
		case par.Description == "":
			bad("param %q has no description", par.Name)
		}
		names[par.Name] = true
	}
	refs := append(append(append([]string{}, r.Required...), r.Args...), r.Unsupported...)
	for _, f := range r.Flags {
		refs = append(refs, f.Param)
		if f.Flag == "" || (f.As != "" && f.As != "seconds") {
			bad("flag mapping %+v is invalid", f)
		}
	}
	for _, name := range refs {
		if !names[name] {
			bad("%q is not a param of this row", name)
		}
	}
	if r.Kind != KindMerged && r.Plugin == "" && r.SideEffect != "" && r.Kind != KindRemoved {
		bad("side_effect is only for plugin and removed rows")
	}
	return p
}

// ParamNames lists the legacy property names of the row.
func (r *Row) ParamNames() []string {
	out := make([]string, 0, len(r.Params))
	for _, p := range r.Params {
		out = append(out, p.Name)
	}
	return out
}

func (r *Row) param(name string) (Param, bool) {
	for _, p := range r.Params {
		if p.Name == name {
			return p, true
		}
	}
	return Param{}, false
}
