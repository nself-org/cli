package mcpgen

// Purpose: map a legacy tool call (the deprecated properties of a merged tool,
// or the parameters of an alias) onto a machine request.
// Inputs: a Row and the members of the call's arguments object.
// Outputs: an invoke.Request, or an E420 naming the offending property.
// Constraints: shape = legacy iff at least one legacy-only property is present;
// confirm is shape-neutral because the legacy form has no way to say it.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/invoke"
)

func badParams(format string, a ...any) error { return errs.Newf("E420", format, a...) }

// IsLegacyShape reports whether the call carries at least one legacy-only property.
func (r *Row) IsLegacyShape(members map[string]json.RawMessage) bool {
	for _, p := range r.Params {
		if _, ok := members[p.Name]; ok {
			return true
		}
	}
	return false
}

// CheckNoMix refuses a call that carries both machine members and legacy properties.
func (r *Row) CheckNoMix(members map[string]json.RawMessage) error {
	for _, m := range []string{"args", "flags", "argv"} {
		if _, ok := members[m]; ok {
			return badParams("%s mixes the machine member %q with legacy properties (%s); send one shape",
				r.Name, m, joinNames(r.legacyPresent(members)))
		}
	}
	return nil
}

func (r *Row) legacyPresent(members map[string]json.RawMessage) []string {
	var out []string
	for _, p := range r.Params {
		if _, ok := members[p.Name]; ok {
			out = append(out, p.Name)
		}
	}
	return out
}

func joinNames(names []string) string {
	var b bytes.Buffer
	for i, n := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(n)
	}
	return b.String()
}

// value decodes one raw member and checks it against the declared type.
// A JSON null counts as absent.
func (r *Row) value(p Param, raw json.RawMessage) (v any, present bool, err error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, false, badParams("%s: %s is not valid JSON", r.Name, p.Name)
	}
	if v == nil {
		return nil, false, nil
	}
	switch x := v.(type) {
	case string:
		if p.Type == "string" {
			return x, x != "", nil
		}
	case bool:
		if p.Type == "boolean" {
			return x, true, nil
		}
	case json.Number:
		switch p.Type {
		case "number":
			return x, true, nil
		case "integer":
			if _, err := x.Int64(); err == nil {
				return x, true, nil
			}
			if f, err := x.Float64(); err == nil && f == float64(int64(f)) {
				return json.Number(strconv.FormatInt(int64(f), 10)), true, nil
			}
		}
	}
	return nil, false, badParams("%s: %s must be a %s", r.Name, p.Name, p.Type)
}

// Request maps members onto a machine request. allowConfirm admits the
// shape-neutral confirm member (the target has a confirm block).
func (r *Row) Request(members map[string]json.RawMessage, allowConfirm bool) (invoke.Request, error) {
	var zero invoke.Request
	for name := range members {
		if _, ok := r.param(name); !ok && (name != "confirm" || !allowConfirm) {
			return zero, badParams("%s has no parameter %q", r.Name, name)
		}
	}
	vals := map[string]any{}
	for _, p := range r.Params {
		raw, given := members[p.Name]
		if !given && p.Default != nil {
			b, _ := json.Marshal(p.Default)
			raw, given = b, true
		}
		if !given {
			continue
		}
		v, present, err := r.value(p, raw)
		if err != nil {
			return zero, err
		}
		if present {
			vals[p.Name] = v
		}
	}
	for _, name := range r.Required {
		if _, ok := vals[name]; !ok {
			return zero, badParams("%s needs the parameter %q", r.Name, name)
		}
	}
	for _, name := range r.Unsupported {
		if v, ok := vals[name]; ok && v != false {
			return zero, badParams("%s: the parameter %q is not available on machine surfaces; %s", r.Name, name, unsupportedHint(r.Target))
		}
	}
	out := map[string]any{}
	if len(r.Args) > 0 {
		args := make([]string, 0, len(r.Args))
		for _, name := range r.Args {
			s, _ := vals[name].(string)
			args = append(args, s)
		}
		for len(args) > 0 && args[len(args)-1] == "" {
			args = args[:len(args)-1]
		}
		if len(args) > 0 {
			out["args"] = args
		}
	}
	flags := map[string]any{}
	for _, f := range r.Flags {
		v, ok := vals[f.Param]
		if !ok {
			continue
		}
		if f.As == "seconds" {
			n, _ := v.(json.Number)
			v = n.String() + "s"
		}
		flags[f.Flag] = v
	}
	if len(flags) > 0 {
		out["flags"] = flags
	}
	if raw, ok := members["confirm"]; ok {
		var c string
		if err := json.Unmarshal(raw, &c); err != nil {
			return zero, badParams("%s: confirm must be a string", r.Name)
		}
		out["confirm"] = c
	}
	b, err := json.Marshal(out)
	if err != nil {
		return zero, badParams("%s: %v", r.Name, err)
	}
	return invoke.DecodeRequest(b)
}

func unsupportedHint(target string) string {
	return fmt.Sprintf("run nself %s from a shell, or select the environment with 'nself config env use'", target)
}
