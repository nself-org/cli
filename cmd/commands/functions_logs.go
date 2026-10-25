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
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"

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

	logsCmd := exec.CommandContext(cmd.Context(), "docker", dockerArgs...)
	stdout, err := logsCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pipe: %w", err)
	}
	stderr, err := logsCmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := logsCmd.Start(); err != nil {
		return fmt.Errorf("starting docker logs: %w", err)
	}

	// Filter lines containing the function name and print.
	filterAndPrint := func(r io.Reader) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, name) || strings.Contains(line, "/v1/"+name) {
				fmt.Println(line)
			}
		}
	}

	go filterAndPrint(stderr)
	filterAndPrint(stdout)

	return logsCmd.Wait()
}
