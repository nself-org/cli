package plugin

// installer_finish.go — the post-install reporting tail of installLocked.
//
// Purpose: emit the permission audit log, warn on dangerous permissions,
// print the "run nself build" hint, and fire the anonymous install event.
// Inputs: the plugin name and its resolved registry manifest.
// Outputs: none — every step here is reporting, and none can fail the
// install, which has already succeeded by the time this runs.
// Constraints: split out of installer_locked.go to keep that file within the
// 300-line cap; a pure move, same order, no behaviour change.

import (
	"fmt"
	"log/slog"
	"os"
)

// finishInstall runs the reporting tail of a successful plugin install.
func finishInstall(name string, manifest *PluginManifest) {
	// S71-T02: Emit structured audit log for the granted permission set.
	// One line per install, consumable by Loki. Never logs secret values —
	// only the permission strings declared in the manifest.
	slog.Info("plugin.install.permissions",
		"plugin", name,
		"version", manifest.Version,
		"permissions", manifest.Permissions.Strings(),
	)

	// S71-T02: Warn via doctor when dangerous permissions are present.
	logDangerousPermissions(name, manifest.Permissions.Strings())

	fmt.Fprintf(os.Stderr, "\n\u2139 Run 'nself build' to include %s in your stack.\n", name)

	// S68-T02: Fire-and-forget install-event to plugins.nself.org registry.
	// Silent, 1s timeout, never blocks the install. Sends only an opaque
	// SHA-256 hash of the machine fingerprint — no PII in the payload.
	go postInstallEvent(name)
}
