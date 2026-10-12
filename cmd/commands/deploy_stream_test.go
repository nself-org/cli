package commands

// deploy_stream_test.go: `--stream` for deploy and deploy promote
// (contract:cli.deploy-progress v1). Fakes only: no network, no docker daemon.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat/compattest"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	dprog "github.com/nself-org/cli/internal/deploy/progress"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/schemas"
	"github.com/spf13/cobra"
)

// streamDest swaps the output seam for a buffer and returns it.
func streamDest(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := pilotWriter
	pilotWriter = func() output.Writer { return output.Writer{Out: &buf, Err: &bytes.Buffer{}} }
	t.Cleanup(func() { pilotWriter = old; output.ResetState() })
	return &buf
}

// captureStd runs fn with os.Stdout and os.Stderr on pipes and returns both.
func captureStd(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = outW, errW
	read := func(r *os.File) chan string {
		ch := make(chan string, 1)
		go func() { b, _ := io.ReadAll(r); ch <- string(b) }()
		return ch
	}
	outCh, errCh := read(outR), read(errR)
	func() {
		defer func() {
			os.Stdout, os.Stderr = oldOut, oldErr
			_ = outW.Close()
			_ = errW.Close()
		}()
		fn()
	}()
	return <-outCh, <-errCh
}

// resolvedSchema loads a committed schema; $ref targets resolve by $id through
// schemas/index.json.
func resolvedSchema(t *testing.T, file string) *jsonschema.Resolved {
	t.Helper()
	read := func(f string) *jsonschema.Schema {
		b, ok := schemas.Lookup(f)
		if !ok {
			t.Fatalf("schemas/%s is missing (run make schemas)", f)
		}
		var s jsonschema.Schema
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		return &s
	}
	var idx struct{ Schemas []struct{ Path, ID string } }
	raw, _ := schemas.Lookup("index.json")
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatal(err)
	}
	loader := func(u *url.URL) (*jsonschema.Schema, error) {
		for _, e := range idx.Schemas {
			if e.ID == u.String() {
				return read(e.Path), nil
			}
		}
		return nil, fmt.Errorf("no schema with id %s", u)
	}
	rs, err := read(file).Resolve(&jsonschema.ResolveOptions{Loader: loader})
	if err != nil {
		t.Fatalf("resolve %s: %v", file, err)
	}
	return rs
}

