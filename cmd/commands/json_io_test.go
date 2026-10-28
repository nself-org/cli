package commands

// Tests for the io fragment (P7-SURF-27): `logs` prints one v1 envelope, or an
// NDJSON stream under --follow (contract:cli.json-stream v1), in v1.5 mode and
// keeps the pre-contract per-line JSON in v1.4 mode; `exec` is interactive and
// refuses --json (E402) in both modes.
//
// Constraints: docker is a PATH stub (a POSIX shell script, so the tests skip
// on Windows); the project and the stub live under t.TempDir; fixtures hold
// obviously fake lines only.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestEnvelopeCoverageIO(t *testing.T) {
	assertFragmentEnvelopeCoverage(t, "io")
	reg, err := buildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]int{}
	for i, c := range reg.Commands {
		rows[c.Path] = i
	}
	exec := reg.Commands[rows["nself exec"]]
	if exec.Output != canon.OutputInteractive || exec.JSON != canon.JSONNone {
		t.Fatalf("exec = output %s json %s, want interactive/none", exec.Output, exec.JSON)
	}
	logs := reg.Commands[rows["nself logs"]]
	if logs.Output != canon.OutputDocument || logs.JSON != canon.JSONEnvelope {
		t.Fatalf("logs = output %s json %s, want document/envelope", logs.Output, logs.JSON)
	}
	if got := jsonRecordTypes["logs"]; got == nil {
		t.Fatal("logs has no registered stream record type")
	}
	follow := ""
	for _, f := range logs.Flags {
		if f.Name == "follow" && f.Output != nil {
			follow = *f.Output
		}
	}
	if follow != canon.OutputStream {
		t.Fatalf("logs --follow output = %q, want stream", follow)
	}
	// v1.4 does not advertise an envelope (its per-line JSON is pre-contract).
	old, err := buildRegistry(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range old.Commands {
		if c.Path == "nself logs" && c.JSON != canon.JSONLegacy {
			t.Fatalf("v1.4 logs json = %s, want legacy", c.JSON)
		}
	}
}

// ioLogLines are the fake docker compose lines (service prefix kept).
var ioLogLines = []string{
	"web-1  | 2026-01-02T03:04:05Z [info] server started",
	"web-1  | 2026-01-02T03:04:06Z GET /health 200",
	"db-1  | 2026-01-02T03:04:07Z [error] connection refused",
	"db-1  | 2026-01-02T03:04:08Z [warn] slow query <b> & done",
	"web-1  | plain line without a timestamp",
}

// ioPlainLines are what v1.4 receives: it passes --no-log-prefix.
var ioPlainLines = []string{
	"2026-01-02T03:04:05Z [info] server started",
	"2026-01-02T03:04:06Z GET /health 200",
	"2026-01-02T03:04:07Z [error] connection refused",
	"2026-01-02T03:04:08Z [warn] slow query <b> & done",
	"plain line without a timestamp",
}

// ioLegacyLines is the pre-contract per-line JSON of ioPlainLines, written out
// by hand from the base implementation (keys sorted, HTML escaped).
var ioLegacyLines = []string{
	`{"level":"info","message":"[info] server started","timestamp":"2026-01-02T03:04:05Z"}`,
	`{"message":"GET /health 200","timestamp":"2026-01-02T03:04:06Z"}`,
	`{"level":"error","message":"[error] connection refused","timestamp":"2026-01-02T03:04:07Z"}`,
	`{"level":"warn","message":"[warn] slow query ` + "\\u003cb\\u003e \\u0026" + ` done","timestamp":"2026-01-02T03:04:08Z"}`,
	`{"message":"plain line without a timestamp"}`,
}

// ioProject creates a project dir (chdir'd into) and a docker stub that
// answers `compose ps -q` with a container id and `compose logs` with lines,
// then exits with logsExit.
func ioProject(t *testing.T, lines []string, containers bool, logsExit int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("docker stub is a POSIX shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=smoke\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lines.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ps := "exit 0"
	if containers {
		ps = "echo abc123; exit 0"
	}
	stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + filepath.Join(dir, "argv.txt") + "'\ncase \" $* \" in\n  *' ps -q '*) " + ps + " ;;\n  *' logs '*) cat '" +
		filepath.Join(dir, "lines.txt") + "'; exit " + itoa(logsExit) + " ;;\nesac\nexit 0\n"
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ioDir = dir
}

// ioDir is the project directory of the current test. The run helpers enter it
// only while the command runs, so relative paths to schemas and goldens keep
// working in the test body.
var ioDir string

// inIODir runs fn with the working directory set to the project.
func inIODir(t *testing.T, fn func()) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(ioDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatal(err)
		}
	}()
	fn()
}

func itoa(n int) string { return strconv.Itoa(n) }

