package commands

// Purpose: Log streaming core split out of logs.go (CLI-R12 Batch B
// mechanical file-size split). Runs `docker compose logs` and pipes each
// line through the logs_filter.go transforms, plus the running-services
// helper shared by the summary/top reports.
// Inputs: a context, project workdir, the docker compose args to run, and
// LogsOptions for per-line filtering.
// Outputs: filtered/formatted lines written to the given io.Writer; a list
// of running service names.
// Constraints: pure move, no behavior change; streamLogLines is the single
// docker spawn of the logs stream (human, legacy JSON and v1.5 JSON paths).

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// streamFilteredLogs runs docker with composeArgs and streams filtered/formatted output to w.
func streamFilteredLogs(ctx context.Context, workdir string, composeArgs []string, opts LogsOptions, w io.Writer) error {
	return streamLogLines(ctx, workdir, composeArgs, opts, func(line string) error {
		if opts.JSON {
			jsonLine := logLineToJSON(line)
			_, _ = fmt.Fprintln(w, jsonLine)
		} else {
			formatted := formatLogLine(line, "", opts)
			_, _ = fmt.Fprintln(w, formatted)
		}
		return nil
	})
}

// streamLogLines runs docker with composeArgs and calls fn with every line that
// passes the opts filters (the one place the logs commands spawn docker). An
// error from fn stops the stream, kills docker and is returned. A cancelled
// parent context ends the stream without an error.
func streamLogLines(parent context.Context, workdir string, composeArgs []string, opts LogsOptions, fn func(line string) error) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	dockerCmd := exec.CommandContext(ctx, "docker", composeArgs...)
	dockerCmd.Dir = workdir
	dockerCmd.Stderr = os.Stderr

	if opts.Follow {
		dockerCmd.Stdin = os.Stdin
	}

	stdout, err := dockerCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := dockerCmd.Start(); err != nil {
		return fmt.Errorf("starting docker: %w", err)
	}

	var fnErr error
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		filtered, ok := filterLogLine(scanner.Text(), opts)
		if !ok {
			continue
		}
		if fnErr = fn(filtered); fnErr != nil {
			cancel()
			break
		}
	}

	waitErr := dockerCmd.Wait()
	if fnErr != nil {
		return fnErr
	}
	if parent.Err() != nil {
		return nil
	}
	return waitErr
}

// getRunningServices returns the list of service names from docker compose ps --services.
func getRunningServices(ctx context.Context, workdir string) ([]string, error) {
	out, err := exec.CommandContext(ctx, "docker", "compose", "ps", "--services").Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose ps --services: %w", err)
	}
	var services []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			services = append(services, line)
		}
	}
	return services, nil
}