// checkStream asserts the stream in buf obeys the contract and returns the
// events and the result data. It requires at least the result line.
func checkStream(t *testing.T, buf string, command, wantResult string) ([]dprog.Event, dprog.Result) {
	t.Helper()
	envelope, event := resolvedSchema(t, "envelope.v1.schema.json"), resolvedSchema(t, "deploy-progress.v1.schema.json")
	if buf == "" || !strings.HasSuffix(buf, "\n") {
		t.Fatalf("stream must end with a newline: %q", buf)
	}
	lines := strings.Split(strings.TrimSuffix(buf, "\n"), "\n")
	var events []dprog.Event
	var res dprog.Result
	for i, line := range lines {
		var doc map[string]any
		if err := json.Unmarshal([]byte(line), &doc); err != nil {
			t.Fatalf("line %d is not JSON (%v): %q", i+1, err, line)
		}
		if err := envelope.Validate(doc); err != nil {
			t.Fatalf("line %d is not a v1 envelope: %v: %s", i+1, err, line)
		}
		var compact bytes.Buffer
		_ = json.Compact(&compact, []byte(line))
		if compact.String() != line || doc["command"] != command {
			t.Fatalf("line %d is not compact or names command %v: %s", i+1, doc["command"], line)
		}
		data, _ := doc["data"].(map[string]any)
		if i == len(lines)-1 {
			keys := make([]string, 0, len(data))
			for k := range data {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			if strings.Join(keys, ",") != "result,steps,type" || data["type"] != "result" {
				t.Fatalf("last line must be the result data with exactly type, result, steps: %s", line)
			}
			b, _ := json.Marshal(data)
			_ = json.Unmarshal(b, &res)
			continue
		}
		if err := event.Validate(data); err != nil {
			t.Fatalf("line %d data does not validate against deploy-progress.v1: %v: %s", i+1, err, line)
		}
		var ev dprog.Event
		b, _ := json.Marshal(data)
		_ = json.Unmarshal(b, &ev)
		if ev.Seq != i+1 {
			t.Fatalf("line %d has seq %d, want %d", i+1, ev.Seq, i+1)
		}
		events = append(events, ev)
	}
	if res.Result != wantResult {
		t.Fatalf("data.result = %q, want %q", res.Result, wantResult)
	}
	return events, res
}

func phases(events []dprog.Event, step string) string {
	var out []string
	for _, e := range events {
		if e.Step == step {
			out = append(out, e.Phase)
		}
	}
	return strings.Join(out, ",")
}

func stepStatuses(res dprog.Result) string {
	var out []string
	for _, s := range res.Steps {
		out = append(out, s.Step+"="+s.Status)
	}
	return strings.Join(out, ";")
}

func isSilent(err error) bool {
	var s errs.Silencer
	return errors.As(err, &s) && s.Silent()
}

// TestDeployProgressEnvelopeLines: a sim deploy in stream mode prints only v1
// envelopes on the stream, every progress line validates against the schema with
// seq 1..n, and the last line carries the outcome, also when the run failed.
func TestDeployProgressEnvelopeLines(t *testing.T) {
	flags := func(extra ...string) map[string]string {
		m := map[string]string{"json": "true", "stream": "true"}
		for _, e := range extra {
			m[e] = "true"
		}
		return m
	}
	t.Run("pipeline dry run", func(t *testing.T) {
		scopeFixture(t, false)
		dest := streamDest(t)
		var err error
		stdout, stderr := captureStd(t, func() { err = runDeployArgs(t, flags("dry-run"), "qa") })
		if err != nil || stdout != "" {
			t.Fatalf("err=%v stdout=%q, want success and nothing on stdout (stderr %q)", err, stdout, stderr)
		}
		events, res := checkStream(t, dest.String(), "deploy", "ok")
		if len(events) != 1 || events[0].Step != "Deploy qa-app" || events[0].Phase != "skipped" ||
			events[0].Env == nil || *events[0].Env != "qa" || events[0].Server == nil || *events[0].Server != "qa-app" {
			t.Fatalf("events = %+v", events)
		}
		if stepStatuses(res) != "Deploy qa-app=planned" {
			t.Errorf("steps = %s", stepStatuses(res))
		}
	})
	t.Run("local dry run", func(t *testing.T) {
		scopeFixture(t, false)
		dest := streamDest(t)
		var err error
		stdout, stderr := captureStd(t, func() { err = runDeployArgs(t, flags("dry-run"), "local") })
		if err != nil || stdout != "" {
			t.Fatalf("err=%v stdout=%q (stderr %q)", err, stdout, stderr)
		}
		events, res := checkStream(t, dest.String(), "deploy", "ok")
		if phases(events, "Build images") != "skipped" || phases(events, "Start local stack") != "skipped" {
			t.Errorf("events = %+v", events)
		}
		if stepStatuses(res) != "Build images=pending;Start local stack=pending" {
			t.Errorf("steps = %s", stepStatuses(res))
		}
		if !strings.Contains(stderr, "finished in") {
			t.Errorf("the human summary belongs on stderr: %q", stderr)
		}
	})
	t.Run("failed push ends with a result line and keeps the exit class", func(t *testing.T) {
		t.Chdir(t.TempDir())
		isolateDeployEnv(t)
		t.Setenv("NSELF_DEPLOY_HOST_QA", "deploy@qa.example.test:/opt/nself")
		ob, op := deployBuildStepFn, remoteDeployPushFn
		deployBuildStepFn = func(_ context.Context, _ string, _ bool, s []deployStep) ([]deployStep, error) { return s, nil }
		remoteDeployPushFn = func(context.Context, string, string, string, bool) error { return errors.New("stub push failed") }
		t.Cleanup(func() { deployBuildStepFn, remoteDeployPushFn = ob, op })
		dest := streamDest(t)
		var err error
		stdout, stderr := captureStd(t, func() { err = runDeployArgs(t, flags(), "qa") })
		if stdout != "" {
			t.Fatalf("stdout must stay empty, got %q", stdout)
		}
		if err == nil || !isSilent(err) || !strings.Contains(err.Error(), "stub push failed") || errs.ExitCodeFor(err) != 1 {
			t.Fatalf("err = %v (silent %v, exit %d), want the push error, silent, exit 1", err, isSilent(err), errs.ExitCodeFor(err))
		}
		if !strings.Contains(stderr, "stub push failed") {
			t.Errorf("the failure text belongs on stderr: %q", stderr)
		}
		events, res := checkStream(t, dest.String(), "deploy", "failed")
		push := "Push artefacts to deploy@qa.example.test:/opt/nself"
		if phases(events, "Build images") != "start,done" || phases(events, push) != "start,failed" {
			t.Errorf("events = %+v", events)
		}
		if stepStatuses(res) != "Build images=done;"+push+"=failed" {
			t.Errorf("steps = %s", stepStatuses(res))
		}
		if events[0].Env == nil || *events[0].Env != "qa" || events[0].Server != nil {
			t.Errorf("events should carry env qa and no server: %+v", events[0])
		}
	})
}

// TestDeployStreamRollingEvents: the rolling restart reports each service as
// start then done, or start then failed, through the stream.
func TestDeployStreamRollingEvents(t *testing.T) {
	run := func(t *testing.T, s *rollingStack) (events []dprog.Event, res dprog.Result, err error) {
		t.Helper()
		var buf bytes.Buffer
		w := dprog.NewWriter(&buf, "deploy")
		_, err = runRollingRestart(dprog.WithWriter(context.Background(), w), s.dir, true)
		_ = w.Finish(map[bool]string{true: "ok", false: "failed"}[err == nil], nil)
		events, res = checkStream(t, buf.String(), "deploy", map[bool]string{true: "ok", false: "failed"}[err == nil])
		return events, res, err
	}
	t.Run("healthy stack", func(t *testing.T) {
		s := newRollingStack(t, []string{"postgres", "web"}, stackConfig)
		s.setPs(t, "postgres", `[{"Name":"p-pg-1","State":"running","Health":"healthy"}]`)
		s.setPs(t, "web", psRunning)
		events, res, err := run(t, s)
		if err != nil {
			t.Fatal(err)
		}
		if phases(events, "Restart postgres") != "start,done" || phases(events, "Restart web") != "start,done" {
			t.Errorf("events = %+v", events)
		}
		if stepStatuses(res) != "Restart postgres=done;Restart web=done" {
			t.Errorf("steps = %s", stepStatuses(res))
		}
	})
	t.Run("a one-shot that exits 1", func(t *testing.T) {
		s := newRollingStack(t, []string{"postgres", "init", "web"}, stackConfig)
		s.setPs(t, "postgres", `[{"Name":"p-pg-1","State":"running","Health":"healthy"}]`)
		s.setPs(t, "init", `[{"ID":"c1","Name":"p-init-1","State":"exited","ExitCode":1}]`)
		s.setPs(t, "web", psRunning)
		events, res, err := run(t, s)
		if err == nil {
			t.Fatal("want the gate failure")
		}
		if phases(events, "Restart init") != "start,failed" || phases(events, "Restart web") != "" {
			t.Errorf("events = %+v", events)
		}
		if stepStatuses(res) != "Restart postgres=done;Restart init=failed" {
			t.Errorf("steps = %s", stepStatuses(res))
		}
	})
}

// streamPromoteCmd is promoteTestCommand with the stream flag.
func streamPromoteCmd(out *bytes.Buffer, dry, jsonOut, stream bool) *cobra.Command {
	c := promoteTestCommand(out, dry, true, jsonOut)
	c.Flags().Bool("stream", stream, "")
	c.SetContext(context.Background())
	return c
}

// TestDeployStreamRequiresJSON: --stream without --json exits 1 with E401 and
// does nothing; deploy promote --stream also needs <source> <target>.
func TestDeployStreamRequiresJSON(t *testing.T) {
	wantE401 := func(t *testing.T, err error, contains string) {
		t.Helper()
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E401" || errs.ExitCodeFor(err) != 1 || !strings.Contains(err.Error(), contains) {
			t.Fatalf("err = %v, want E401 / exit 1 containing %q", err, contains)
		}
	}
	t.Run("deploy", func(t *testing.T) {
		scopeFixture(t, false)
		dest := streamDest(t)
		var err error
		stdout, _ := captureStd(t, func() { err = runDeployArgs(t, map[string]string{"stream": "true", "dry-run": "true"}, "qa") })
		wantE401(t, err, "--stream requires --json")
		if stdout != "" || dest.Len() != 0 || len(scopeBuilds) != 0 {
			t.Errorf("nothing may run or print: stdout=%q stream=%q builds=%v", stdout, dest.String(), scopeBuilds)
		}
	})
	t.Run("promote", func(t *testing.T) {
		compattest.Set(t, true)
		promoteFixture(t)
		dest := streamDest(t)
		wantE401(t, runDeployPromote(streamPromoteCmd(&bytes.Buffer{}, true, false, true), []string{"ls", "prodsim"}), "--stream requires --json")
		wantE401(t, runDeployPromote(streamPromoteCmd(&bytes.Buffer{}, true, true, true), nil), "<source> <target>")
		wantE401(t, runDeployPromote(streamPromoteCmd(&bytes.Buffer{}, true, true, true), []string{"only-one"}), "<source> <target>")
		if dest.Len() != 0 {
			t.Errorf("a refused run printed %q", dest.String())
		}
	})
	t.Run("promote in v1.4", func(t *testing.T) {
		compattest.Set(t, false)
		promoteFixture(t)
		wantE401(t, runDeployPromote(streamPromoteCmd(&bytes.Buffer{}, true, true, true), []string{"ls", "prodsim"}), "<source> <target>")
	})
}

