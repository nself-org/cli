package commands

// Purpose: RunE implementations for "nself config show" and "nself config
// get". Inputs are the cobra command/args; outputs are printed config values
// or an error.
// Constraints: split out of config.go (CLI-R12) as a pure move, no behavior change.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/output"

	"github.com/spf13/cobra"
)

func runConfigShow(cmd *cobra.Command, args []string) error {
	reveal, _ := cmd.Flags().GetBool("reveal")
	envFlag, _ := cmd.Flags().GetString("env")
	format, _ := cmd.Flags().GetString("format")

	projectDir, err := resolveProjectDir()
	if err != nil {
		return err
	}

	envFile := envFileName(projectDir, envFlag)
	pairs, err := config.ReadEnvFile(envFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("env file not found: %s", envFile)
		}
		return config.EnvReadError(envFile, err)
	}

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// compat.V15(P7-REG-09): --json ignored -> envelope
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut && compat.V15() {
		return output.EmitData(pilotWriter(), "config show", maskedPairs(pairs, keys, reveal))
	}

	switch format {
	case "yaml":
		for _, k := range keys {
			v := maskValue(k, pairs[k], reveal)
			if strings.ContainsAny(v, ": \t#\"'\\") || v == "" {
				fmt.Printf("%s: %q\n", k, v)
			} else {
				fmt.Printf("%s: %s\n", k, v)
			}
		}
	case "json":
		// Bare map in v1.4 mode (and with NSELF_JSON_LEGACY=1), envelope in v1.5.
		return output.EmitLegacyCompatible(pilotWriter(), "config show", maskedPairs(pairs, keys, reveal))
	default: // table
		for _, k := range keys {
			fmt.Printf("%s=%s\n", k, maskValue(k, pairs[k], reveal))
		}
	}
	return nil
}

// maskedPairs is the key/value map of the config JSON outputs, masked exactly
// as the human output masks it.
func maskedPairs(pairs map[string]string, keys []string, reveal bool) map[string]string {
	m := make(map[string]string, len(pairs))
	for _, k := range keys {
		m[k] = maskValue(k, pairs[k], reveal)
	}
	return m
}

// --- S4-T02: config get ---

// configGetValue resolves key for `config get`.
// Inputs: the project directory, the --env flag and the key.
// Outputs: the value in clear and the base name of the file it came from.
// Constraints: with no --env the key goes through the same env cascade as
// `nself build` (config.ExplainActive), so a .env.local override wins and the
// value matches what the stack runs with (D-0919). An explicit --env reads
// only .env.<env>, as before. The process environment is not consulted: the
// command reports project files.
func configGetValue(projectDir, envFlag, key string) (value, file string, err error) {
	if envFlag != "" {
		envFile := envFileName(projectDir, envFlag)
		pairs, err := config.ReadEnvFile(envFile)
		if err != nil {
			if os.IsNotExist(err) {
				return "", "", fmt.Errorf("env file not found: %s", envFile)
			}
			return "", "", config.EnvReadError(envFile, err)
		}
		val, ok := pairs[key]
		if !ok {
			return "", "", fmt.Errorf("key not found: %s", key)
		}
		return val, filepath.Base(envFile), nil
	}

	base := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(base); err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("env file not found: %s", base)
		}
		return "", "", config.EnvReadError(base, err)
	}
	exp, err := config.ExplainActive(projectDir, key)
	if err != nil {
		return "", "", err
	}
	file, val, ok := exp.Winner()
	if !ok {
		return "", "", fmt.Errorf("key not found: %s", key)
	}
	return val, file, nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]
	reveal, _ := cmd.Flags().GetBool("reveal")
	envFlag, _ := cmd.Flags().GetString("env")

	projectDir, err := resolveProjectDir()
	if err != nil {
		return err
	}

	val, file, err := configGetValue(projectDir, envFlag, key)
	if err != nil {
		return err
	}

	shown := maskValue(key, val, reveal)
	// compat.V15(P7-REG-09): --json ignored -> envelope
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut && compat.V15() {
		return output.EmitData(pilotWriter(), "config get", configGetData{
			Key: key, Value: shown, Masked: shown != val, File: file,
		})
	}

	fmt.Println(shown)
	return nil
}

// --- S4-T03: config set ---
