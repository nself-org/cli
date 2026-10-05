package manifestv2

import (
	"encoding/json"
	"sort"
)

// fillShared copies the shared v1 keys into m, converting the polymorphic
// shapes released CLIs accept into the one canonical shape v2 authors write.
func fillShared(m *Manifest, v *v1Manifest) (err error) {
	s := &m.Shared
	s.Author, s.Homepage, s.Repository, s.Tags = v.Author, v.Homepage, v.Repository, v.Tags
	s.Language, s.Port, s.HealthEndpoint, s.PackageManager = v.Language, v.Port, v.HealthEndpoint, v.PackageManager
	s.Framework, s.MinNodeVersion, s.ArchSupport = v.Framework, v.MinNodeVersion, v.ArchSupport
	s.Tables, s.Views, s.Consumes, s.Provides = v.Tables, v.Views, v.Consumes, v.Provides
	s.RestRoutes, s.MultiApp, s.Permissions, s.CompatBlock = v.RestRoutes, v.MultiApp, v.Permissions, v.CompatBlock
	s.Deprecation, s.GraphQL, s.RequiredEntitlements = v.Deprecation, v.GraphQL, v.RequiredEntitlements
	s.MaxNselfVersion, s.Visibility, s.UpdatedAt, s.PlatformChecksums = v.MaxNselfVersion, v.Visibility, v.UpdatedAt, v.PlatformChecksums
	if s.APIEndpoints, err = v1Endpoints(v.APIEndpoints); err != nil {
		return err
	}
	if s.Webhooks, err = v1Webhooks(v.Webhooks); err != nil {
		return err
	}
	if s.EnvVars, err = v1EnvVars(v.EnvVars); err != nil {
		return err
	}
	if s.SystemDependencies, err = v1SystemDeps(v.SystemDependencies); err != nil {
		return err
	}
	s.Dependencies, s.OptionalDependencies, err = v1Deps(v.Dependencies, v.OptionalDependencies)
	return err
}

func absent(raw json.RawMessage) bool { return len(raw) == 0 || string(raw) == "null" }

// v1Deps reads dependencies as a list or as {required, optional}; the grouped
// optional names merge after the top-level optionalDependencies.
func v1Deps(raw json.RawMessage, optional []string) (required, opt []string, err error) {
	opt = optional
	if absent(raw) {
		return nil, opt, nil
	}
	var g struct{ Required, Optional []string }
	if json.Unmarshal(raw, &g) == nil {
		return g.Required, mergeUnique(opt, g.Optional), nil
	}
	if err := json.Unmarshal(raw, &required); err != nil {
		return nil, nil, invalid("dependencies", "expected a list of plugin names or {required, optional}")
	}
	return required, opt, nil
}

func mergeUnique(a, b []string) []string {
	seen := map[string]bool{}
	for _, v := range a {
		seen[v] = true
	}
	for _, v := range b {
		if !seen[v] {
			a = append(a, v)
			seen[v] = true
		}
	}
	return a
}

// v1Endpoints reads apiEndpoints as "METHOD /path" strings or {method, path} objects.
func v1Endpoints(raw json.RawMessage) ([]string, error) {
	if absent(raw) {
		return nil, nil
	}
	var strs []string
	if json.Unmarshal(raw, &strs) == nil {
		return strs, nil
	}
	var objs []struct{ Method, Path string }
	if err := json.Unmarshal(raw, &objs); err != nil {
		return nil, invalid("apiEndpoints", `expected "METHOD /path" strings or {method, path} objects`)
	}
	out := make([]string, 0, len(objs))
	for _, e := range objs {
		switch {
		case e.Method != "" && e.Path != "":
			out = append(out, e.Method+" "+e.Path)
		case e.Path != "":
			out = append(out, e.Path)
		}
	}
	return out, nil
}