// fakePromoteEngine returns an engine whose health step fails once, then
// passes for the post-rollback re-check. Snapshot and apply can be undone.
func fakePromoteEngine(undone *[]string) func(*cobra.Command, string, string, string, bool, bool) (cpromote.Engine, *cpromote.State, error) {
	return func(_ *cobra.Command, _, source, target string, dry, _ bool) (cpromote.Engine, *cpromote.State, error) {
		st := &cpromote.State{Source: source, Target: target, DryRun: dry, Values: map[string]any{}}
		healthRuns := 0
		var steps []cpromote.Step
		for _, name := range cpromote.StepNames {
			step := cpromote.Step{Name: name, Run: func(context.Context, *cpromote.State) error { return nil }}
			switch name {
			case "snapshot", "apply":
				step.Undo = func(context.Context, *cpromote.State) error { *undone = append(*undone, name); return nil }
			case "health":
				step.Run = func(context.Context, *cpromote.State) error {
					healthRuns++
					if healthRuns == 1 {
						return errors.New("injected health failure")
					}
					return nil
				}
			}
			steps = append(steps, step)
		}
		return cpromote.Engine{Steps: steps}, st, nil
	}
}

// TestPromoteStreamRollback: a health failure after apply is rolled back; the
// stream shows rollback events, ends with data.result rolled_back, and the exit
// status is the promote E488 class (2).
func TestPromoteStreamRollback(t *testing.T) {
	compattest.Set(t, true)
	promoteFixture(t)
	dest := streamDest(t)
	var undone []string
	old := promoteEngineFn
	promoteEngineFn = fakePromoteEngine(&undone)
	t.Cleanup(func() { promoteEngineFn = old })
	var err error
	stdout, stderr := captureStd(t, func() {
		err = runDeployPromote(streamPromoteCmd(&bytes.Buffer{}, false, true, true), []string{"ls", "prodsim"})
	})
	if stdout != "" {
		t.Fatalf("stdout must stay empty, got %q", stdout)
	}
	var ce *errs.CLIError
	if err == nil || !isSilent(err) || !errors.As(err, &ce) || ce.Code != "E488" || errs.ExitCodeFor(err) != 2 {
		t.Fatalf("err = %v (silent %v), want E488 / exit 2", err, isSilent(err))
	}
	if !strings.Contains(stderr, "E488") || !strings.Contains(stderr, "health") {
		t.Errorf("the failure text belongs on stderr: %q", stderr)
	}
	events, res := checkStream(t, dest.String(), "deploy promote", "rolled_back")
	if phases(events, "health") != "start,failed,rollback" || phases(events, "apply") != "start,done,rollback" {
		t.Errorf("health/apply phases = %q / %q", phases(events, "health"), phases(events, "apply"))
	}
	rollbacks := 0
	for _, e := range events {
		if e.Phase == "rollback" {
			rollbacks++
		}
		if e.Env == nil || *e.Env != "prodsim" || e.Server != nil {
			t.Fatalf("promote events carry the target env and no server: %+v", e)
		}
	}
	if rollbacks < 3 || strings.Join(undone, ",") != "apply,snapshot" {
		t.Errorf("rollback events = %d, undone = %v", rollbacks, undone)
	}
	statuses := map[string]string{}
	for _, s := range res.Steps {
		statuses[s.Step] = s.Status
	}
	if statuses["health"] != "failed" || statuses["apply"] != "rolled_back" || statuses["resolve"] != "ok" {
		t.Errorf("result steps = %v", statuses)
	}
	// A clean promote (no failure) ends ok and exits 0.
	dest.Reset()
	undone = nil
	promoteEngineFn = func(c *cobra.Command, w, s, tg string, d, j bool) (cpromote.Engine, *cpromote.State, error) {
		eng, st, err := fakePromoteEngine(&undone)(c, w, s, tg, d, j)
		for i := range eng.Steps {
			if eng.Steps[i].Name == "health" {
				eng.Steps[i].Run = func(context.Context, *cpromote.State) error { return nil }
			}
		}
		return eng, st, err
	}
	var okErr error
	stdout, _ = captureStd(t, func() {
		okErr = runDeployPromote(streamPromoteCmd(&bytes.Buffer{}, false, true, true), []string{"ls", "prodsim"})
	})
	if okErr != nil || stdout != "" {
		t.Fatalf("clean promote: err=%v stdout=%q", okErr, stdout)
	}
	events, _ = checkStream(t, dest.String(), "deploy promote", "ok")
	for _, e := range events {
		if e.Phase == "rollback" || e.Phase == "failed" {
			t.Errorf("clean promote has event %+v", e)
		}
	}
}

