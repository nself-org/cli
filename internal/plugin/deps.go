package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// validateConsumes checks that every plugin listed in the consuming plugin's
// Consumes field is already installed on disk. A plugin in Consumes declares
// that it will call that plugin's HTTP API via X-Source-Plugin; if the target
// is not installed the call will always fail. A name that fails ValidateName is
// a hard error (P7-PLUG-20). A missing plugin is a warning, not a hard
// error, to avoid blocking installs in environments where the consumed plugin
// will be installed separately (e.g., modular deploy). The warning is printed
// to stderr. S43-T17.
func validateConsumes(pluginDir, pluginName string, consumes []string) error {
	var missing []string
	for _, dep := range consumes {
		if dep == pluginName {
			continue // self-reference is harmless, skip
		}
		if err := ValidateName(dep); err != nil {
			return fmt.Errorf("plugin %q consumes an invalid plugin name: %w", pluginName, err)
		}
		depManifestPath := filepath.Join(pluginDir, dep, "plugin.json")
		if _, err := os.Stat(depManifestPath); os.IsNotExist(err) {
			missing = append(missing, dep)
		}
	}
	if len(missing) > 0 {
		// Warn only — the user may install the consumed plugins afterwards or
		// deploy them on separate hosts. A hard error here would break modular
		// installs. The AllowedCallers middleware enforces policy at runtime.
		fmt.Fprintf(os.Stderr,
			"[warn] %s declares inter-plugin calls to: %s — ensure those plugins are installed before starting\n",
			pluginName, strings.Join(missing, ", "),
		)
	}
	return nil
}

// compareSemver compares two version strings with CompareSemver and returns
// -1, 0 or 1. It never fails: a version CompareSemver cannot parse sorts below
// any parseable one, and two unparseable versions compare equal (0). Unlike the
// old lenient parser it reads "v1.2.3" as 1.2.3 (it used to read 0.2.3) and
// orders a pre-release below its release.
func compareSemver(a, b string) int {
	if c, err := CompareSemver(a, b); err == nil {
		return c
	}
	_, errA := parseSemver(a)
	_, errB := parseSemver(b)
	switch {
	case errA != nil && errB != nil:
		return 0
	case errA != nil:
		return -1
	default:
		return 1
	}
}