// runIOLogs runs the real logs body with flags on a fresh command and returns
// the pilot stdout (v1.5 JSON) and the error.
func runIOLogs(t *testing.T, flags ...string) (string, error) {
	t.Helper()
	out, _ := pilotBuffers(t)
	c := &cobra.Command{Use: "logs", RunE: runLogs}
	addLogsFlags(c)
	c.SetContext(context.Background())
	c.SetOut(io.Discard)
	c.SetErr(io.Discard)
	if err := c.ParseFlags(flags); err != nil {
		t.Fatal(err)
	}
	var err error
	inIODir(t, func() { err = c.RunE(c, nil) })
	return out.String(), err
}

// runIOLogsLegacy runs logs with os.Stdout redirected, as v1.4 writes it.
func runIOLogsLegacy(t *testing.T, flags ...string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	c := &cobra.Command{Use: "logs", RunE: runLogs}
	addLogsFlags(c)
	c.SetContext(context.Background())
	c.SetOut(io.Discard)
	c.SetErr(io.Discard)
	if perr := c.ParseFlags(flags); perr != nil {
		os.Stdout = old
		t.Fatal(perr)
	}
	var runErr error
	inIODir(t, func() { runErr = c.RunE(c, nil) })
	os.Stdout = old
	_ = w.Close()
	return <-done, runErr
}

func TestLogsJSON(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		if !compat.V15() {
			ioProject(t, ioPlainLines, true, 0)
			out, err := runIOLogsLegacy(t, "--json", "--tail", "5")
			if err != nil {
				t.Fatal(err)
			}
			if want := strings.Join(ioLegacyLines, "\n") + "\n"; out != want {
				t.Fatalf("v1.4 logs --json changed\n--- got ---\n%s--- want ---\n%s", out, want)
			}
			return
		}
		ioProject(t, ioLogLines, true, 0)
		out, err := runIOLogs(t, "--json", "--tail", "5")
		if err != nil {
			t.Fatal(err)
		}
		if want := goldenFor(t, "logs"); out != want {
			t.Fatalf("logs --json differs from testdata/json/logs.golden.json\n--- got ---\n%s--- want ---\n%s", out, want)
		}
		data := validateEnvelope(t, out, "logs")
		recs, _ := data["records"].([]any)
		if len(recs) != 5 {
			t.Fatalf("records = %d, want 5", len(recs))
		}
		rs := contractResolve(t, "commands/logs.record.v1.schema.json")
		for i, r := range recs {
			raw, _ := json.Marshal(r)
			if err := contractValidate(rs, raw); err != nil {
				t.Fatalf("record %d does not validate against the record schema: %v\n%s", i, err, raw)
			}
		}
	})
}

func TestLogsJSONEmptyIsAnEmptyList(t *testing.T) {
	compattest.Set(t, true)
	ioProject(t, nil, true, 0)
	out, err := runIOLogs(t, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if data := validateEnvelope(t, out, "logs"); data["records"] == nil {
		t.Fatalf("records is null, want []: %s", out)
	}
}

// streamLines splits an NDJSON stream and checks each line is one compact
// envelope that validates against envelope.v1.
func streamLines(t *testing.T, out string) []map[string]any {
	t.Helper()
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("stream does not end with a newline: %q", out)
	}
	env := contractResolve(t, "envelope.v1.schema.json")
	var docs []map[string]any
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), maxLogLineBytes+4096)
	for sc.Scan() {
		line := sc.Text()
		var compact bytes.Buffer
		if err := json.Compact(&compact, []byte(line)); err != nil || compact.String() != line {
			t.Fatalf("line is not compact JSON: %q (%v)", line, err)
		}
		if err := contractValidate(env, []byte(line)); err != nil {
			t.Fatalf("line does not validate against envelope.v1: %v\n%s", err, line)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(line), &doc); err != nil {
			t.Fatal(err)
		}
		docs = append(docs, doc)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading stream: %v", err)
	}
	return docs
}

func TestLogsJSONStream(t *testing.T) {
	compattest.Set(t, true)
	ioProject(t, ioLogLines, true, 0)
	out, err := runIOLogs(t, "--json", "--follow", "--tail", "5")
	if err != nil {
		t.Fatal(err)
	}
	if want := goldenFor(t, "logs-follow"); out != want {
		t.Fatalf("logs --json --follow differs from testdata/json/logs-follow.golden.json\n--- got ---\n%s--- want ---\n%s", out, want)
	}
	docs := streamLines(t, out)
	if len(docs) != 6 {
		t.Fatalf("lines = %d, want 5 records and 1 result", len(docs))
	}
	rs := contractResolve(t, "commands/logs.record.v1.schema.json")
	for i, d := range docs[:5] {
		data, _ := d["data"].(map[string]any)
		if d["command"] != "logs" || data["type"] != "log" {
			t.Fatalf("line %d = %v, want a log record", i, d)
		}
		raw, _ := json.Marshal(data)
		if err := contractValidate(rs, raw); err != nil {
			t.Fatalf("line %d data does not validate against the record schema: %v\n%s", i, err, raw)
		}
	}
	last, _ := docs[5]["data"].(map[string]any)
	if last["type"] != "result" || last["lines"] != float64(5) {
		t.Fatalf("last line data = %v, want type result and lines 5", last)
	}
	// Records carry the html-looking text unescaped.
	if !strings.Contains(out, "slow query <b> & done") {
		t.Fatalf("record message was HTML-escaped: %s", out)
	}
}

