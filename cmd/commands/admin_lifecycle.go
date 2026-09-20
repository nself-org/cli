package commands

// admin_lifecycle.go — the stop and logs subcommands of `nself admin`.
//
// Purpose: run the container-lifecycle half of the admin command group.
// Inputs: the resolved admin container name from adminContainerID().
// Outputs: a stopped container, or streamed logs.
// Constraints: split out of admin.go purely for the 300-line file cap; a
// pure move, no behaviour change.

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// runAdminStop stops the admin container gracefully (or forcefully with --force).
func runAdminStop(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cid := adminContainerID()

	dockerArgs := []string{"stop"}
	if adminStopFlags.force {
		dockerArgs = append(dockerArgs, "-t", "0")
	}
	dockerArgs = append(dockerArgs, cid)

	stopCmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	stopCmd.Stdout = os.Stdout
	stopCmd.Stderr = os.Stderr
	if err := stopCmd.Run(); err != nil {
		return fmt.Errorf("stopping admin container: %w", err)
	}

	fmt.Println("Admin stopped.")
	return nil
}

// runAdminLogs tails or streams admin container logs.
func runAdminLogs(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cid := adminContainerID()

	dockerArgs := []string{"logs"}
	if adminLogsFlags.follow {
		dockerArgs = append(dockerArgs, "--follow")
	}
	dockerArgs = append(dockerArgs, fmt.Sprintf("--tail=%d", adminLogsFlags.tail))
	dockerArgs = append(dockerArgs, cid)

	logsCmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	logsCmd.Stdout = os.Stdout
	logsCmd.Stderr = os.Stderr
	return logsCmd.Run()
}

// runAdminHealth probes GET /health on the admin service.
