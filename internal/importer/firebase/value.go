package firebase

// Purpose: decode Firestore REST typed values into plain JSON values plus a type tag.
// Inputs: one REST Value (`{"stringValue": "x"}`). Outputs: a JSON-marshalable value and its kind.
// Constraints: integers and doubles keep their exact digits (json.Number); a string with a NUL
// cannot be stored in Postgres text or jsonb, so it is refused instead of altered.
import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

type kind string

const (
	kString    kind = "string"
	kInteger   kind = "integer"
	kDouble    kind = "double"
	kBoolean   kind = "boolean"
	kTimestamp kind = "timestamp"
	kNull      kind = "null"
	kMap       kind = "map"
	kArray     kind = "array"
	kBytes     kind = "bytes"
	kReference kind = "reference"
	kGeoPoint  kind = "geopoint"
)

const maxDepth = 60

var errNul = errors.New("document holds a NUL character")

func checkText(s string) error {
	if strings.IndexByte(s, 0) >= 0 {
		return errNul
	}
	return nil
}

// decodeValue converts one typed REST value. Exactly one type key is expected.
func decodeValue(raw json.RawMessage, depth int) (any, kind, error) {
	if depth > maxDepth {
		return nil, "", errors.New("document nesting too deep")
	}
	var typed map[string]json.RawMessage
	if json.Unmarshal(raw, &typed) != nil || len(typed) != 1 {
		return nil, "", errors.New("document value invalid")
	}
	for key, v := range typed {
		return decodeTyped(key, v, depth)
	}
	return nil, "", errors.New("document value invalid")
}

func decodeTyped(key string, v json.RawMessage, depth int) (any, kind, error) {
	var s string
	switch key {
	case "nullValue":
		return nil, kNull, nil
	case "booleanValue":
		var b bool
		if json.Unmarshal(v, &b) != nil {
			return nil, "", errors.New("document boolean invalid")
		}
		return b, kBoolean, nil
	case "integerValue":
		// REST sends int64 as a JSON string; a bare number is also accepted.
		lit := strings.Trim(string(v), `"`)
		if _, err := strconv.ParseInt(lit, 10, 64); err != nil {
			return nil, "", errors.New("document integer invalid")
		}
		return json.Number(lit), kInteger, nil
	case "doubleValue":
		if json.Unmarshal(v, &s) == nil { // "NaN", "Infinity", "-Infinity"
			if s != "NaN" && s != "Infinity" && s != "-Infinity" {
				return nil, "", errors.New("document double invalid")
			}
			return s, kDouble, nil
		}
		if _, err := strconv.ParseFloat(string(v), 64); err != nil {
			return nil, "", errors.New("document double invalid")
		}
		return json.Number(string(v)), kDouble, nil
	case "timestampValue", "stringValue", "bytesValue", "referenceValue":
		if json.Unmarshal(v, &s) != nil {
			return nil, "", errors.New("document string invalid")
		}
		if err := checkText(s); err != nil {
			return nil, "", err
		}
		return s, map[string]kind{"timestampValue": kTimestamp, "stringValue": kString, "bytesValue": kBytes, "referenceValue": kReference}[key], nil
	case "geoPointValue":
		var g map[string]any
		if json.Unmarshal(v, &g) != nil {
			return nil, "", errors.New("document geopoint invalid")
		}
		return g, kGeoPoint, nil
	case "arrayValue":
		var a struct {
			Values []json.RawMessage `json:"values"`
		}
		if json.Unmarshal(v, &a) != nil {
			return nil, "", errors.New("document array invalid")
		}
		out := make([]any, 0, len(a.Values))
		for _, item := range a.Values {
			p, _, err := decodeValue(item, depth+1)
			if err != nil {
				return nil, "", err
			}
			out = append(out, p)
		}
		return out, kArray, nil
	case "mapValue":
		var m struct {
			Fields map[string]json.RawMessage `json:"fields"`
		}
		if json.Unmarshal(v, &m) != nil {
			return nil, "", errors.New("document map invalid")
		}
		out, _, err := decodeFields(m.Fields, depth+1)
		return out, kMap, err
	}
	return nil, "", errors.New("document value type unknown")
}

// decodeFields decodes a field map; keys are checked for NUL like strings. kinds maps each field
// to the type tag of its value.
func decodeFields(fields map[string]json.RawMessage, depth int) (map[string]any, map[string]kind, error) {
	out := make(map[string]any, len(fields))
	kinds := make(map[string]kind, len(fields))
	for name, raw := range fields {
		if err := checkText(name); err != nil {
			return nil, nil, err
		}
		p, k, err := decodeValue(raw, depth)
		if err != nil {
			return nil, nil, err
		}
		out[name], kinds[name] = p, k
	}
	return out, kinds, nil
}

// scalarText is the Postgres text form of a promotable scalar value.
func scalarText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}
