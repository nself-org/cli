package commands

// Purpose: Log streaming core split out of logs.go (CLI-R12 Batch B
// mechanical file-size split). Runs `docker compose logs` and pipes each
// line through the logs_filter.go transforms, plus the running-services
// helper shared by the summary/top reports.
// Inputs: a context, project workdir, the docker compose args to run, and
// LogsOptions for per-line filtering.
// Outputs: filtered/formatted lines written to the given io.Writer; a list
// of running service names.
// Constraints: streamLogLines shares filtering across human, legacy JSON and
// v1.5 JSON paths; process handling belongs to internal/docker.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/docker"
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

// maxLogLineBytes caps one docker log line; a longer line ends the stream
// with an error instead of a silent stop.
const maxLogLineBytes = 4 << 20

// streamLogLines runs docker with composeArgs and calls fn with every line that
// passes the opts filters. An error from fn stops the stream, kills docker and
// is returned. A cancelled
// parent context ends the stream without an error.
func streamLogLines(parent context.Context, workdir string, composeArgs []string, opts LogsOptions, fn func(line string) error) error {
	var stdin io.Reader
	if opts.Follow {
		stdin = os.Stdin
	}
	return docker.NewCompose().StreamLogs(parent, workdir, composeArgs[2:], stdin, os.Stderr, func(line string) error {
		filtered, ok := filterLogLine(line, opts)
		if !ok {
			return nil
		}
		return fn(filtered)
	})
}

// getRunningServices returns the list of service names from docker compose ps --services.
func getRunningServices(ctx context.Context, workdir string) ([]string, error) {
	var out bytes.Buffer
	err := docker.NewCompose().PsOutput(ctx, workdir, &out, nil, "--services")
	if err != nil {
		return nil, fmt.Errorf("docker compose ps --services: %w", err)
	}
	var services []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			services = append(services, line)
		}
	}
	return services, nil
}
