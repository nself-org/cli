package manifestv2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// validateV1 is the check the v1 normalizer runs. It mirrors v1.4.12's
// validateManifest (name, version, port, np_ tables, language, status and
// deprecation block, consumes/provides) and adds nothing: the normalizer must
// never reject a file the released CLI accepts (a v1 file need not state
// category or description). The runtime permission allowlist stays with
// internal/plugin. Validate is for v2-native input and the migrate tool, which
// requires a valid v2 file before it writes one.
func validateV1(m *Manifest) error {
	if !nameRe.MatchString(m.Name) {
		return invalid("name", "must be a slug matching "+NamePattern)
	}
	if !versionRe.MatchString(m.Version) {
		return invalid("version", "must be semver (X.Y.Z, optional -pre and +build)")
	}
	if m.Port >= 65536 {
		return invalid("port", "must be between 1 and 65535")
	}
	for i, t := range m.Tables {
		if !strings.HasPrefix(t, "np_") {
			return invalidf("tables["+itoa(i)+"]", "%q must start with np_", t)
		}
	}
	if m.Language != "" && !in(Languages, m.Language) {
		return invalid("language", "must be one of "+strings.Join(Languages, ", "))
	}
	for _, g := range []struct {
		key  string
		list []string
	}{{"consumes", m.Consumes}, {"provides", m.Provides}} {
		for i, v := range g.list {
			if !nameRe.MatchString(v) {
				return invalidf(g.key+"["+itoa(i)+"]", "%q is not a plugin slug", v)
			}
		}
	}
	return validateDeprecation(m)
}

// Unmapped is one v1 key the normalizer has no v2 home for.
type Unmapped struct {
	Key  string // top-level key as written
	Path string // JSON path of its value, e.g. $.config
	Kind string // object, array, string, number or bool
	Size int    // entries of an object or array, else 0
}

// String renders "$.config (object, 3 entries)".
func (u Unmapped) String() string {
	if u.Kind == "object" || u.Kind == "array" {
		return fmt.Sprintf("%s (%s, %d entr%s)", u.Path, u.Kind, u.Size, map[bool]string{true: "y", false: "ies"}[u.Size == 1])
	}
	return fmt.Sprintf("%s (%s)", u.Path, u.Kind)
}

// v1MappedKeys is the set of lowercase top-level keys the v1 decode reads (the
// json tags and names of v1Manifest, matched case-insensitively like
// encoding/json does) plus manifest_version.
var v1MappedKeys = func() map[string]bool {
	out := map[string]bool{"manifest_version": true}
	t := reflect.TypeOf(v1Manifest{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "" {
			name = f.Name
		}
		out[strings.ToLower(name)] = true
	}
	return out
}()

// UnmappedV1Keys lists the non-empty top-level keys of a v1 plugin.json that
// the normalizer does not carry into v2 (config, hooks, actions, notes,
// api_version, entry and so on, plus a routes, capabilities, env or env_vars
// value whose shape does not convert losslessly), sorted by key. Registry-owned
// keys (ForbiddenKeys) are not listed: ADR 0008 and the release pipeline own
// them, see RegistryKeys. A null, empty string, empty list or empty object has
// nothing to lose and is skipped.
func UnmappedV1Keys(data []byte) ([]Unmapped, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, invalid("plugin.json", "not a JSON object: "+err.Error())
	}
	blocked := mapExtras(raw).blocked
	var out []Unmapped
	for k, v := range raw {
		lk := strings.ToLower(k)
		if isExtraKey(k) {
			if !blocked[k] {
				continue
			}
		} else if v1MappedKeys[lk] || in(ForbiddenKeys, k) {
			continue
		}
		if u, ok := describeValue(k, v); ok {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// RegistryKeys lists the registry-owned keys (E111 in v2) a v1 file carries;
// converting removes them on purpose and the migrate tool says so.
func RegistryKeys(data []byte) []string {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return nil
	}
	return forbiddenIn(raw)
}

func describeValue(key string, v json.RawMessage) (Unmapped, bool) {
	u := Unmapped{Key: key, Path: "$." + key}
	t := bytes.TrimSpace(v)
	if len(t) == 0 {
		return u, false
	}
	switch t[0] {
	case '{':
		var o map[string]json.RawMessage
		_ = json.Unmarshal(t, &o)
		u.Kind, u.Size = "object", len(o)
	case '[':
		var a []json.RawMessage
		_ = json.Unmarshal(t, &a)
		u.Kind, u.Size = "array", len(a)
	case '"':
		var s string
		_ = json.Unmarshal(t, &s)
		u.Kind = "string"
		if s == "" {
			return u, false
		}
	case 't', 'f':
		u.Kind = "bool"
	case 'n':
		return u, false
	default:
		u.Kind = "number"
	}
	return u, u.Kind != "object" && u.Kind != "array" || u.Size > 0
}
