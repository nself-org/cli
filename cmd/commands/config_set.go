package commands

// Purpose: RunE for "nself config set" plus its key/value validators and the
// setEnvFileLine helper that rewrites the env file in place. Inputs are the
// cobra command/args; outputs are an updated env file or an error.
// Constraints: split out of config.go (CLI-R12). P7-LIVE-05 routes the write
// through config_set_plan.go (plan, preview, v1.5 apply); validation, the
// selectable files and the bytes written are unchanged.

import (
	"fmt"
	"os"
	"regexp"

	"github.com/nself-org/cli/internal/reconcile"
	"github.com/nself-org/cli/internal/security"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]
	envFlag, _ := cmd.Flags().GetString("env")

	// Validate key: only [A-Z0-9_] allowed, max 128 chars.
	if err := validateConfigKey(key); err != nil {
		return err
	}
	// Validate value: no null bytes.
	if err := validateConfigValue(value); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	projectDir, err := resolveProjectDir()
	if err != nil {
		return err
	}

	envFile := envFileName(projectDir, envFlag)
	jsonOut, _ := cmd.Flags().GetBool("json")
	// P7-LIVE-05: plan the regeneration the change causes, then write the
	// line (and in v1.5 apply it). See config_set_plan.go.
	return configSetRun{cmd: cmd, projectDir: projectDir, envFile: envFile, key: key, value: value,
		monorepo: projectDir != cwd, jsonOut: jsonOut}.run()
}

// reportWritten prints the result of the env write.
func (r configSetRun) reportWritten(updated bool) {
	// FindNSelfRoot's monorepo convenience silently resolves to a
	// `.backend/` project root above cwd when one exists — fine for reads,
	// but a write with no indication of where it actually landed is
	// dangerous: a user working in a nested app dir sees "Added KEY to
	// .env" with no signal the file is two directories up. Surface the
	// redirection explicitly whenever it happens; this does not change
	// where the write lands, only what gets printed.
	if r.monorepo {
		ui.Info(fmt.Sprintf("Using monorepo backend at %s (found via .backend/ in a parent directory)", r.projectDir))
	}

	if updated {
		ui.Success(fmt.Sprintf("Updated %s in %s", r.key, r.envFile))
	} else {
		ui.Success(fmt.Sprintf("Added %s to %s", r.key, r.envFile))
	}
}

// configKeyRe matches valid config keys: uppercase letters, digits, underscores only.
var configKeyRe = regexp.MustCompile(`^[A-Z0-9_]+$`)

// validateConfigKey returns an error if key contains invalid characters,
// is empty, or exceeds 128 characters.
func validateConfigKey(key string) error {
	if len(key) == 0 {
		return fmt.Errorf("config key cannot be empty")
	}
	if len(key) > 128 {
		return fmt.Errorf("config key too long (max 128 characters)")
	}
	if !configKeyRe.MatchString(key) {
		return fmt.Errorf("config key %q contains invalid characters (only A-Z, 0-9, _ allowed)", key)
	}
	return nil
}

// validateConfigValue returns an error if value contains null bytes.
func validateConfigValue(value string) error {
	for i, b := range []byte(value) {
		if b == 0 {
			return fmt.Errorf("config value contains null byte at position %d", i)
		}
	}
	return nil
}

// setEnvFileLine updates KEY=VALUE in the named file in-place, preserving all
// comments and blank lines. If the key is not found it is appended. Returns
// true when an existing line was replaced, false when the key was appended.
// The new content is reconcile.EnvFileWithLine, the same rewrite the
// `config set` plan previews (P7-LIVE-05).
func setEnvFileLine(envFile, key, value string) (updated bool, err error) {
	// If the file does not exist, create it with just this key.
	if _, statErr := os.Stat(envFile); os.IsNotExist(statErr) {
		content, _, _ := reconcile.EnvFileWithLine(nil, false, key, value)
		if writeErr := os.WriteFile(envFile, content, 0600); writeErr != nil {
			return false, fmt.Errorf("creating %s: %w", envFile, writeErr)
		}
		if chmodErr := security.EnforceFilePermissions(envFile, 0600); chmodErr != nil {
			return false, fmt.Errorf("enforcing permissions on %s: %w", envFile, chmodErr)
		}
		return false, nil
	}

	// Read the existing file and rewrite it line by line.
	old, openErr := os.ReadFile(envFile)
	if openErr != nil {
		return false, fmt.Errorf("opening %s: %w", envFile, openErr)
	}
	content, found, scanErr := reconcile.EnvFileWithLine(old, true, key, value)
	if scanErr != nil {
		return false, fmt.Errorf("scanning %s: %w", envFile, scanErr)
	}
	if writeErr := os.WriteFile(envFile, content, 0600); writeErr != nil {
		return false, fmt.Errorf("writing %s: %w", envFile, writeErr)
	}
	if chmodErr := security.EnforceFilePermissions(envFile, 0600); chmodErr != nil {
		return false, fmt.Errorf("enforcing permissions on %s: %w", envFile, chmodErr)
	}
	return found, nil
}

// --- S4-T04: config list ---

// knownVarsExported mirrors loader.go's knownEnvVars. We access it through the
// exported function below rather than duplicating the list.