// TestPromoteStreamDryRun: the plan is reported as skipped events and the
// stream ends ok.
func TestPromoteStreamDryRun(t *testing.T) {
	compattest.Set(t, true)
	promoteFixture(t)
	dest := streamDest(t)
	var err error
	stdout, _ := captureStd(t, func() {
		err = runDeployPromote(streamPromoteCmd(&bytes.Buffer{}, true, true, true), []string{"ls", "prodsim"})
	})
	if err != nil || stdout != "" {
		t.Fatalf("err=%v stdout=%q", err, stdout)
	}
	events, res := checkStream(t, dest.String(), "deploy promote", "ok")
	if len(events) != len(cpromote.StepNames) || len(res.Steps) != len(cpromote.StepNames) {
		t.Fatalf("events=%d steps=%d, want %d", len(events), len(res.Steps), len(cpromote.StepNames))
	}
	for _, e := range events {
		if e.Phase != "skipped" {
			t.Errorf("dry-run event %+v should be skipped", e)
		}
	}
}

var durationRe = regexp.MustCompile(`"durationMs": \d+`)

const goldenPipelineDryRun = `{
  "dry_run": true,
  "topology": [
    {
      "env": "qa",
      "server": "qa-app",
      "capability": "manage"
    }
  ]
}
`

