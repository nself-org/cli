package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// canonicalPermissions is the allowlist of valid permission strings for plugin
// manifests. The format is <category>:<qualifier> where qualifier may contain
// additional colon-separated segments (e.g. network:plugin:<name>).
//
// Tier 1 (v1.0.9): declaration + audit log only. No runtime enforcement yet.
// Tier 2 (v1.1.0): install-time prompt + revoke command.
// Tier 3 (v1.2.0): runtime egress enforcement via proxy.
//
// Each permission string below is an exact prefix pattern; the checker strips
// the fixed prefix and validates the remaining segment(s) are non-empty for
// parameterized forms (e.g. "network:plugin:<name>" requires a name).
//
// S71-T01.
var canonicalPermissions = map[string]bool{
	// Network permissions
	"network:internet": true,  // unrestricted outbound internet access
	"network:plugin":   false, // parameterized: network:plugin:<target-plugin-name>
	// Database permissions
	"db:read":  true, // read-only SELECT on plugin-owned tables
	"db:write": true, // INSERT/UPDATE/DELETE on plugin-owned tables
	// Secret permissions
	"secrets:env": false, // parameterized: secrets:env:<VAR_NAME>
	// Filesystem permissions
	"fs:write": false, // parameterized: fs:write:<volume-name>
	// System permissions
	"system:exec": true, // execute arbitrary subprocess commands
	// AI provider permissions
	"ai:provider": false, // parameterized: ai:provider:<provider-name>
}

// validatePermissions validates that every permission declared in m.Permissions
// is known to the canonical allowlist. Unknown permissions are rejected to
// prevent silent capability grants (fail-closed semantics). S71-T01.
//
// For parameterized permissions (network:plugin, secrets:env, fs:write,
// ai:provider) the format must be <category>:<qualifier>:<param> where <param>
// is a non-empty, safe identifier.
func validatePermissions(m *PluginManifest) error {
	// Both forms are validated now. The descriptive object form that 121 shipped
	// manifests use is not a competing vocabulary — it is a more specific one,
	// naming hosts, paths and CRUD verbs where the canonical form names
	// categories. DeriveCanonicalPermissions reduces it, widening at every step
	// so nothing is under-declared, and the result goes through the same
	// fail-closed allowlist as everything else.
	//
	// Before this, those 121 plugins were simply exempt from a check that
	// exists to be mandatory.
	var unknown []string
	for _, perm := range m.Permissions.Effective() {
		if !isKnownPermission(perm) {
			unknown = append(unknown, perm)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown permission%s: %s: %w",
			pluralSuffix(len(unknown)),
			strings.Join(unknown, ", "),
			errs.ErrPluginManifest,
		)
	}
	return nil
}

// isKnownPermission returns true when perm matches a canonical permission
// pattern (exact or parameterized).
func isKnownPermission(perm string) bool {
	// Exact match first.
	if ok, exists := canonicalPermissions[perm]; exists && ok {
		return true
	}

	// Parameterized forms: the key in canonicalPermissions is the base prefix
	// (value == false). Check for "<prefix>:<param>" format.
	for base, exact := range canonicalPermissions {
		if exact {
			continue // already handled above
		}
		prefix := base + ":"
		if strings.HasPrefix(perm, prefix) {
			param := perm[len(prefix):]
			// Param must be non-empty and contain no shell-dangerous characters.
			if len(param) > 0 && isSafePermissionParam(param) {
				return true
			}
		}
	}
	return false
}

// permParamPattern matches safe permission parameter segments: alphanumeric,
// hyphens, underscores, dots, and colons (for nested params like ai:provider:openai).
var permParamPattern = regexp.MustCompile(`^[a-zA-Z0-9_\-.:]+$`)

// isSafePermissionParam returns true when param contains only safe characters
// for permission parameter segments.
func isSafePermissionParam(param string) bool {
	return permParamPattern.MatchString(param)
}

// pluralSuffix returns "s" when n != 1, else "".
func pluralSuffix(n int) string {
	if n != 1 {
		return "s"
	}
	return ""
}

// ParseManifest reads a plugin.json file at the given path and returns a
// validated PluginManifest. It returns an error if the file cannot be read,
// is not valid JSON, or is missing any required field (name, version,
// description, category, license).
func parseManifest(path string) (*PluginManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading plugin manifest: %w", err)
	}

	var m PluginManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing plugin manifest: %w", err)
	}

	if err := validateRequiredFields(&m); err != nil {
		return nil, err
	}

	if err := validateManifest(&m); err != nil {
		return nil, err
	}

	return &m, nil
}

