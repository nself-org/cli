package config

// env_protected.go — security knobs a project env file can never set.
//
// Purpose: Load exports every key of the project's env cascade into the
// process environment, and a cloned project's .env is attacker-controlled
// input. A key here turns off or redirects a trust check (plugin signature
// and checksum enforcement, licence verification, the hosts that serve
// signed artifacts and licence answers), so it is read only from the
// environment the operator started the process with, or from a value the
// CLI itself sets from a command-line flag. A project file that sets one is
// ignored with a warning (P7-PLUG-87 review H1).
// Inputs: a key name, the file it came from and its value.
// Outputs: whether the loader must skip the key; one slog warning per
// file and key per process.
// Constraints: the list is closed and reviewed; adding a key is a security
// change. ENV is not here: it selects the cascade, and no verification
// step reads it since registry installs lost the signature bypass.
// NSELF_V15 is one-way: a project may opt into v1.5 behaviour, but never
// turn off an opt-in the process already has (v1.5 refuses downgrades and
// unsigned licence caches).

import (
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/nself-org/cli/internal/compat"
)

// protectedEnvKeys are the keys only the process environment may set.
var protectedEnvKeys = map[string]bool{
	// Verification bypasses and strictness switches.
	"NSELF_LICENSE_SKIP_VERIFY":       true,
	"NSELF_LICENSE_SKIP_VERIFY_FORCE": true,
	"NSELF_LICENSE_FAIL_OPEN":         true,
	"NSELF_PLUGIN_ALLOW_UNSIGNED":     true,
	"NSELF_PLUGIN_REQUIRE_CHECKSUM":   true,
	"NSELF_PLUGIN_ALLOW_DOWNGRADE":    true,
	"NSELF_SKIP_SBOM_CHECK":           true,
	"NSELF_PLUGIN_INSTALL_TIER":       true,
	// Hosts that serve signed artifacts, catalogues and licence answers.
	"NSELF_PLUGIN_REGISTRY":       true,
	"NSELF_PING_URL":              true,
	"NSELF_PING_API_URL":          true,
	"NSELF_BUNDLES_URL":           true,
	"NSELF_MARKETPLACE_URL":       true,
	"NSELF_TEMPLATE_REGISTRY_URL": true,
	// `nself ci serve` sandbox and repository allowlist.
	"NSELF_CI_ALLOW_UNSANDBOXED": true,
	"NSELF_CI_ALLOWED_REPOS":     true,
}

// IsProtectedEnvKey reports whether key is a security knob that a project env
// file can never set.
func IsProtectedEnvKey(key string) bool {
	return protectedEnvKeys[key]
}

// ProtectedEnvKeys returns the protected keys, sorted.
func ProtectedEnvKeys() []string {
	keys := make([]string, 0, len(protectedEnvKeys))
	for k := range protectedEnvKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// protectedWarned holds "file\x00key" pairs already warned about, so a
// command that loads the cascade several times warns once.
var protectedWarned sync.Map

// skipProtectedKey reports whether the loader must skip key from file f. It
// warns once per file and key when the file's value would have changed the
// process environment. The value is never logged: a project file can hold a
// secret under any name.
func skipProtectedKey(f, key, value string) bool {
	switch {
	case protectedEnvKeys[key]:
		if os.Getenv(key) != value {
			warnProtected(f, key)
		}
		return true
	case key == compat.EnvVar && isV15Value(os.Getenv(key)) && !isV15Value(value):
		warnProtected(f, key)
		return true
	}
	return false
}

// isV15Value is compat.V15's truth test applied to a value.
func isV15Value(v string) bool {
	return v == "1" || strings.EqualFold(v, "true")
}

// warnProtected logs the ignored key once per file and key per process.
func warnProtected(f, key string) {
	if _, seen := protectedWarned.LoadOrStore(f+"\x00"+key, true); !seen {
		slog.Warn("ignoring a security setting in a project env file; a project cannot set it, export it in your shell instead",
			"file", f, "var", key)
	}
}
