package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
)

var codeRe = regexp.MustCompile(`^E[0-9]{3}$`)

var classes = map[string]bool{
	ClassUser: true, ClassInfra: true, ClassAuth: true,
	ClassDestructiveBlocked: true, ClassOther: true,
}

// CheckEnvelope reports why b is not one valid v1 envelope document, or nil.
// It is the dependency-free equivalent of schemas/envelope.v1.schema.json:
// exactly one object; only the members schema_version, command, data, error
// and meta; schema_version the string "1"; command a string; exactly one of
// data and error present (data may be null); the error object with a code
// matching E and three digits, a message, an integer exit_code from 1 to 255,
// a class from the five exit classes and optional string cause, remediation
// and docs_url; meta with optional deprecations (objects with string old, new
// and removal_at) and warnings (strings).
//
// A stream line is one envelope, so plugin tests call it per line. The CLI's
// repoqa test asserts it agrees with the JSON Schema on every fixture.
func CheckEnvelope(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return fmt.Errorf("envelope: invalid JSON: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("envelope: more than one JSON value")
	}
	doc, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("envelope: not a JSON object")
	}
	if err := onlyKeys(doc, "envelope", "schema_version", "command", "data", "error", "meta"); err != nil {
		return err
	}
	if sv, ok := doc["schema_version"].(string); !ok || sv != SchemaVersion {
		return fmt.Errorf("envelope: schema_version must be the string %q", SchemaVersion)
	}
	if _, ok := doc["command"].(string); !ok {
		return fmt.Errorf("envelope: command must be a string")
	}
	_, hasData := doc["data"]
	e, hasErr := doc["error"]
	if hasData == hasErr {
		return fmt.Errorf("envelope: exactly one of data and error must be present")
	}
	if hasErr {
		if err := checkError(e); err != nil {
			return err
		}
	}
	if m, ok := doc["meta"]; ok {
		return checkMeta(m)
	}
	return nil
}

func onlyKeys(o map[string]any, what string, allowed ...string) error {
	for k := range o {
		found := false
		for _, a := range allowed {
			found = found || a == k
		}
		if !found {
			return fmt.Errorf("%s: unknown member %q", what, k)
		}
	}
	return nil
}

func checkError(v any) error {
	o, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("error: not an object")
	}
	if err := onlyKeys(o, "error", "code", "message", "cause", "remediation", "docs_url", "exit_code", "class"); err != nil {
		return err
	}
	for _, k := range []string{"code", "message", "exit_code", "class"} {
		if _, ok := o[k]; !ok {
			return fmt.Errorf("error: missing %q", k)
		}
	}
	if c, ok := o["code"].(string); !ok || !codeRe.MatchString(c) {
		return fmt.Errorf("error: code must match ^E[0-9]{3}$")
	}
	for _, k := range []string{"message", "cause", "remediation", "docs_url"} {
		if x, ok := o[k]; ok {
			if _, ok := x.(string); !ok {
				return fmt.Errorf("error: %s must be a string", k)
			}
		}
	}
	if !isIntIn(o["exit_code"], 1, 255) {
		return fmt.Errorf("error: exit_code must be an integer from 1 to 255")
	}
	if c, ok := o["class"].(string); !ok || !classes[c] {
		return fmt.Errorf("error: class is not an exit class")
	}
	return nil
}

// isIntIn reports whether v is a JSON number with an integral value in
// [lo, hi]; like JSON Schema, 2.0 counts as an integer.
func isIntIn(v any, lo, hi float64) bool {
	n, ok := v.(json.Number)
	if !ok {
		return false
	}
	f, err := strconv.ParseFloat(n.String(), 64)
	if err != nil || math.IsInf(f, 0) || f != math.Trunc(f) {
		return false
	}
	return f >= lo && f <= hi
}

func checkMeta(v any) error {
	o, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("meta: not an object")
	}
	if err := onlyKeys(o, "meta", "deprecations", "warnings"); err != nil {
		return err
	}
	if d, ok := o["deprecations"]; ok {
		list, ok := d.([]any)
		if !ok {
			return fmt.Errorf("meta: deprecations must be an array")
		}
		for _, it := range list {
			m, ok := it.(map[string]any)
			if !ok {
				return fmt.Errorf("meta: deprecation is not an object")
			}
			if err := onlyKeys(m, "deprecation", "old", "new", "removal_at"); err != nil {
				return err
			}
			for _, k := range []string{"old", "new", "removal_at"} {
				if _, ok := m[k].(string); !ok {
					return fmt.Errorf("deprecation: %s must be a string", k)
				}
			}
		}
	}
	if w, ok := o["warnings"]; ok {
		list, ok := w.([]any)
		if !ok {
			return fmt.Errorf("meta: warnings must be an array")
		}
		for _, it := range list {
			if _, ok := it.(string); !ok {
				return fmt.Errorf("meta: a warning is not a string")
			}
		}
	}
	return nil
}
