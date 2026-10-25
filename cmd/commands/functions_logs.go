package commands

// Purpose: `nself functions logs` split out of functions.go (CLI-R12
// Batch B mechanical file-size split). Tails or streams the functions
// container's docker logs, filtered to lines mentioning the given
// function name.
// Inputs: cobra command flags (--follow, --since, --tail) and the
// positional function name.
// Outputs: matching log lines written to stdout as they are read.
// Constraints: pure move, no behavior change. functionNamePattern and
// functionsCmd (parent) remain in functions.go.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/docker"
	"github.com/spf13/cobra"
)

var functionsLogsFlags struct {
	follow bool
	since  string
	tail   int
}

var functionsLogsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "Stream logs for a function",
	Long: `Tail or stream logs for a deployed function, filtered by function name.

Examples:
  nself functions logs hello-world
  nself functions logs hello-world --follow
  nself functions logs hello-world --tail 50
  nself functions logs hello-world --since 1h`,
	Args: cobra.ExactArgs(1),
	RunE: runFunctionsLogs,
}

func init() {
	functionsLogsCmd.Flags().BoolVar(&functionsLogsFlags.follow, "follow", false, "Stream logs continuously")
	functionsLogsCmd.Flags().StringVar(&functionsLogsFlags.since, "since", "", "Show logs since duration (e.g. 1h, 30m)")
	functionsLogsCmd.Flags().IntVar(&functionsLogsFlags.tail, "tail", 100, "Number of recent log lines")
}

func runFunctionsLogs(cmd *cobra.Command, args []string) error {
	name := args[0]
	if !functionNamePattern.MatchString(name) {
		return fmt.Errorf("invalid function name %q", name)
	}

	cfg, _, cfgErr := loadHealthConfig()
	containerID := "functions"
	if cfgErr == nil {
		containerID = cfg.ServiceContainerName("functions")
	}

	dockerArgs := []string{"logs"}
	if functionsLogsFlags.follow {
		dockerArgs = append(dockerArgs, "--follow")
	}
	if functionsLogsFlags.since != "" {
		dockerArgs = append(dockerArgs, "--since", functionsLogsFlags.since)
	}
	dockerArgs = append(dockerArgs, fmt.Sprintf("--tail=%d", functionsLogsFlags.tail))
	dockerArgs = append(dockerArgs, containerID)

	// Both docker streams carry function output; keep the lines that mention
	// the function.
	return docker.StreamLines(cmd.Context(), "", dockerArgs, nil, true, maxLogLineBytes, func(line string) error {
		if strings.Contains(line, name) || strings.Contains(line, "/v1/"+name) {
			fmt.Println(line)
		}
		return nil
	})
}