// v1Webhooks reads webhooks as an event-name list or an object keyed by event.
func v1Webhooks(raw json.RawMessage) ([]string, error) {
	if absent(raw) {
		return nil, nil
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list, nil
	}
	var byName map[string]string
	if err := json.Unmarshal(raw, &byName); err != nil {
		return nil, invalid("webhooks", "expected event names or an object keyed by event name")
	}
	out := make([]string, 0, len(byName))
	for n := range byName {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

// v1EnvVars reads envVars as declarations, bare names or {required, optional}
// groups (each group a name list or a name to default object).
func v1EnvVars(raw json.RawMessage) ([]EnvVar, error) {
	if absent(raw) {
		return nil, nil
	}
	var objs []EnvVar
	if json.Unmarshal(raw, &objs) == nil {
		return objs, nil
	}
	var names []string
	if json.Unmarshal(raw, &names) == nil {
		return namesToEnv(names, false), nil
	}
	var groups map[string]json.RawMessage
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil, invalid("envVars", "expected declarations, names or required/optional groups")
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	rank := func(k string) int {
		switch k {
		case "required":
			return 0
		case "optional":
			return 1
		}
		return 2
	}
	sort.Slice(keys, func(i, j int) bool {
		if rank(keys[i]) != rank(keys[j]) {
			return rank(keys[i]) < rank(keys[j])
		}
		return keys[i] < keys[j]
	})
	var out []EnvVar
	for _, k := range keys {
		out = append(out, envGroup(groups[k], k == "required")...)
	}
	return out, nil
}

func namesToEnv(names []string, required bool) []EnvVar {
	out := make([]EnvVar, 0, len(names))
	for _, n := range names {
		out = append(out, EnvVar{Name: n, Required: required})
	}
	return out
}

func envGroup(raw json.RawMessage, required bool) []EnvVar {
	var names []string
	if json.Unmarshal(raw, &names) == nil {
		return namesToEnv(names, required)
	}
	var withDefaults map[string]json.RawMessage
	if json.Unmarshal(raw, &withDefaults) != nil {
		return nil
	}
	keys := make([]string, 0, len(withDefaults))
	for k := range withDefaults {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]EnvVar, 0, len(keys))
	for _, k := range keys {
		def := string(withDefaults[k])
		var s string
		if json.Unmarshal(withDefaults[k], &s) == nil {
			def = s
		}
		out = append(out, EnvVar{Name: k, Required: required, Default: def})
	}
	return out
}

// v1SystemDeps reads systemDependencies as {required, recommended}, a list of
// declarations, a list of names, or an empty array.
func v1SystemDeps(raw json.RawMessage) (*SystemDependencies, error) {
	if absent(raw) || string(raw) == "[]" {
		return nil, nil
	}
	var obj SystemDependencies
	if json.Unmarshal(raw, &obj) == nil {
		return &obj, nil
	}
	var list []SystemDependency
	if json.Unmarshal(raw, &list) == nil {
		return &SystemDependencies{Required: list}, nil
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		return nil, invalid("systemDependencies", "expected required/recommended groups or a list of dependencies")
	}
	d := &SystemDependencies{}
	for _, n := range names {
		d.Required = append(d.Required, SystemDependency{Name: n})
	}
	return d, nil
}

// Canonicalize makes absent and empty the same value (nil), so a v1 file and
// its v2 twin decode to equal Manifests. confirm.flags keeps its empty list.
func Canonicalize(m *Manifest) {
	s := &m.Shared
	for _, p := range []*[]string{&s.Tags, &s.ArchSupport, &s.Tables, &s.Views, &s.APIEndpoints, &s.Webhooks,
		&s.Dependencies, &s.OptionalDependencies, &s.Consumes, &s.Provides, &s.RequiredEntitlements} {
		if len(*p) == 0 {
			*p = nil
		}
	}
	if len(s.RestRoutes) == 0 {
		s.RestRoutes = nil
	}
	if len(s.EnvVars) == 0 {
		s.EnvVars = nil
	}
	if len(s.PlatformChecksums) == 0 {
		s.PlatformChecksums = nil
	}
	if d := s.SystemDependencies; d != nil && len(d.Required) == 0 && len(d.Recommended) == 0 {
		s.SystemDependencies = nil
	}
	switch p := s.Permissions.(type) {
	case []any:
		if len(p) == 0 {
			s.Permissions = nil
		}
	case map[string]any:
		if len(p) == 0 {
			s.Permissions = nil
		}
	}
	if len(m.CLICommands) == 0 {
		m.CLICommands = nil
	}
	if len(m.Routes) == 0 {
		m.Routes = nil
	}
	if r := m.Requires; r != nil && r.Nself == "" && len(r.Plugins) == 0 && len(r.PostgresExtensions) == 0 {
		m.Requires = nil
	}
}
