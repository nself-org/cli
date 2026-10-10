// JSON data-type registration for the build domain (build, reset, clean, uninstall).
//
// Purpose: `nself build --json` answers with the v1 envelope whose data is the
// change plan (contract:cli.change-plan v1): the plan `--plan` shows, or the
// plan that was applied. One data type for the command (REG D1). The flags are
// additive in both compat modes, so the envelope is not v1.5-only.
//
// reset, clean and uninstall answer with the list of removed paths plus one
// flag describing what was kept or pruned (P7-SURF-26). Their --json was
// accepted and ignored before P7-REG (EPIC D8), so those three are envelope in
// v1.5 mode only and print exactly what they printed before in v1.4 mode.
// Human text still prints; in JSON mode stdout is isolated so it lands on
// stderr and the document is the only thing on stdout.
// Constraints: registers from init(); see registry_types.go for the rules.

package commands

import (
	"fmt"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
)

// ResetResult is the data of `nself reset --json`.
type ResetResult struct {
	// Removed lists the generated paths that were deleted, relative to the project.
	Removed []string `json:"removed"`
	// KeptData is true when the database volumes were left in place (--keep-data).
	KeptData bool `json:"kept_data"`
}

// CleanResult is the data of `nself clean --json`.
type CleanResult struct {
	// Removed lists the generated artifacts that were deleted, relative to the project.
	Removed []string `json:"removed"`
	// HostPrune is true when the host-wide `docker system prune` (--all) ran.
	HostPrune bool `json:"host_prune"`
}

// UninstallResult is the data of `nself uninstall --json`.
type UninstallResult struct {
	// Removed lists the paths that were deleted, relative to the project.
	Removed []string `json:"removed"`
	// KeptData is true when the database volumes were left in place (no --purge).
	KeptData bool `json:"kept_data"`
}

func init() {
	registerJSONType("build", reconcile.Plan{})
	registerJSONType("reset", ResetResult{})
	registerJSONType("clean", CleanResult{})
	registerJSONType("uninstall", UninstallResult{})
	for _, p := range []string{"reset", "clean", "uninstall"} {
		registerV15OnlyEnvelope(p)
	}
}

// buildEnvelopeWanted reports whether this invocation prints the v1 envelope.
func buildEnvelopeWanted(cmd *cobra.Command) bool {
	jsonOut, _ := cmd.Flags().GetBool("json")
	// compat.V15(P7-SURF-26): --json ignored -> envelope
	return jsonOut && compat.V15()
}

// emitBuildResult writes the envelope for command when JSON mode is on.
func emitBuildResult(cmd *cobra.Command, command string, data any) error {
	if !buildEnvelopeWanted(cmd) {
		return nil
	}
	return output.EmitData(pilotWriter(), command, data)
}

// errConfirmRequired is what JSON mode returns where human mode prints a
// cancellation and exits 0: a machine caller must not read a refusal as done.
func errConfirmRequired(what string) error {
	return fmt.Errorf("%s was not confirmed: pass --yes to run it without a prompt: %w", what, errs.ErrDestructiveBlocked)
}

// nonNil returns s, or an empty slice when s is nil, so JSON arrays are never null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