const goldenLocalDryRun = `{
  "target": "local",
  "strategy": "rolling",
  "steps": [
    {
      "name": "Build images",
      "status": "pending"
    },
    {
      "name": "Start local stack",
      "status": "pending"
    }
  ],
  "durationMs": 0,
  "success": true
}
`

// TestDeployJSONUnchanged: `deploy --json` without --stream prints the same
// bytes as before this feature (goldens captured from the base code) and nothing
// goes to the stream writer.
func TestDeployJSONUnchanged(t *testing.T) {
	for _, tc := range []struct{ name, target, golden string }{
		{"pipeline dry run", "qa", goldenPipelineDryRun},
		{"legacy local dry run", "local", goldenLocalDryRun},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scopeFixture(t, false)
			dest := streamDest(t)
			var err error
			stdout, _ := captureStd(t, func() {
				err = runDeployArgs(t, map[string]string{"json": "true", "dry-run": "true"}, tc.target)
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := durationRe.ReplaceAllString(stdout, `"durationMs": 0`); got != tc.golden {
				t.Fatalf("deploy --json changed\n--- got ---\n%s\n--- want ---\n%s", got, tc.golden)
			}
			if dest.Len() != 0 {
				t.Errorf("--json alone wrote %q to the stream writer", dest.String())
			}
		})
	}
}

// TestDeployStreamRegistryOverride: the registry marks --stream on deploy and
// deploy promote as output: stream, so MCP and HTTP keep the document form. The
// test fails when either override is removed from internal/canon/domains/deploy.yaml.
func TestDeployStreamRegistryOverride(t *testing.T) {
	compattest.Set(t, true)
	reattachRealTree()
	defer PrepareTreeForGeneration()()
	reg, err := BuildRegistry(true)
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}
	for _, path := range []string{"nself deploy", "nself deploy promote"} {
		c, ok := reg.Lookup(path)
		if !ok {
			t.Fatalf("%s is not in the registry", path)
		}
		var flag *cmdregistry.Flag
		for i := range c.Flags {
			if c.Flags[i].Name == "stream" {
				flag = &c.Flags[i]
			}
		}
		if flag == nil || flag.Type != "bool" || flag.Output == nil || *flag.Output != "stream" {
			t.Errorf("%s: --stream registry entry = %+v, want a bool flag with output: stream", path, flag)
		}
		if c.Output != "document" {
			t.Errorf("%s: command output = %q; only the flag may switch to a stream", path, c.Output)
		}
	}
}
