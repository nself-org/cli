package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// plugin is one registry row: name, status and the CLI version range.
type plugin struct{ name, status, min, max string }

// str returns the string value of a raw JSON field, or "" when null, false or absent.
func str(m map[string]json.RawMessage, key string) string {
	var s string
	if json.Unmarshal(m[key], &s) != nil {
		return ""
	}
	return s
}

// registryPlugins reads a registry in file order (jq to_entries keeps it).
// A missing file yields no rows; a bad one is an error.
func registryPlugins(path string) ([]plugin, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	var out []plugin
	for dec.More() {
		k, _ := dec.Token()
		if k != "plugins" {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return nil, err
			}
			continue
		}
		// Token returns '[' for an array without an error, so check the delimiter:
		// an array's elements must not be misread as plugin names.
		if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
			return nil, fmt.Errorf("%s: plugins is not an object", path)
		}
		for dec.More() {
			nameTok, _ := dec.Token()
			var fields map[string]json.RawMessage
			if err := dec.Decode(&fields); err != nil {
				return nil, fmt.Errorf("%s: %v", path, err)
			}
			p := plugin{name: fmt.Sprint(nameTok)}
			p.status = str(fields, "status")
			if r := string(fields["status"]); r == "" || r == "null" || r == "false" {
				p.status = "stable"
			}
			p.min = str(fields, "minNselfVersion")
			if p.min == "" {
				p.min = str(fields, "min_nself_version")
			}
			if p.min == "" {
				p.min = "0.0.0"
			}
			if p.max = str(fields, "maxNselfVersion"); p.max == "" {
				p.max = "99.99.99"
			}
			out = append(out, p)
		}
		dec.Token() // }
	}
	return out, nil
}

// semverLTE reports a <= b comparing major.minor.patch numerically.
// A non-numeric part makes the comparison false, as the shell's failing test did.
func semverLTE(a, b string) bool {
	pa, pb := strings.SplitN(a, ".", 3), strings.SplitN(b, ".", 3)
	for len(pa) < 3 {
		pa = append(pa, "")
	}
	for len(pb) < 3 {
		pb = append(pb, "")
	}
	for i := 0; i < 3; i++ {
		x, e1 := strconv.Atoi(pa[i])
		y, e2 := strconv.Atoi(pb[i])
		if e1 != nil || e2 != nil {
			return false
		}
		if x != y {
			return x < y
		}
	}
	return true
}
