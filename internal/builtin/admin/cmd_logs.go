package admin

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
)

func logsCommand(r runner) *cobra.Command {
	cmd := &cobra.Command{Use: "logs", Short: "Tail Admin container logs", RunE: r.runAdminLogs}
	cmd.Flags().Bool("follow", false, "Stream logs continuously")
	cmd.Flags().Int("tail", 100, "Number of recent lines to show")
	return cmd
}

// runAdminLogs tails or streams admin container logs.
func (r runner) runAdminLogs(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cid := r.adminContainerID()
	follow, _ := cmd.Flags().GetBool("follow")
	tail, _ := cmd.Flags().GetInt("tail")

	dockerArgs := []string{"logs"}
	if follow {
		dockerArgs = append(dockerArgs, "--follow")
	}
	dockerArgs = append(dockerArgs, fmt.Sprintf("--tail=%d", tail))
	dockerArgs = append(dockerArgs, cid)

	logsCmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	logsCmd.Stdout = os.Stdout
	logsCmd.Stderr = os.Stderr
	return logsCmd.Run()
}
