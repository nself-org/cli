package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

// compareSemver compares two dotted-numeric version strings (e.g. "1.2.3").
// It returns -1 if a < b, 0 if a == b, and 1 if a > b. Missing segments are
// treated as zero, so "1.2" equals "1.2.0".
func compareSemver(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	max := len(aParts)
	if len(bParts) > max {
		max = len(bParts)
	}

	for i := 0; i < max; i++ {
		av := 0
		bv := 0
		if i < len(aParts) {
			av, _ = strconv.Atoi(aParts[i])
		}
		if i < len(bParts) {
			bv, _ = strconv.Atoi(bParts[i])
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}
