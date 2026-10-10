package invoke

// Purpose: Invoke is the whole path from a machine request to a JSON document:
// lookup, exposure, argv, self-exec, single-document check.
// Inputs: the machine registry, a command path, a Request, Options.
// Outputs: a Result whose Stdout is the child's document (byte for byte) or a
// v1 error envelope (E420-E423, E432) written through internal/output.
// Constraints: a refusal is a Result, not a Go error; the error return is for
// a cancelled context only. stderr never leaves this package except as the
// scrubbed 2 KiB cause of an E422. Confirm is validated and ignored here: the
// gate that consumes it is P7-SURF-25.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/observability"
	"github.com/nself-org/cli/internal/output"
)

// Invoke runs the registry command at path for req.
func Invoke(ctx context.Context, reg *cmdregistry.Registry, path string, req Request, o Options) (Result, error) {
	transport := o.Transport
	if transport == "" {
		transport = TransportMCP
	}
	command := barePath(path)
	var cmd *cmdregistry.Command
	if reg != nil {
		cmd, _ = reg.Lookup(command)
	}
	res := Result{RequestID: RequestID(path, cmd, req)}
	if cmd == nil {
		if r := []rune(command); len(r) > 96 {
			command = string(r[:96]) // the envelope never echoes an unbounded caller string
		}
		return refuse(res, command, errs.Newf("E432", "no command %s", quoteName(command)).
			WithWhy("The registry has no command at this path.").
			WithFix("List the commands with nself help capabilities and use one of those paths.")), nil
	}
	if ok, reason := Exposure(cmd, transport, SetFlags(req)); !ok {
		return refuse(res, command, errs.New("E421", "command "+quoteName(command)+" is not exposed").WithWhy(reason)), nil
	}
	argv, err := BuildArgvFor(cmd, req, transport)
	if err != nil {
		return refuse(res, command, err), nil
	}
	stream := transport == TransportHTTPStream
	spec := Spec{Argv: argv, Dir: o.Dir, Transport: transport, Timeout: o.Timeout}
	switch {
	case stream:
		spec.Timeout, spec.Stdout = 0, o.Stdout
	case spec.Timeout <= 0:
		spec.Timeout = DefaultTimeout
	}
	ran, err := Exec(ctx, spec)
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	secrets := SecretValues(cmd, req)
	if err != nil {
		return refuse(res, command, errs.New("E422", "could not run the nself child").
			WithWhy(scrub(err.Error(), secrets))), nil
	}
	if ran.TimedOut {
		return refuse(res, command, errs.Newf("E423", "%s did not finish within %s", quoteName(command), spec.Timeout).
			WithWhy("The command was stopped after the invocation timeout.")), nil
	}
	res.Stdout, res.ExitCode = ran.Stdout, ran.ExitCode
	if stream {
		return res, nil
	}
	if why := notOneDocument(ran); why != "" {
		tail := observability.Redact(lastBytes(scrub(string(ran.Stderr), secrets), CauseTailBytes))
		return refuse(res, command, errs.New("E422", fmt.Sprintf("%s produced no single JSON document (%s)", quoteName(command), why)).
			WithWhy(tail)), nil
	}
	return res, nil
}

// refuse writes err as the v1 error envelope for command into res.
func refuse(res Result, command string, err error) Result {
	var buf bytes.Buffer
	if werr := output.EmitError(output.Writer{Out: &buf, Err: io.Discard}, command, err); werr != nil {
		buf.Reset()
	}
	res.Stdout = buf.Bytes()
	res.ExitCode = errs.ExitCodeFor(err)
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		res.ErrorCode = ce.Code
	}
	return res
}

// notOneDocument returns why the child's stdout is not exactly one JSON object
// ("" when it is).
func notOneDocument(r Result) string {
	switch {
	case r.StdoutTruncated:
		return "stdout was larger than the output cap"
	case r.ExitCode < 0:
		return "the command was killed by a signal"
	}
	code := r.ExitCode
	body := bytes.TrimSpace(r.Stdout)
	if len(body) == 0 || body[0] != '{' {
		return fmt.Sprintf("exit %d, stdout is not a JSON object", code)
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	var doc json.RawMessage
	if err := dec.Decode(&doc); err != nil {
		return fmt.Sprintf("exit %d, stdout is not valid JSON", code)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fmt.Sprintf("exit %d, stdout holds more than one JSON value", code)
	}
	return ""
}

// scrub replaces every secret value (and its JSON and Go-quoted spellings) in s
// by Redacted. The caller keeps the tail afterwards, so a secret cut by the
// tail boundary was already gone.
func scrub(s string, secrets []string) string {
	for _, v := range secrets {
		forms := []string{v}
		if b, err := json.Marshal(v); err == nil && len(b) > 2 {
			forms = append(forms, string(b[1:len(b)-1]))
		}
		for _, f := range forms {
			s = strings.ReplaceAll(s, f, Redacted)
		}
	}
	return s
}

// lastBytes returns the last n bytes of s, starting on a rune boundary.
func lastBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[len(s)-n:]
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return s
}