func TestLogsJSONStreamFailuresEndWithAnErrorLine(t *testing.T) {
	compattest.Set(t, true)
	t.Run("no containers", func(t *testing.T) {
		ioProject(t, ioLogLines, false, 0)
		out, err := runIOLogs(t, "--json", "--follow")
		if err == nil {
			t.Fatal("want an error")
		}
		var s errs.Silencer
		if !errors.As(err, &s) || !s.Silent() {
			t.Fatalf("error is not silent, main would print a second document: %v", err)
		}
		docs := streamLines(t, out)
		if len(docs) != 1 || docs[0]["error"] == nil || docs[0]["data"] != nil {
			t.Fatalf("want exactly one error envelope, got %s", out)
		}
	})
	t.Run("docker fails mid stream", func(t *testing.T) {
		ioProject(t, ioLogLines[:2], true, 3)
		out, err := runIOLogs(t, "--json", "--follow")
		if err == nil {
			t.Fatal("want an error")
		}
		docs := streamLines(t, out)
		if len(docs) != 3 || docs[2]["error"] == nil {
			t.Fatalf("want 2 records then an error envelope, got %s", out)
		}
		for _, d := range docs[:2] {
			if data, _ := d["data"].(map[string]any); data["type"] != "log" {
				t.Fatalf("record line = %v", d)
			}
		}
	})
	t.Run("no containers without follow is a plain error", func(t *testing.T) {
		ioProject(t, ioLogLines, false, 0)
		out, err := runIOLogs(t, "--json")
		if err == nil || out != "" {
			t.Fatalf("err=%v stdout=%q: main writes the one error envelope", err, out)
		}
	})
}

// withRealFlags resolves a real command, parses flags (no body runs), calls fn
// and restores the flag state.
func withRealFlags(t *testing.T, path []string, flags []string, fn func(*cobra.Command)) {
	t.Helper()
	reattachRealTree()
	cmd, _, err := RootCmd.Find(path)
	if err != nil || cmd == nil || cmd == RootCmd {
		t.Fatalf("real command %v not found: %v", path, err)
	}
	if err := cmd.ParseFlags(flags); err != nil {
		t.Fatalf("%s: %v", cmd.CommandPath(), err)
	}
	defer cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Changed {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	})
	fn(cmd)
}

// exec is interactive (json none) and logs reports have no JSON: --json is
// refused with E402 in both modes for exec, and for logs --status/--summary/--top
// in v1.5 (v1.4 keeps accepting and ignoring it).
func TestExecAndLogsReportsRefuseJSON(t *testing.T) {
	resetRegistryCache()
	t.Cleanup(resetRegistryCache)
	compattest.Both(t, func(t *testing.T) {
		defer prepareTreeWith(&canonTable, RootCmd, compat.V15(), false)()
		withRealFlags(t, []string{"exec"}, []string{"--json"}, func(cmd *cobra.Command) {
			wantE402(t, refuseUnsupportedJSON(cmd))
		})
		for _, flag := range []string{"--status", "--summary", "--top"} {
			withRealFlags(t, []string{"logs"}, []string{"--json", flag}, func(cmd *cobra.Command) {
				err := refuseUnsupportedJSON(cmd)
				if compat.V15() {
					wantE402(t, err)
				} else if err != nil {
					t.Fatalf("v1.4 logs --json %s refused: %v", flag, err)
				}
			})
		}
		withRealFlags(t, []string{"logs"}, []string{"--json", "--follow"}, func(cmd *cobra.Command) {
			if err := refuseUnsupportedJSON(cmd); err != nil {
				t.Fatalf("logs --json --follow refused: %v", err)
			}
		})
	})
}

// A write failure on stdout stops the stream and is returned, not swallowed.
func TestLogsJSONStreamWriteFailure(t *testing.T) {
	compattest.Set(t, true)
	ioProject(t, ioLogLines, true, 0)
	old := pilotWriter
	pilotWriter = func() output.Writer { return output.Writer{Out: ioFailWriter{}, Err: io.Discard} }
	t.Cleanup(func() { pilotWriter = old; output.ResetState() })
	c := &cobra.Command{Use: "logs", RunE: runLogs}
	addLogsFlags(c)
	c.SetContext(context.Background())
	if err := c.ParseFlags([]string{"--json", "--follow"}); err != nil {
		t.Fatal(err)
	}
	var err error
	inIODir(t, func() { err = c.RunE(c, nil) })
	if err == nil {
		t.Fatal("write failure was swallowed")
	}
}

