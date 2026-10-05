package portable

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// checkKeys walks the manifest JSON and refuses ambiguous object keys:
//   - the same key twice in one object (Go keeps the last, others the first);
//   - two keys that differ only in case (Go matches struct fields without
//     regard to case, so SCHEMA_VERSION would override schema_version);
//   - a key that names a Manifest field only case-insensitively.
//
// Maps (auth.hash_algorithms) get the duplicate check only: their keys are
// data, and "bcrypt" and "BCRYPT" are two names.
// Unknown fields are left to the decoder (DisallowUnknownFields).
func checkKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	return walkKeys(dec, reflect.TypeOf(Manifest{}), "")
}

func walkKeys(dec *json.Decoder, t reflect.Type, path string) error {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	if d == '[' {
		var elem reflect.Type
		if t != nil && t.Kind() == reflect.Slice {
			elem = t.Elem()
		}
		for dec.More() {
			if err := walkKeys(dec, elem, path+"[]"); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	}
	fields := jsonFields(t)
	seen := map[string]bool{}
	folded := map[string]string{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return err
		}
		key := kt.(string)
		where := strings.TrimPrefix(path+"."+key, ".")
		if seen[key] {
			return fmt.Errorf("duplicate key %q", where)
		}
		seen[key] = true
		var child reflect.Type
		switch {
		case fields != nil:
			ft, exact := fields[key]
			if !exact {
				if want := fieldFold(fields, key); want != "" {
					return fmt.Errorf("key %q must be spelled %q", where, want)
				}
			}
			if prev, dup := folded[strings.ToLower(key)]; dup {
				return fmt.Errorf("keys %q and %q differ only in case", prev, key)
			}
			folded[strings.ToLower(key)] = key
			child = ft
		case t != nil && t.Kind() == reflect.Map:
			child = t.Elem()
		}
		if err := walkKeys(dec, child, where); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	return err
}

// jsonFields maps each JSON field name of struct type t to its type; nil when
// t is not a struct.
func jsonFields(t reflect.Type) map[string]reflect.Type {
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	out := make(map[string]reflect.Type, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = t.Field(i).Type
	}
	return out
}

// fieldFold returns the field name that equals key without regard to case, or "".
func fieldFold(fields map[string]reflect.Type, key string) string {
	for name := range fields {
		if strings.EqualFold(name, key) {
			return name
		}
	}
	return ""
}
