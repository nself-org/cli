package manifestv2

import (
	"encoding/json"
	"strings"
)

// derivedKeys are v1 keys that have a v2 home but whose value is only safe to
// take once the rest of the manifest is derived: each is accepted when it fills
// an empty v2 value or repeats the value already derived, and stays unmapped
// (so the migrate tool refuses it) when it conflicts or its shape does not fit.
//
//	replacedBy, replacement, deprecated -> deprecation (v1 shape)
//	docker_image                        -> service.image (kind compose)
//	internalPort                        -> service.port (equal to the derived port)
//	health_check_path                   -> service.healthcheck (equal to the derived one)
//	depends_on                          -> requires.plugins
//	migrations                          -> migrations, only as {dir: "migrations", apply: "boot"}
//
// deprecatedSince, deprecated_in and removal_target (version strings; v2
// deprecation holds dates), a v1 commands list, docker, serviceType and
// schema_version have no lossless v2 home and are never mapped.
var derivedKeys = []string{"deprecated", "replacedBy", "replacement", "docker_image", "internalPort", "health_check_path", "depends_on", "migrations"}

func isDerivedKey(k string) bool { return in(derivedKeys, k) }

// applyDerived fills m from the derived keys of raw and returns the keys that
// are present and could not be mapped.
func applyDerived(m *Manifest, raw map[string]json.RawMessage) map[string]bool {
	blocked := map[string]bool{}
	text := func(key string) (string, bool) {
		v, ok := present(raw, key)
		if !ok {
			return "", false
		}
		var s string
		if json.Unmarshal(v, &s) != nil || s == "" {
			blocked[key] = true
			return "", false
		}
		return s, true
	}
	compose := m.Service != nil && m.Service.Kind == KindCompose

	if v, ok := present(raw, "deprecated"); ok {
		if string(v) != "true" || m.Deprecation == nil || m.Deprecation.State == "" {
			blocked["deprecated"] = true
		}
	}
	var repl []string
	for _, k := range []string{"replacedBy", "replacement"} {
		if s, ok := text(k); ok {
			repl = append(repl, k+"="+s)
		}
	}
	if len(repl) > 0 {
		vals := map[string]bool{}
		for _, kv := range repl {
			vals[kv[strings.Index(kv, "=")+1:]] = true
		}
		var only string
		for s := range vals {
			only = s
		}
		d := m.Deprecation
		if d == nil || d.State == "" || len(vals) != 1 || (d.ReplacedBy != "" && d.ReplacedBy != only) {
			for _, kv := range repl {
				blocked[kv[:strings.Index(kv, "=")]] = true
			}
		} else {
			d.ReplacedBy = only
		}
	}
	if s, ok := text("docker_image"); ok {
		if compose && m.Service.Image == nil {
			m.Service.Image = &s
		} else {
			blocked["docker_image"] = true
		}
	}
	if v, ok := present(raw, "internalPort"); ok {
		var n int
		if json.Unmarshal(v, &n) != nil || !compose || m.Service.Port == nil || *m.Service.Port != n {
			blocked["internalPort"] = true
		}
	}
	if s, ok := text("health_check_path"); ok && (!compose || m.Service.Healthcheck == nil || *m.Service.Healthcheck != s) {
		blocked["health_check_path"] = true
	}
	if v, ok := present(raw, "depends_on"); ok {
		var names []string
		good := json.Unmarshal(v, &names) == nil && len(names) > 0 && (m.Requires == nil || len(m.Requires.Plugins) == 0)
		for _, n := range names {
			good = good && nameRe.MatchString(n)
		}
		if good {
			if m.Requires == nil {
				m.Requires = &Requires{}
			}
			m.Requires.Plugins = names
		} else {
			blocked["depends_on"] = true
		}
	}
	if v, ok := present(raw, "migrations"); ok {
		var mg Migrations
		var obj map[string]json.RawMessage
		if json.Unmarshal(v, &obj) == nil && len(obj) == 2 && json.Unmarshal(v, &mg) == nil && mg.Dir == "migrations" && mg.Apply == "boot" {
			m.Migrations = &mg
		} else {
			blocked["migrations"] = true
		}
	}
	return blocked
}
