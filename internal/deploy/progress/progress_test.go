package progress

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/nself-org/cli/schemas"
)

// fakeClock advances 5ms per call.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { c.t = c.t.Add(5 * time.Millisecond); return c.t }

func newTestWriter(buf *bytes.Buffer) *Writer {
	c := &fakeClock{t: time.Unix(1700000000, 0)}
	return NewWriterClock(buf, "deploy", c.now)
}

func eventSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	b, ok := schemas.Lookup("deploy-progress.v1.schema.json")
	if !ok {
		t.Fatal("schemas/deploy-progress.v1.schema.json is missing (run make schemas)")
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	rs, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

type envelope struct {
	SchemaVersion string          `json:"schema_version"`
	Command       string          `json:"command"`
	Data          json.RawMessage `json:"data"`
}

func lines(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()
	out := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(out) == 1 && out[0] == "" {
		return nil
	}
	return out
}

func TestEventLines(t *testing.T) {
	rs := eventSchema(t)
	var buf bytes.Buffer
	w := newTestWriter(&buf)
	w.Event("Build images", PhaseStart, "")
	w.Status("Build images", "done", "")
	w.At("prod", "app-1").Status("deploy", "failed", "boom\nsecond line")
	w.At("prod", "").Status("Health checks", "pending", "dry-run")
	w.Event("apply", PhaseRollback, "undone")
	if err := w.Finish(ResultFailed, nil); err != nil {
		t.Fatal(err)
	}
	got := lines(t, &buf)
	if len(got) != 6 {
		t.Fatalf("lines = %d, want 6:\n%s", len(got), buf.String())
	}
	want := []struct{ step, phase string }{
		{"Build images", "start"}, {"Build images", "done"}, {"deploy", "failed"}, {"Health checks", "skipped"}, {"apply", "rollback"},
	}
	for i, line := range got[:5] {
		var env envelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			t.Fatalf("line %d: %v", i+1, err)
		}
		if env.SchemaVersion != "1" || env.Command != "deploy" {
			t.Errorf("line %d envelope = %+v", i+1, env)
		}
		if strings.Contains(line, "\n") || strings.Contains(line, ": ") || strings.Contains(line, "  ") {
			t.Errorf("line %d is not compact: %q", i+1, line)
		}
		var v any
		_ = json.Unmarshal(env.Data, &v)
		if err := rs.Validate(v); err != nil {
			t.Errorf("line %d data does not validate: %v: %s", i+1, err, env.Data)
		}
		var ev Event
		_ = json.Unmarshal(env.Data, &ev)
		if ev.Seq != i+1 || ev.Step != want[i].step || ev.Phase != want[i].phase {
			t.Errorf("line %d = %+v, want seq %d %v", i+1, ev, i+1, want[i])
		}
		if !strings.HasPrefix(string(env.Data), `{"type":"progress","seq":`) {
			t.Errorf("line %d data does not lead with type then seq: %s", i+1, env.Data)
		}
	}
	if !strings.Contains(got[2], `"env":"prod","server":"app-1"`) || !strings.Contains(got[2], `"detail":"boom"`) {
		t.Errorf("scoped event lost its labels or kept a second line: %s", got[2])
	}
	if !strings.Contains(got[0], `"env":null,"server":null,"elapsed_ms":5,"detail":null`) {
		t.Errorf("unlabelled event should carry nulls: %s", got[0])
	}
	var last envelope
	if err := json.Unmarshal([]byte(got[5]), &last); err != nil {
		t.Fatal(err)
	}
	var res Result
	if err := json.Unmarshal(last.Data, &res); err != nil {
		t.Fatal(err)
	}
	if res.Type != "result" || res.Result != ResultFailed || len(res.Steps) != 4 {
		t.Errorf("result = %+v", res)
	}
	if string(last.Data) != `{"type":"result","result":"failed","steps":[{"step":"Build images","status":"done"},{"step":"deploy","status":"failed"},{"step":"Health checks","status":"pending"},{"step":"apply","status":"rolled_back"}]}` {
		t.Errorf("result data = %s", last.Data)
	}
}