// validateRequiredFields checks that all required manifest fields are present.
func validateRequiredFields(m *PluginManifest) error {
	var missing []string

	if m.Name == "" {
		missing = append(missing, "name")
	}
	if m.Version == "" {
		missing = append(missing, "version")
	}
	if m.Description == "" {
		missing = append(missing, "description")
	}
	if m.Category == "" {
		missing = append(missing, "category")
	}
	if m.License == "" {
		missing = append(missing, "license")
	}

	if len(missing) > 0 {
		return fmt.Errorf("plugin manifest missing required fields %v: %w", missing, errs.ErrPluginManifest)
	}

	return nil
}

var (
	namePattern    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	versionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?(\+[a-zA-Z0-9.]+)?$`)
)

// validPluginStatuses enumerates the 6 canonical lifecycle values (S58-T01).
// Absent status defaults to "stable" (backwards compatible — CR-A).
var validPluginStatuses = map[string]bool{
	"experimental": true,
	"planned":      true,
	"beta":         true,
	"stable":       true,
	"deprecated":   true,
	"eol":          true,
}

// validLanguages lists allowed values for the Language field.
var validLanguages = map[string]bool{
	"go":         true,
	"rust":       true,
	"typescript": true,
	"python":     true,
	"bash":       true,
}

// validateManifest performs strict validation of manifest field values beyond
// simple presence checks. It validates name format, semver version, port
// range, table name prefixes, and language enum.
func validateManifest(m *PluginManifest) error {
	if !namePattern.MatchString(m.Name) {
		return fmt.Errorf("plugin name must be lowercase with hyphens only: %w", errs.ErrPluginManifest)
	}

	if !versionPattern.MatchString(m.Version) {
		return fmt.Errorf("version must be semver format (X.Y.Z, optional pre-release -suffix and build +suffix): %w", errs.ErrPluginManifest)
	}

	if m.Port > 0 && m.Port >= 65536 {
		return fmt.Errorf("port must be between 1 and 65535: %w", errs.ErrPluginManifest)
	}

	for _, table := range m.Tables {
		if !strings.HasPrefix(table, "np_") {
			return fmt.Errorf("table names must start with np_ prefix: %w", errs.ErrPluginManifest)
		}
	}

	if m.Language != "" && !validLanguages[m.Language] {
		return fmt.Errorf("language must be one of: go, rust, typescript, python, bash: %w", errs.ErrPluginManifest)
	}

	// Validate status field (S58-T01): 6 valid values; absent defaults to stable.
	if m.PublishStatus != "" && !validPluginStatuses[m.PublishStatus] {
		validList := "experimental, planned, beta, stable, deprecated, eol"
		return fmt.Errorf("invalid plugin status %q: must be one of: %s: %w", m.PublishStatus, validList, errs.ErrPluginManifest)
	}

	// Validate deprecation block (S58-T02): required when status == "deprecated".
	if m.PublishStatus == "deprecated" {
		if m.Deprecation == nil {
			return fmt.Errorf("status \"deprecated\" requires a deprecation block with announcedDate, eolDate, and migrationGuide: %w", errs.ErrPluginManifest)
		}
		if m.Deprecation.AnnouncedDate == "" {
			return fmt.Errorf("deprecation.announcedDate is required: %w", errs.ErrPluginManifest)
		}
		if m.Deprecation.EOLDate == "" {
			return fmt.Errorf("deprecation.eolDate is required: %w", errs.ErrPluginManifest)
		}
		if m.Deprecation.MigrationGuide == "" {
			return fmt.Errorf("deprecation.migrationGuide is required: %w", errs.ErrPluginManifest)
		}
	}

	// Validate consumes/provides entries: each must be a valid plugin name.
	for _, dep := range m.Consumes {
		if !namePattern.MatchString(dep) {
			return fmt.Errorf("consumes entry %q is not a valid plugin name (lowercase-with-hyphens): %w", dep, errs.ErrPluginManifest)
		}
	}
	for _, dep := range m.Provides {
		if !namePattern.MatchString(dep) {
			return fmt.Errorf("provides entry %q is not a valid plugin name (lowercase-with-hyphens): %w", dep, errs.ErrPluginManifest)
		}
	}

	// Validate permissions against canonical allowlist (S71-T01).
	// Fail-closed: unknown permissions block install.
	if err := validatePermissions(m); err != nil {
		return err
	}

	return nil
}
