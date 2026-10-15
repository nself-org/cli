package main

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/compat"
)

// Purpose: export the CLI's resolved mode to in-process SDK gates (D-0705).
// Inputs: the mode resolved by internal/compat at command startup.
// Outputs: NSELF_V15=1 or 0, or an environment update error.
// Constraints: run before commands; keep the SDK's standalone rule unchanged.
// SPORT: contract:cli.plugin-exec-env.
func exportCompatMode(v15 bool) error {
	value := "0"
	if v15 {
		value = "1"
	}
	if err := os.Setenv(compat.EnvVar, value); err != nil {
		return fmt.Errorf("export compatibility mode: %w", err)
	}
	return nil
}
