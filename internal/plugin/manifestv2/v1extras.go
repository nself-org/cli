package manifestv2

import (
	"encoding/json"
	"strings"
)

// v1Extras are the v1 keys that map to v2 only when their value has a shape
// that converts without losing anything: `routes` (plugin API docs) to
// rest_routes, `capabilities` to the optional v2 capabilities list, and `env` or
// `env_vars` to env {required, optional}. A value that does not convert stays
// unmapped: UnmappedV1Keys lists it and the migrate tool refuses it.
type v1Extras struct {
	RestRoutes   []RestRoute
	Capabilities []string
	Env          *Env
	blocked      map[string]bool // keys present and non-empty but not convertible
}

// extraKeys are the exact key names mapExtras reads.
var extraKeys = []string{"routes", "capabilities", "env", "env_vars"}

func isExtraKey(k string) bool { return in(extraKeys, k) }

// IsMappedV1Key reports whether the normalizer carries or converts the v1 key k
// in at least one shape (the migrate tool refuses a drop list that names one).
func IsMappedV1Key(k string) bool {
	return v1MappedKeys[strings.ToLower(k)] || isExtraKey(k) || in(ForbiddenKeys, k)
}

// mapExtras converts the conditional keys of raw (a decoded top-level v1 object).
func mapExtras(raw map[string]json.RawMessage) v1Extras {
	x := v1Extras{blocked: map[string]bool{}}
	if v, ok := present(raw, "routes"); ok {
		if _, both := present(raw, "rest_routes"); both {
			x.blocked["routes"] = true
		} else if rr, ok := v1Routes(v); ok {
			x.RestRoutes = rr
		} else {
			x.blocked["routes"] = true
		}
	}
	if v, ok := present(raw, "capabilities"); ok {
		var c []string
		if json.Unmarshal(v, &c) == nil && c != nil {
			x.Capabilities = c
		} else {
			x.blocked["capabilities"] = true
		}
	}
	env, haveEnv := present(raw, "env")
	vars, haveVars := present(raw, "env_vars")
	switch {
	case haveEnv && haveVars:
		x.blocked["env"], x.blocked["env_vars"] = true, true
	case haveEnv || haveVars:
		key, v := "env", env
		if haveVars {
			key, v = "env_vars", vars
		}
		if e, ok := v1Env(v); ok {
			x.Env = e
		} else {
			x.blocked[key] = true
		}
	}
	return x
}

// present returns raw[key] when it holds something (not null, "", [] or {}).
func present(raw map[string]json.RawMessage, key string) (json.RawMessage, bool) {
	v, ok := raw[key]
	if !ok {
		return nil, false
	}
	if _, nonEmpty := describeValue(key, v); !nonEmpty {
		return nil, false
	}
	return v, true
}

// v1Routes maps v1 routes entries {method, path, auth, description?, hmac?} to
// rest_routes (description becomes summary). Any other key, or a non-string
// value, makes the list unconvertible.
func v1Routes(v json.RawMessage) ([]RestRoute, bool) {
	var list []map[string]json.RawMessage
	if json.Unmarshal(v, &list) != nil {
		return nil, false
	}
	out := make([]RestRoute, 0, len(list))
	for _, e := range list {
		var r RestRoute
		fields := map[string]*string{"method": &r.Method, "path": &r.Path, "auth": &r.Auth, "description": &r.Summary, "hmac": &r.HMAC}
		for k, raw := range e {
			dst, known := fields[k]
			if !known || json.Unmarshal(raw, dst) != nil {
				return nil, false
			}
		}
		out = append(out, r)
	}
	return out, true
}

// v1Env converts {required: [name], optional: [name]}, the shape env carries.
// Defaults, descriptions and per-variable flags have no place in v2 env, so any
// richer value stays unmapped.
func v1Env(v json.RawMessage) (*Env, bool) {
	var o map[string]json.RawMessage
	if json.Unmarshal(v, &o) != nil {
		return nil, false
	}
	e := &Env{}
	for k, raw := range o {
		var names []string
		if (k != "required" && k != "optional") || json.Unmarshal(raw, &names) != nil {
			return nil, false
		}
		if k == "required" {
			e.Required = names
		} else {
			e.Optional = names
		}
	}
	return e, true
}
