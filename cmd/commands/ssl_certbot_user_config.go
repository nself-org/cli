package commands

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

// certbotLookupUser is replaceable so tests never read root's real config.
var certbotLookupUser = user.LookupId

// Purpose: locate per-user configs that a certbot scheduler can replay.
// Inputs: the invoking user's HOME/XDG_CONFIG_HOME and root's account home.
// Outputs: candidate cli.ini paths; absent files are handled by the scanner.
// Constraints: read only; lookup failures refuse rather than omit root's hooks.
func certbotUserConfigs() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locate certbot user config: %w", err)
	}
	configs := []string{filepath.Join(home, ".config", "letsencrypt", "cli.ini")}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		configs = append(configs, filepath.Join(xdg, "letsencrypt", "cli.ini"))
	}
	if runtime.GOOS != "windows" {
		root, err := certbotLookupUser("0")
		if err != nil {
			return nil, fmt.Errorf("locate root's certbot config: %w", err)
		}
		configs = append(configs, filepath.Join(root.HomeDir, ".config", "letsencrypt", "cli.ini"))
	}
	return configs, nil
}