func TestStatusPhases(t *testing.T) {
	cases := map[string]string{
		"done": PhaseDone, "failed": PhaseFailed, "unhealthy": PhaseFailed,
		"pending": PhaseSkipped, "skipped": PhaseSkipped, "planned": PhaseSkipped, "ok": PhaseDone, "": PhaseDone,
	}
	for status, want := range cases {
		if got := PhaseForStatus(status); got != want {
			t.Errorf("PhaseForStatus(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestFinishRules(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriter(&buf)
	if err := w.Finish(ResultOK, nil); err != nil {
		t.Fatal(err)
	}
	w.Event("late", PhaseStart, "")
	_ = w.Finish(ResultFailed, nil)
	got := lines(t, &buf)
	if len(got) != 1 || !strings.Contains(got[0], `"data":{"type":"result","result":"ok","steps":[]}`) {
		t.Fatalf("want one result line with steps [], got %q", got)
	}
}

func TestDetailRedactedAndShort(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriter(&buf)
	w.Status("push", "failed", "dial "+"postgres:"+"//app:"+"hunter2"+"@db.internal:5432/x failed")
	w.Status("push2", "failed", strings.Repeat("x", 500))
	got := buf.String()
	if strings.Contains(got, "hunter2") {
		t.Errorf("credential leaked into detail: %s", got)
	}
	var ev Event
	var env envelope
	_ = json.Unmarshal([]byte(lines(t, &buf)[1]), &env)
	_ = json.Unmarshal(env.Data, &ev)
	if ev.Detail == nil || len([]rune(*ev.Detail)) > maxDetail+3 {
		t.Errorf("detail not shortened: %v", ev.Detail)
	}
}

type failingWriter struct{ n int }

func (f *failingWriter) Write(p []byte) (int, error) {
	f.n++
	return 0, errors.New("pipe closed")
}

func TestWriteErrorStopsStream(t *testing.T) {
	fw := &failingWriter{}
	w := NewWriter(fw, "deploy")
	w.Event("a", PhaseStart, "")
	w.Event("b", PhaseStart, "")
	if err := w.Finish(ResultOK, nil); err == nil || w.Err() == nil {
		t.Fatal("write error was not reported")
	}
	if fw.n != 1 {
		t.Errorf("writes after the first failure = %d, want none", fw.n-1)
	}
}

func TestNilWriterIsNoOp(t *testing.T) {
	var w *Writer
	w.Event("a", PhaseStart, "")
	w.Status("a", "done", "")
	if w.At("e", "s") != nil || w.Steps() != nil || w.Err() != nil || w.Finish(ResultOK, nil) != nil {
		t.Fatal("a nil Writer must do nothing")
	}
	NewPromoteObserver(nil, "prod").Start("x")
	if FromContext(context.Background()) != nil || FromContext(nil) != nil { //nolint:staticcheck // nil ctx is allowed by contract
		t.Fatal("no writer expected in a bare context")
	}
}

func TestContextRoundTrip(t *testing.T) {
	w := NewWriter(&bytes.Buffer{}, "deploy")
	if got := FromContext(WithWriter(context.Background(), w)); got != w {
		t.Fatal("writer did not survive the context")
	}
}

func TestPromoteObserverPhases(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriter(&buf)
	o := NewPromoteObserver(w, "prodsim")
	o.Start("apply")
	o.Done("apply", time.Second)
	o.Failed("health", errors.New("unhealthy"))
	o.Skipped("verify", "no release manifest")
	o.Rollback("apply", nil)
	o.Rollback("snapshot", errors.New("restore failed"))
	var phases []string
	for _, l := range lines(t, &buf) {
		var env envelope
		var ev Event
		_ = json.Unmarshal([]byte(l), &env)
		_ = json.Unmarshal(env.Data, &ev)
		if ev.Env == nil || *ev.Env != "prodsim" || ev.Server != nil {
			t.Errorf("labels = %+v", ev)
		}
		phases = append(phases, ev.Phase)
	}
	if strings.Join(phases, ",") != "start,done,failed,skipped,rollback,rollback" {
		t.Errorf("phases = %v", phases)
	}
	if !strings.Contains(buf.String(), `"detail":"rollback failed: restore failed"`) {
		t.Errorf("failed undo not reported: %s", buf.String())
	}
}

func TestResultForPromote(t *testing.T) {
	cases := map[string]string{"ok": "ok", "rolled_back": "rolled_back", "rollback_failed": "rollback_failed", "failed": "failed", "": "failed", "weird": "failed"}
	for in, want := range cases {
		if got := ResultForPromote(in); got != want {
			t.Errorf("ResultForPromote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOutcomeOverridesRecordedSteps(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriter(&buf)
	if _, _, ok := w.Outcome(); ok {
		t.Fatal("no outcome expected yet")
	}
	w.Event("apply", PhaseStart, "")
	w.SetOutcome(ResultRolledBack, []Step{{Step: "apply", Status: "rolled_back"}})
	res, steps, ok := w.Outcome()
	if !ok || res != ResultRolledBack || len(steps) != 1 {
		t.Fatalf("outcome = %q %v %v", res, steps, ok)
	}
	_ = w.Finish(res, steps)
	if got := lines(t, &buf); !strings.HasSuffix(got[len(got)-1], `"data":{"type":"result","result":"rolled_back","steps":[{"step":"apply","status":"rolled_back"}]}}`) {
		t.Fatalf("last line = %s", got[len(got)-1])
	}
}
