package manifestv2

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	nameRe    = regexp.MustCompile(NamePattern)
	versionRe = regexp.MustCompile(VersionPattern)
	extRe     = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
)

func in(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// Validate checks the structural rules of a v2 manifest: required keys, slug
// and semver shape, enums, service rules, np_ tables, boot migrations,
// deprecation state and the commands block. Failures are E106 naming the field,
// except a core-verb command, which is E113. It does not check compatibility
// keys (CheckCompat, E112) or the runtime permission allowlist.
func Validate(m *Manifest) error {
	if m.ManifestVersion != ManifestVersion {
		return invalid("manifest_version", "must be 2")
	}
	if !nameRe.MatchString(m.Name) {
		return invalid("name", "must be a slug matching "+NamePattern)
	}
	if !versionRe.MatchString(m.Version) {
		return invalid("version", "must be semver (X.Y.Z, optional -pre and +build)")
	}
	for _, f := range []struct{ key, val string }{{"description", m.Description}, {"category", m.Category}} {
		if strings.TrimSpace(f.val) == "" {
			return invalid(f.key, "is required")
		}
	}
	if !in(Licenses, m.License) {
		return invalidf("license", "must be one of %s, got %q", strings.Join(Licenses, ", "), m.License)
	}
	if !in(Maturities, m.Maturity) {
		return invalidf("maturity", "must be one of %s, got %q", strings.Join(Maturities, ", "), m.Maturity)
	}
	for _, check := range []func(*Manifest) error{validateService, validateShared, validateBlocks, validateDeprecation, validateCommands} {
		if err := check(m); err != nil {
			return err
		}
	}
	return nil
}

func validateService(m *Manifest) error {
	s := m.Service
	if s == nil {
		return invalid("service", "is required")
	}
	if !in(ServiceKinds, s.Kind) {
		return invalidf("service.kind", "must be one of %s, got %q", strings.Join(ServiceKinds, ", "), s.Kind)
	}
	if s.Port != nil && (*s.Port < 1 || *s.Port > 65535) {
		return invalid("service.port", "must be between 1 and 65535")
	}
	if s.Port != nil && m.Port > 0 && *s.Port != m.Port {
		return invalidf("service.port", "is %d but port is %d", *s.Port, m.Port)
	}
	if s.Kind == KindCompose {
		if s.Compose == nil || *s.Compose == "" {
			return invalid("service.compose", "is required for kind compose")
		}
		if s.Healthcheck == nil || *s.Healthcheck == "" {
			return invalid("service.healthcheck", "is required for kind compose")
		}
	}
	return nil
}

func validateShared(m *Manifest) error {
	if m.Port < 0 || m.Port > 65535 {
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
	}{{"consumes", m.Consumes}, {"provides", m.Provides}, {"dependencies", m.Dependencies}, {"optionalDependencies", m.OptionalDependencies}} {
		for i, v := range g.list {
			if !nameRe.MatchString(v) {
				return invalidf(g.key+"["+itoa(i)+"]", "%q is not a plugin slug", v)
			}
		}
	}
	if err := checkPermissionShape(m.Permissions); err != nil {
		return err
	}
	if m.DocsURL != "" {
		if u, err := url.Parse(m.DocsURL); err != nil || u.Scheme != "https" || u.Host == "" {
			return invalid("docs_url", "must be an https URL")
		}
	}
	return nil
}

func validateBlocks(m *Manifest) error {
	if m.Schema != nil {
		want := "np_" + strings.ReplaceAll(m.Name, "-", "_")
		if *m.Schema != want {
			return invalidf("schema", "must be %q, got %q", want, *m.Schema)
		}
	}
	if m.Migrations != nil && (m.Migrations.Dir != "migrations" || m.Migrations.Apply != "boot") {
		return invalid("migrations", `must be {"dir": "migrations", "apply": "boot"}`)
	}
	if m.Seed != nil && len(m.Seed.Command) == 0 {
		return invalid("seed.command", "must be a non-empty argv")
	}
	for i, r := range m.Routes {
		p := "routes[" + itoa(i) + "]"
		if !strings.HasPrefix(r.Path, "/") {
			return invalid(p+".path", "must start with /")
		}
		if r.UpstreamPort < 1 || r.UpstreamPort > 65535 {
			return invalid(p+".upstream_port", "must be between 1 and 65535")
		}
	}
	if r := m.Requires; r != nil {
		for i, p := range r.Plugins {
			if !nameRe.MatchString(p) {
				return invalidf("requires.plugins["+itoa(i)+"]", "%q is not a plugin slug", p)
			}
		}
		for i, e := range r.PostgresExtensions {
			if !extRe.MatchString(e) {
				return invalidf("requires.postgres_extensions["+itoa(i)+"]", "%q is not an extension name", e)
			}
		}
	}
	return nil
}

func validateDeprecation(m *Manifest) error {
	d := m.Deprecation
	if d != nil && d.State != "" && !in(States, d.State) {
		return invalid("deprecation.state", "must be one of "+strings.Join(States, ", "))
	}
	if m.Maturity == MaturityDeferred && (d == nil || d.State == "") {
		return invalid("deprecation.state", "is required when maturity is deferred")
	}
	if d != nil && d.State == StateDeprecated {
		for _, f := range []struct{ key, val string }{{"announcedDate", d.AnnouncedDate}, {"eolDate", d.EOLDate}, {"migrationGuide", d.MigrationGuide}} {
			if f.val == "" {
				return invalid("deprecation."+f.key, "is required for state deprecated")
			}
		}
	}
	return nil
}

// checkPermissionShape accepts nil, an array of strings or an object whose
// values are arrays of strings (the two shapes v1 files use).
func checkPermissionShape(p any) error {
	switch v := p.(type) {
	case nil:
		return nil
	case []any:
		for i, e := range v {
			if _, ok := e.(string); !ok {
				return invalid("permissions["+itoa(i)+"]", "must be a string")
			}
		}
	case map[string]any:
		for k, e := range v {
			list, ok := e.([]any)
			if !ok {
				return invalid("permissions."+k, "must be an array of strings")
			}
			for _, s := range list {
				if _, ok := s.(string); !ok {
					return invalid("permissions."+k, "must be an array of strings")
				}
			}
		}
	default:
		return invalid("permissions", "must be an array of strings or an object of string arrays")
	}
	return nil
}