type ioFailWriter struct{}

func (ioFailWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

// TestLogsJSONStreamLongLine pins that a log line over the default 64 KiB
// scanner buffer becomes a record and the stream still ends with a result.
func TestLogsJSONStreamLongLine(t *testing.T) {
	compattest.Set(t, true)
	long := "web-1  | " + strings.Repeat("x", 100*1024)
	ioProject(t, []string{long, "web-1  | after"}, true, 0)
	out, err := runIOLogs(t, "--json", "--follow")
	if err != nil {
		t.Fatal(err)
	}
	docs := streamLines(t, out)
	if len(docs) != 3 {
		t.Fatalf("lines = %d, want 2 records and 1 result", len(docs))
	}
	if last, _ := docs[2]["data"].(map[string]any); last["type"] != "result" || last["lines"] != float64(2) {
		t.Fatalf("last line data = %v, want type result and lines 2", last)
	}
}

func TestLogsRecordPreservesIndentation(t *testing.T) {
	for _, line := range []string{"web-1  |   indented  ", "web-1  | 2026-01-02T03:04:05Z   indented  "} {
		if got := logRecordFromLine(line).Message; got != "  indented  " {
			t.Fatalf("message = %q", got)
		}
	}
}

func TestLogsJSONFollowLegacyBytes(t *testing.T) {
	compattest.Set(t, false)
	ioProject(t, ioLogLines, true, 0)
	out, err := runIOLogsLegacy(t, "--json", "--follow")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		`{"level":"info","message":"web-1  | 2026-01-02T03:04:05Z [info] server started","service":"web-1  |"}`,
		`{"message":"web-1  | 2026-01-02T03:04:06Z GET /health 200","service":"web-1  |"}`,
		`{"level":"error","message":"db-1  | 2026-01-02T03:04:07Z [error] connection refused","service":"db-1  |"}`,
		`{"level":"warn","message":"db-1  | 2026-01-02T03:04:08Z [warn] slow query \u003cb\u003e \u0026 done","service":"db-1  |"}`,
		`{"message":"web-1  | plain line without a timestamp","service":"web-1  |"}`,
	}, "\n") + "\n"
	if out != want {
		t.Fatalf("legacy follow bytes changed: got %s want %s", out, want)
	}
}

func TestLogsJSONArgv(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		for _, follow := range []bool{false, true} {
			t.Run(strconv.FormatBool(follow), func(t *testing.T) {
				ioProject(t, nil, true, 0)
				flags := []string{"--json", "--tail", "5"}
				want := []string{"compose", "logs"}
				if follow {
					flags = append(flags, "--follow")
					want = append(want, "--follow")
				}
				want = append(want, "--tail", "5")
				if !follow && !compat.V15() {
					want = append(want, "--no-log-prefix")
				}
				if compat.V15() {
					want = append(want, "--no-color")
				}
				var err error
				if compat.V15() {
					_, err = runIOLogs(t, flags...)
				} else {
					_, err = runIOLogsLegacy(t, flags...)
				}
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(filepath.Join(ioDir, "argv.txt"))
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != strings.Join(want, "\n")+"\n" {
					t.Fatalf("argv = %s want %v", got, want)
				}
			})
		}
	})
}

func TestLogsJSONOverlongLine(t *testing.T) {
	compattest.Set(t, true)
	ioProject(t, []string{strings.Repeat("x", maxLogLineBytes+1)}, true, 0)
	out, err := runIOLogs(t, "--json", "--follow")
	if err == nil {
		t.Fatal("overlong line must fail")
	}
	docs := streamLines(t, out)
	if len(docs) != 1 || docs[0]["error"] == nil {
		t.Fatalf("want one terminal error: %s", out)
	}
}

func TestLogsJSONFiltersIgnoreServicePrefix(t *testing.T) {
	compattest.Set(t, true)
	for _, flag := range []string{"--search", "--grep", "--level"} {
		t.Run(flag, func(t *testing.T) {
			ioProject(t, []string{"error-service  | [info] ordinary message"}, true, 0)
			value := "error"
			if flag == "--grep" {
				value = "^error"
			}
			out, err := runIOLogs(t, "--json", flag, value)
			if err != nil {
				t.Fatal(err)
			}
			data := validateEnvelope(t, out, "logs")
			if len(data["records"].([]any)) != 0 {
				t.Fatalf("prefix affected filter: %s", out)
			}
		})
	}
}
