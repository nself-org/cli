package admin

import (
	"bufio"
	"fmt"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
	"io"
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
	if jsonOn(cmd) {
		return runAdminLogsJSON(cmd, logsCmd, cid, follow)
	}
	logsCmd.Stdout = os.Stdout
	logsCmd.Stderr = os.Stderr
	return logsCmd.Run()
}

// runAdminLogsJSON prints the v1.5 JSON form of `admin logs`: one envelope, or
// with --follow a stream (contract:cli.json-stream v1) of log records ending
// in a result line. docker's stderr (its own diagnostics) stays on stderr.
func runAdminLogsJSON(cmd *cobra.Command, logsCmd *exec.Cmd, cid string, follow bool) error {
	logsCmd.Stderr = os.Stderr
	pipe, err := logsCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("admin logs: %w", err)
	}
	if err := logsCmd.Start(); err != nil {
		return fmt.Errorf("admin logs: %w", err)
	}
	w := output.Default()
	data := LogsData{Container: cid, Lines: []string{}}
	count := 0
	werr := readLines(pipe, func(line string) error {
		count++
		if follow {
			return output.StreamRecord(w.Out, commandKey(cmd), LogRecord{Type: "log", Message: line})
		}
		data.Lines = append(data.Lines, line)
		return nil
	})
	if rerr := logsCmd.Wait(); werr == nil {
		werr = rerr
	}
	if follow {
		_ = output.StreamEnd(w.Out, commandKey(cmd), LogsEnd{Lines: count}, werr)
		if werr != nil {
			return silentReported{werr}
		}
		return nil
	}
	if werr != nil {
		return werr
	}
	return emit(cmd, data, false)
}

// readLines calls fn for each line of r until EOF or fn fails.
func readLines(r io.Reader, fn func(string) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if err := fn(sc.Text()); err != nil {
			return err
		}
	}
	return sc.Err()
}

// silentReported marks a stream failure already written as the last (error
// envelope) line, so main prints no second document.
type silentReported struct{ cause error }

func (e silentReported) Error() string { return e.cause.Error() }
func (e silentReported) Unwrap() error { return e.cause }

// Silent tells main the error envelope is already on stdout.
func (silentReported) Silent() bool { return true }
