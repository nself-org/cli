package commands

// Purpose: print, during `nself deploy --dry-run`, one sentence per custom
// service naming the env vars it will receive — so an operator reviewing a
// dry-run can see that adding a var to .env.prod does NOT reach a custom
// service's container unless CS_N_ENV_PASSTHROUGH or CS_N_ENV names it
// explicitly (internal/compose/custom_services.go:31-71's deliberately
// narrow surface). Names only, never values — safe for any dry-run output.
// Inputs:  workdir (used to config.Load the project so CustomServices is
//          populated).
// Outputs: lines printed to stdout; no error is ever surfaced to the caller
//          — a dry-run listing is best-effort and must never fail the
//          command over a config load problem the real build step will
//          report anyway.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

// printCustomServiceEnvSummary loads workdir's config and, for each declared
// custom service, prints one line listing every env var name it will
// receive. Silently does nothing if the config can't be loaded or there are
// no custom services — a dry-run preview is advisory, not authoritative.
func printCustomServiceEnvSummary(workdir string) {
	cfg, err := config.Load(workdir)
	if err != nil || len(cfg.CustomServices) == 0 {
		return
	}
	for _, cs := range cfg.CustomServices {
		names := compose.CustomServiceEnvVarNames(cs)
		fmt.Printf("  [dry-run] Custom service %s will receive: %s\n", cs.Name, strings.Join(names, ", "))
	}
}
