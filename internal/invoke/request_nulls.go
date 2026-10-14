package invoke

// Purpose: refuse a null item in the args or argv array of a request.
// Inputs: the raw JSON value of the args or argv member.
// Outputs: an E420 error, or nil.
// Constraints: encoding/json decodes a null item of a []string as the empty
// string, so {"args":[null]} would run with an empty argument while a schema
// validator or gateway that reads the bytes exactly sees a request the
// schema forbids (items are strings). The check walks the array once and does
// not descend into nested values: those fail the typed decode as before.

import (
	"bytes"
	"encoding/json"
)

// checkNoNullItems refuses a null directly inside the array raw. A value that
// is not an array is left to the typed decode (null for the whole member stays
// the empty list).
func checkNoNullItems(raw []byte, member string) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	_, _ = dec.Token() // the opening bracket
	depth := 0
	for dec.More() || depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return badRequest("request is not valid: %v", err)
		}
		switch t := tok.(type) {
		case json.Delim:
			if t == '[' || t == '{' {
				depth++
			} else {
				depth--
			}
		case nil:
			if depth == 0 {
				return badRequest("%s must not hold a null item", member)
			}
		}
	}
	return nil
}
