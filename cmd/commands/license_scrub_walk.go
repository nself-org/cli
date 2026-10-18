package commands

// Purpose: the reflect walk of the licence scrubber (P7-SURF-16, D-0981): a
// copy of a document value with every string leaf scrubbed. It reaches map
// keys and values, []byte and json.RawMessage text, and the exported fields
// promoted from an unexported embedded struct; a cycle or a value nested past
// scrubDepthMax is cut to its zero value, never copied unscrubbed.
// Inputs: a reflect.Value of a document. Outputs: a scrubbed copy of its type.
// Constraints: an unexported embedded pointer is out of reach (the registry
// test TestAccountDocTypesAreScrubbable rejects such a document type).
// SPORT: P7-SURF-16, contract:cli.json-envelope

import "reflect"

// scrubDepthMax bounds the walk: a value nested deeper than this (or cyclic)
// is cut to its zero value, never copied unscrubbed.
const scrubDepthMax = 64

// value returns a copy of src with every string leaf scrubbed.
func (sc scrubber) value(src reflect.Value) reflect.Value {
	return sc.walk(src, 0, map[uintptr]bool{})
}

// walk is value with a depth and the pointers of the path from the root: a
// reference met again on its own path is a cycle and ends as a zero value.
func (sc scrubber) walk(src reflect.Value, depth int, path map[uintptr]bool) reflect.Value {
	if depth > scrubDepthMax {
		return reflect.Zero(src.Type())
	}
	switch src.Kind() {
	case reflect.String:
		out := reflect.New(src.Type()).Elem()
		out.SetString(sc.str(src.String()))
		return out
	case reflect.Pointer, reflect.Map:
		if src.IsNil() {
			return src
		}
		id := src.Pointer()
		if path[id] {
			return reflect.Zero(src.Type())
		}
		path[id] = true
		defer delete(path, id)
		if src.Kind() == reflect.Map {
			out := reflect.MakeMapWithSize(src.Type(), src.Len())
			for it := src.MapRange(); it.Next(); {
				out.SetMapIndex(sc.walk(it.Key(), depth+1, path), sc.walk(it.Value(), depth+1, path))
			}
			return out
		}
		out := reflect.New(src.Type().Elem())
		out.Elem().Set(sc.walk(src.Elem(), depth+1, path))
		return out
	case reflect.Interface:
		if src.IsNil() {
			return src
		}
		out := reflect.New(src.Type()).Elem()
		out.Set(sc.walk(src.Elem(), depth+1, path))
		return out
	case reflect.Slice:
		if src.IsNil() {
			return src
		}
		if src.Type().Elem().Kind() == reflect.Uint8 { // []byte, json.RawMessage: text on the wire
			out := reflect.New(src.Type()).Elem()
			out.SetBytes([]byte(sc.str(string(src.Bytes()))))
			return out
		}
		if src.Len() > 0 { // a slice holding itself shares its first element's address
			id := src.Pointer()
			if path[id] {
				return reflect.Zero(src.Type())
			}
			path[id] = true
			defer delete(path, id)
		}
		out := reflect.MakeSlice(src.Type(), src.Len(), src.Len())
		for i := 0; i < src.Len(); i++ {
			out.Index(i).Set(sc.walk(src.Index(i), depth+1, path))
		}
		return out
	case reflect.Array:
		out := reflect.New(src.Type()).Elem()
		for i := 0; i < src.Len(); i++ {
			out.Index(i).Set(sc.walk(src.Index(i), depth+1, path))
		}
		return out
	case reflect.Struct:
		out := reflect.New(src.Type()).Elem()
		out.Set(src)
		sc.fields(src, out, depth, path)
		return out
	}
	return src
}

// fields scrubs the exported fields of src into out, and the exported fields
// promoted from an unexported embedded struct (encoding/json writes them).
func (sc scrubber) fields(src, out reflect.Value, depth int, path map[uintptr]bool) {
	for i := 0; i < src.NumField(); i++ {
		f := src.Type().Field(i)
		switch {
		case f.IsExported():
			out.Field(i).Set(sc.walk(src.Field(i), depth+1, path))
		case f.Anonymous && f.Type.Kind() == reflect.Struct:
			sc.fields(src.Field(i), out.Field(i), depth+1, path)
		}
	}
}
