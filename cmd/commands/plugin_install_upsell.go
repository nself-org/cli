package commands

// Purpose: Free-account prompt and upsell-counter helpers split out of
// plugin.go (CLI-R12 Batch B mechanical file-size split). Called from
// runPluginInstall (plugin_install.go) around the actual install loop —
// never registers commands or touches cobra directly.
// Inputs: an install context, the ping/telemetry URL, and the list of
// plugin names being installed.
// Outputs: an optional interactive Y/n prompt, a persisted free-install
// counter file under ~/.nself, and an occasional upsell message on stderr.
// Constraints: pure move, no behavior change.

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/count"
	"github.com/nself-org/cli/internal/ui"
)

// maybeRegisterFreeAccount prompts the user to create a free account if they
// do not yet have a license key and are installing only free plugins.
// The generated key is stored in NSELF_PLUGIN_LICENSE_KEY for this session.
func maybeRegisterFreeAccount(ctx context.Context, pingURL string, pluginNames []string) error {
	// Only prompt when all requested plugins are free (not in paidPlugins).
	// If any pro plugin is in the list, the normal license flow handles it.
	allFree := true
	for _, name := range pluginNames {
		if plugin.IsPaidPlugin(name) {
			allFree = false
			break
		}
	}
	if !allFree {
		return nil
	}

	ui.Info("To track your installs and enable usage insights, nSelf uses a free account key.")
	fmt.Fprintf(os.Stderr, "Create a free account? [Y/n] ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return nil // EOF or no tty — skip silently
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if answer != "" && answer != "y" && answer != "yes" {
		return nil // user declined
	}

	key, err := plugin.RegisterFreeAccount(ctx, pingURL)
	if err != nil {
		return err
	}

	// Store for this session.
	if err := os.Setenv("NSELF_PLUGIN_LICENSE_KEY", key); err != nil {
		return fmt.Errorf("storing free key in env: %w", err)
	}

	ui.Success("Free account created. Run `nself license set " + key + "` to persist it.")
	return nil
}

// freeInstallCounterFile returns the path to the file tracking how many free
// plugins this device has installed (for upsell threshold).
func freeInstallCounterFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nself", "free_install_count")
}

// maybeShowFreeUpsell increments the free-install counter and shows a one-line
// upsell prompt when the 3rd install is reached (items 679/686 from S20).
func maybeShowFreeUpsell(justInstalled int) {
	counterPath := freeInstallCounterFile()

	// Read existing count.
	existing := 0
	if data, err := os.ReadFile(counterPath); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			existing = n
		}
	}

	newCount := existing + justInstalled

	// Write updated count (best-effort; log at debug if it fails).
	_ = os.MkdirAll(filepath.Dir(counterPath), 0700)
	if err := os.WriteFile(counterPath, []byte(strconv.Itoa(newCount)), 0600); err != nil {
		ui.Warn("plugin counter write failed: " + err.Error())
	}

	// Show upsell on reaching (or crossing) the threshold.
	if existing < 3 && newCount >= 3 {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "You've installed 3 free plugins. "+upsellUnlockLine())
		fmt.Fprintln(os.Stderr, "  nself.org/plus")
		fmt.Fprintln(os.Stderr, "")
	}
}

// upsellUnlockLine renders the free-tier upsell sentence using the current
// advertised plugin count, read from the embedded counts.json artifact
// (internal/plugin/count) instead of a hand-typed number that drifts from
// the registries. Falls back to a count-free sentence if the artifact ever
// fails to parse — an upsell message is never worth a hard failure over.
func upsellUnlockLine() string {
	a, err := count.Load()
	if err != nil {
		return "Unlock every plugin with nSelf+ for $3.99/mo."
	}
	return fmt.Sprintf("Unlock all %d plugins with nSelf+ for $3.99/mo.", a.Advertised)
}
