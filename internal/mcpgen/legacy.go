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
	"math"
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

// maxSafeInt is the largest integer a JSON number carries exactly.
const maxSafeInt = 1 << 53

// value decodes one raw member and checks it against the declared type.
// A JSON null counts as absent, and so does an empty string unless the param
// allows it.
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
			return x, x != "" || p.AllowEmpty, nil
		}
	case bool:
		if p.Type == "boolean" {
			return x, true, nil
		}
	case json.Number:
		switch p.Type {
		case "number":
			if f, err := strconv.ParseFloat(x.String(), 64); err == nil && !math.IsInf(f, 0) && !math.IsNaN(f) {
				return x, true, nil
			}
		case "integer":
			if n, err := strconv.ParseInt(x.String(), 10, 64); err == nil {
				return json.Number(strconv.FormatInt(n, 10)), true, nil
			}
			if f, err := strconv.ParseFloat(x.String(), 64); err == nil && f == math.Trunc(f) && math.Abs(f) <= maxSafeInt {
				return json.Number(strconv.FormatInt(int64(f), 10)), true, nil
			}
		}
	}
	return nil, false, badParams("%s: %s must be a %s", r.Name, p.Name, p.Type)
}

// positive reports whether a decoded number is greater than zero.
func positive(v any) bool {
	n, ok := v.(json.Number)
	if !ok {
		return true
	}
	f, err := strconv.ParseFloat(n.String(), 64)
	return err == nil && f > 0
}

// isNull reports whether raw is the JSON null.
func isNull(raw json.RawMessage) bool { return string(bytes.TrimSpace(raw)) == "null" }

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
		if given && isNull(raw) {
			given = false // null is "not given": the legacy default still applies
		}
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
		if present && p.Positive && !positive(v) && p.Default != nil {
			b, _ := json.Marshal(p.Default) // zero or less meant the default
			if v, present, err = r.value(p, b); err != nil {
				return zero, err
			}
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
		last := -1
		for i, name := range r.Args {
			v, ok := vals[name]
			s, _ := v.(string)
			args = append(args, s)
			if ok {
				last = i // only trailing values that were never given are dropped
			}
		}
		if args = args[:last+1]; len(args) > 0 {
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
			secs, err := strconv.ParseFloat(n.String(), 64)
			if err != nil || secs <= 0 {
				return zero, badParams("%s: %s must be greater than zero", r.Name, f.Param)
			}
			v = strconv.FormatFloat(secs, 'f', -1, 64) + "s" // plain decimal, never 1e3s
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
