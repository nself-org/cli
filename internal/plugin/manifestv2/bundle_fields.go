package manifestv2

import (
	"errors"
	"regexp"

	"github.com/nself-org/cli/internal/errs"
)

// Bundle metadata (contract:cwm.manifest.v2). A plugin manifest may carry one
// optional `bundle` block describing how the plugin takes part in a bundle
// plan: the capabilities it provides, the constraints it needs, the roles it
// serves, its resource floor, migration entries, health path and tier.
// The block is additive: manifests without it parse to Bundle == nil.

// Patterns, shared by validation and the generated schema.
const (
	// RangePattern matches a semver range: "*", or one or more comparators
	// separated by spaces or commas, e.g. ">=1.2.0 <2.0.0", "^1.4", "1.x".
	RangePattern = `^(?:\*|(?:[\^~]|[<>]=?|=)?v?[0-9]+(?:\.(?:[0-9]+|[xX*])){0,2}(?:-[a-zA-Z0-9.]+)?)(?:[ ,]+(?:[\^~]|[<>]=?|=)?v?[0-9]+(?:\.(?:[0-9]+|[xX*])){0,2}(?:-[a-zA-Z0-9.]+)?)*$`
	// TierPattern matches a bundle tier name, a lowercase slug.
	TierPattern = `^[a-z][a-z0-9-]*$`
	// MigrationPattern matches a migration entry: a relative .sql path or a
	// plain migration name, with no leading slash and no dot segments.
	MigrationPattern = `^[A-Za-z0-9][A-Za-z0-9_.-]*(?:/[A-Za-z0-9][A-Za-z0-9_.-]*)*$`
)

// Sentinels for bundle metadata failures. Each is joined with
// errs.ErrPluginManifest by the validator, so errors.Is matches both.
var (
	ErrBundleRange    = errors.New("invalid bundle requires range")
	ErrBundleResource = errors.New("invalid bundle resources")
	ErrBundleTier     = errors.New("invalid bundle tier")
	ErrBundleRole     = errors.New("invalid bundle role")
)

var (
	rangeRe     = regexp.MustCompile(RangePattern)
	tierRe      = regexp.MustCompile(TierPattern)
	migrationRe = regexp.MustCompile(MigrationPattern)
)

// BundleProfile is the optional `bundle` block of a v2 manifest.
type BundleProfile struct {
	// Provides lists capability slugs the plugin offers to a bundle plan.
	Provides []string `json:"provides,omitempty"`
	// Requires lists plugin or capability constraints the plan must satisfy.
	Requires []Constraint `json:"requires,omitempty"`
	// Roles lists the roles the plugin serves and the services behind each.
	Roles []Role `json:"roles,omitempty"`
	// Resources is the minimum the plugin needs from the host.
	Resources *Resources `json:"resources,omitempty"`
	// Migrations lists migration entries to apply, in order.
	Migrations []string `json:"migrations,omitempty"`
	// Health is the HTTP path that reports the plugin ready (starts with /).
	Health string `json:"health,omitempty"`
	// Tier names the bundle tier the plugin belongs to.
	Tier string `json:"tier"`
}

// Constraint is one requirement on a plugin or capability, with a semver range.
type Constraint struct {
	Name  string `json:"name"`
	Range string `json:"range"`
}

// Role is a named job in a bundle plan, backed by one or more services.
type Role struct {
	ID string `json:"id"`
	// Services are plugin names: this plugin or one of the Requires entries.
	Services []string `json:"services"`
}

// Resources is the host floor for the plugin. Zero means no stated minimum.
type Resources struct {
	MemoryMB  int `json:"memory_mb,omitempty"`
	DiskMB    int `json:"disk_mb,omitempty"`
	CPUMillis int `json:"cpu_millis,omitempty"`
}

// validateBundle checks the optional bundle block. Range, resource, tier and
// role failures carry their ErrBundle* sentinel plus errs.ErrPluginManifest;
// every other failure is a plain invalid() E106.
func validateBundle(m *Manifest) error {
	b := m.Bundle
	if b == nil {
		return nil
	}
	for i, v := range b.Provides {
		if !nameRe.MatchString(v) {
			return invalidf("bundle.provides["+itoa(i)+"]", "%q is not a slug matching "+NamePattern, v)
		}
	}
	known := map[string]bool{m.Name: true}
	for i, c := range b.Requires {
		p := "bundle.requires[" + itoa(i) + "]"
		if c.Name == "" {
			return invalid(p+".name", "is required")
		}
		if c.Range == "" || !rangeRe.MatchString(c.Range) {
			return bundleErr(ErrBundleRange, p+".range", "must be a semver range matching "+RangePattern)
		}
		known[c.Name] = true
	}
	if err := validateRoles(b.Roles, known); err != nil {
		return err
	}
	if r := b.Resources; r != nil && (r.MemoryMB < 0 || r.DiskMB < 0 || r.CPUMillis < 0) {
		return bundleErr(ErrBundleResource, "bundle.resources", "values must not be negative")
	}
	for i, v := range b.Migrations {
		if !migrationRe.MatchString(v) || hasDotSegment(v) {
			return invalidf("bundle.migrations["+itoa(i)+"]", "%q must match "+MigrationPattern, v)
		}
	}
	if b.Health != "" && b.Health[0] != '/' {
		return invalid("bundle.health", "must start with /")
	}
	if b.Tier == "" || !tierRe.MatchString(b.Tier) {
		return bundleErr(ErrBundleTier, "bundle.tier", "is required and must match "+TierPattern)
	}
	return nil
}

func validateRoles(roles []Role, known map[string]bool) error {
	seen := map[string]bool{}
	for i, r := range roles {
		p := "bundle.roles[" + itoa(i) + "]"
		if !nameRe.MatchString(r.ID) {
			return invalid(p+".id", "must be a slug matching "+NamePattern)
		}
		if seen[r.ID] {
			return bundleErr(ErrBundleRole, p+".id", "duplicate role "+r.ID)
		}
		seen[r.ID] = true
		if len(r.Services) == 0 {
			return bundleErr(ErrBundleRole, p+".services", "must name at least one service")
		}
		for j, s := range r.Services {
			if !known[s] {
				return bundleErr(ErrBundleRole, p+".services["+itoa(j)+"]", s+" is neither this plugin nor a bundle.requires name")
			}
		}
	}
	return nil
}

// hasDotSegment reports a "." or ".." path segment.
func hasDotSegment(s string) bool {
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '/' {
			if seg := s[start:i]; seg == "." || seg == ".." {
				return true
			}
			start = i + 1
		}
	}
	return false
}

// bundleErr is invalid() with a sentinel joined in, so errors.Is matches both
// the sentinel and errs.ErrPluginManifest.
func bundleErr(sentinel error, field, msg string) error {
	return errs.Wrap("E106", field+": "+msg, errors.Join(sentinel, errs.ErrPluginManifest))
}
