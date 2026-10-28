package commands

// Purpose: v1.5 JSON output of `nself logs` (P7-SURF-27). Without --follow the
// command prints one envelope (LogsDocument); with --follow it prints a stream
// (contract:cli.json-stream v1): one compact envelope per log line, then one
// last line with data.type "result" or an error envelope.
// Inputs: the context, project workdir, docker compose args and LogsOptions the
// human path already builds; docker is spawned by streamLogLines only.
// Outputs: envelopes on the pilot writer (the isolated real stdout); nothing
// else on stdout. Human output and the v1.4 per-line JSON are untouched.
// Constraints: the service prefix is kept in v1.5 JSON mode (no
// --no-log-prefix) so a record can name its service.

import (
	"context"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

// composePrefixRE matches the "<service>  | " prefix docker compose puts on every line.
var recordTimestampPrefixRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?[ \t]?`)

var composePrefixRE = regexp.MustCompile(`^([^\s|]+)\s*\|\s?`)

// logRecordFromLine splits one raw compose log line into its record: service
// prefix, leading timestamp, detected level and the remaining message.
func logRecordFromLine(line string) LogRecord {
	rec := LogRecord{Type: logRecordType}
	rest := line
	if m := composePrefixRE.FindStringSubmatch(rest); m != nil {
		rec.Service = m[1]
		rest = rest[len(m[0]):]
	}
	if ts := recordTimestampPrefixRE.FindString(rest); ts != "" {
		rec.Time = strings.TrimSpace(ts)
		rest = rest[len(ts):]
	}
	rec.Message = rest
	rec.Level = detectLineLevel(rest)
	return rec
}

// logsEnvelopeMode reports whether `logs --json` prints the v1.5 envelope or
// stream instead of the pre-contract per-line JSON.
func logsEnvelopeMode(jsonOut bool) bool {
	// compat.V15(P7-SURF-27): logs --json per-line legacy JSON -> envelope / json-stream
	return jsonOut && compat.V15()
}

// logsStreamMode reports whether this run is a v1.5 `logs --json --follow` stream.
func logsStreamMode(cmd *cobra.Command) bool {
	jsonOut, _ := cmd.Flags().GetBool("json")
	follow, _ := cmd.Flags().GetBool("follow")
	return follow && logsEnvelopeMode(jsonOut)
}

// runLogsEnvelope prints the v1.5 JSON form of `logs`: one envelope, or the
// stream when opts.Follow is set.
func runLogsEnvelope(ctx context.Context, workdir string, composeArgs []string, opts LogsOptions) error {
	w := pilotWriter()
	if opts.Follow {
		count := 0
		err := streamLogLines(ctx, workdir, composeArgs, opts, func(line string) error {
			count++
			return output.StreamRecord(w.Out, "logs", logRecordFromLine(line))
		})
		if err != nil {
			return err
		}
		return output.StreamEnd(w.Out, "logs", LogsEnd{Lines: count}, nil)
	}
	res := LogsDocument{Records: []LogRecord{}}
	err := streamLogLines(ctx, workdir, composeArgs, opts, func(line string) error {
		res.Records = append(res.Records, logRecordFromLine(line))
		return nil
	})
	if err != nil {
		return err
	}
	return output.EmitData(w, "logs", res)
}

// logsStreamReported marks a stream failure that was already written as the
// last (error envelope) line, so main prints no second document.
type logsStreamReported struct{ cause error }

func (e logsStreamReported) Error() string { return e.cause.Error() }
func (e logsStreamReported) Unwrap() error { return e.cause }

// Silent tells main the error envelope is already on stdout.
func (logsStreamReported) Silent() bool { return true }

// closeLogsStream turns a failure of a v1.5 `logs --json --follow` run into the
// compact error envelope as the stream's last line. Any other call, and a nil
// err, pass through unchanged.
func closeLogsStream(cmd *cobra.Command, err error) error {
	if err == nil || !logsStreamMode(cmd) {
		return err
	}
	if werr := output.StreamEnd(pilotWriter().Out, "logs", nil, err); werr != nil {
		return err
	}
	return logsStreamReported{cause: err}
}
