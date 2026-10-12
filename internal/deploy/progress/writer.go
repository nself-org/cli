package progress

// Purpose: the line writer of contract:cli.deploy-progress v1.
// Inputs: an io.Writer, the command path for the envelope, step events.
// Outputs: compact v1 envelopes, one per line, written with one Write each.
// Constraints: a nil *Writer is a no-op, so call sites need no stream check;
// after the first failed write the stream stops and Err reports why.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/cli/internal/observability"
	"github.com/nself-org/cli/internal/output"
)

// maxDetail caps the length of a detail note, in runes.
const maxDetail = 200

// sink is the state every view of one stream shares.
type sink struct {
	mu       sync.Mutex
	w        io.Writer
	command  string
	now      func() time.Time
	start    time.Time
	seq      int
	steps    []Step
	finished bool
	err      error
}

// Writer writes progress lines for one run. At returns views that share the
// sequence, the clock and the destination but label events with another
// environment or server.
type Writer struct {
	s           *sink
	env, server string
}

// NewWriter returns a Writer that writes to w. command is the registry command
// path put in every envelope ("deploy" or "deploy promote"). The elapsed
// clock starts now.
func NewWriter(w io.Writer, command string) *Writer {
	return NewWriterClock(w, command, time.Now)
}

// NewWriterClock is NewWriter with an injected clock, for tests.
func NewWriterClock(w io.Writer, command string, now func() time.Time) *Writer {
	return &Writer{s: &sink{w: w, command: command, now: now, start: now()}}
}

// At returns a view of w whose events carry env and server ("" is null). It
// returns nil for a nil w.
func (w *Writer) At(env, server string) *Writer {
	if w == nil {
		return nil
	}
	return &Writer{s: w.s, env: env, server: server}
}

// Event writes one progress line for step in phase and records the step's
// status. detail ("" is null) is redacted and shortened first.
func (w *Writer) Event(step, phase, detail string) {
	w.emit(step, phase, detail, statusForPhase(phase))
}

// Status writes one progress line for a step that finished with a deploy
// status ("done", "failed", "unhealthy", "pending", ...), choosing the phase
// with PhaseForStatus, and records the raw status for the final step list.
func (w *Writer) Status(step, status, detail string) {
	w.emit(step, PhaseForStatus(status), detail, status)
}

// Steps returns a copy of the recorded step list, in first-seen order.
func (w *Writer) Steps() []Step {
	if w == nil {
		return nil
	}
	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	return append([]Step(nil), w.s.steps...)
}

// Err returns the first write error, or nil.
func (w *Writer) Err() error {
	if w == nil {
		return nil
	}
	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	return w.s.err
}

// Finish writes the last line once: data.type "result" with the outcome and
// steps (nil means the recorded step list). Later calls and events are
// ignored. It returns the first write error of the stream.
func (w *Writer) Finish(result string, steps []Step) error {
	if w == nil {
		return nil
	}
	s := w.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished || s.err != nil {
		return s.err
	}
	s.finished = true
	if steps == nil {
		steps = s.steps
	}
	if steps == nil {
		steps = []Step{}
	}
	s.writeLine(Result{Type: TypeResult, Result: result, Steps: steps})
	return s.err
}

func (w *Writer) emit(step, phase, detail, status string) {
	if w == nil {
		return
	}
	s := w.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished || s.err != nil {
		return
	}
	s.record(step, status)
	s.seq++
	ev := Event{Type: TypeProgress, Seq: s.seq, Step: step, Phase: phase,
		Env: nullable(w.env), Server: nullable(w.server),
		ElapsedMS: s.now().Sub(s.start).Milliseconds(), Detail: nullable(clean(detail))}
	if ev.ElapsedMS < 0 {
		ev.ElapsedMS = 0
	}
	s.writeLine(ev)
}

// record sets the status of step, appending it on first sight.
func (s *sink) record(step, status string) {
	for i := range s.steps {
		if s.steps[i].Step == step {
			s.steps[i].Status = status
			return
		}
	}
	s.steps = append(s.steps, Step{Step: step, Status: status})
}

// writeLine renders data as one compact v1 envelope line. The caller holds mu.
func (s *sink) writeLine(data any) {
	var doc bytes.Buffer
	if err := output.EmitData(output.Writer{Out: &doc}, s.command, data); err != nil {
		s.err = fmt.Errorf("progress: render: %w", err)
		return
	}
	var line bytes.Buffer
	if err := json.Compact(&line, doc.Bytes()); err != nil {
		s.err = fmt.Errorf("progress: compact: %w", err)
		return
	}
	line.WriteByte('\n')
	if _, err := s.w.Write(line.Bytes()); err != nil {
		s.err = err
	}
}

// statusForPhase is the step status recorded for an event with no deploy
// status of its own.
func statusForPhase(phase string) string {
	switch phase {
	case PhaseStart:
		return "running"
	case PhaseRollback:
		return "rolled_back"
	}
	return phase
}

// nullable maps "" to a JSON null.
func nullable(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// clean redacts secrets, keeps the first line and shortens it.
func clean(detail string) string {
	detail = strings.TrimSpace(observability.Redact(detail))
	if i := strings.IndexAny(detail, "\r\n"); i >= 0 {
		detail = detail[:i]
	}
	if r := []rune(detail); len(r) > maxDetail {
		detail = string(r[:maxDetail]) + "..."
	}
	return detail
}

type ctxKey struct{}

// WithWriter returns ctx carrying w.
func WithWriter(ctx context.Context, w *Writer) context.Context {
	return context.WithValue(ctx, ctxKey{}, w)
}

// FromContext returns the Writer ctx carries, or nil when the run is not
// streaming (a nil ctx is allowed).
func FromContext(ctx context.Context) *Writer {
	if ctx == nil {
		return nil
	}
	w, _ := ctx.Value(ctxKey{}).(*Writer)
	return w
}
