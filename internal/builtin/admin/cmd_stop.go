package admin

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
)

func stopCommand(r runner) *cobra.Command {
	cmd := &cobra.Command{Use: "stop", Short: "Stop the Admin container", RunE: r.runAdminStop}
	cmd.Flags().Bool("force", false, "Skip graceful drain and force-stop")
	return cmd
}

// runAdminStop stops the admin container gracefully (or forcefully with --force).
func (r runner) runAdminStop(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cid := r.adminContainerID()
	force, _ := cmd.Flags().GetBool("force")

	dockerArgs := []string{"stop"}
	if force {
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
