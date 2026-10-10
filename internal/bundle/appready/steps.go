// Package appready defines what "app-ready" means for a bundle install
// (P7-PLUG-19, cap:cli.bundle-app-ready) and runs the steps after the plugin
// files are on disk.
//
// Purpose: `nself bundle install` used to end at one build. This package names
// the nine steps (licence, fetch, install, ledger, reconcile, start, readiness,
// tracking, health), records each one with its result and duration, stops at the
// first failure and says which step and which plugin failed.
// Inputs: Hooks (cmd-supplied reconcile / start / Hasura / service-health
// functions, injected because this package may not import those layers) and the
// Members read from the installed plugin.json files.
// Outputs: a Report (human text through Render, JSON through the struct tags).
// Constraints: nothing here applies SQL or touches a database; HTTP goes through
// internal/plugin/readiness (internal/httptimeout); the dry run only prints.
package appready

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Step names, in the order they run. These exact strings are used in the
// human output, in the JSON and in E135 messages.
const (
	StepLicence   = "licence"
	StepFetch     = "fetch"
	StepInstall   = "install"
	StepLedger    = "ledger"
	StepReconcile = "reconcile"
	StepStart     = "start"
	StepReadiness = "readiness"
	StepTracking  = "tracking"
	StepHealth    = "health"
)

// Order is the fixed step order.
var Order = []string{
	StepLicence, StepFetch, StepInstall, StepLedger, StepReconcile,
	StepStart, StepReadiness, StepTracking, StepHealth,
}

// Step results.
const (
	ResultOK      = "ok"
	ResultFailed  = "failed"
	ResultSkipped = "skipped"
	ResultPlanned = "planned" // dry run
)

// StepResult is one line of the report.
type StepResult struct {
	Name       string `json:"name"`
	Result     string `json:"result"`
	DurationMS int64  `json:"duration_ms"`
	Detail     string `json:"detail,omitempty"`
}

// Report is the outcome of one bundle install.
type Report struct {
	Bundle    string       `json:"bundle"`
	AppReady  bool         `json:"app_ready"`
	Steps     []StepResult `json:"steps"`
	Failed    string       `json:"failed_step,omitempty"`
	FailedFor string       `json:"failed_plugin,omitempty"`
	// Note explains a report that is neither app-ready nor failed (for example
	// the install ran outside an nSelf project).
	Note string `json:"note,omitempty"`
}

// PrintPlan lists the nine steps for a dry run. It writes nothing but text.
func PrintPlan(w io.Writer, bundle string, members []string) {
	_, _ = fmt.Fprintf(w, "App-ready steps for bundle %s (%d plugins; nothing is changed in a dry run):\n", bundle, len(members))
	for i, name := range Order {
		_, _ = fmt.Fprintf(w, "  %d. %-9s %s\n", i+1, name, ResultPlanned)
	}
}

// Render writes one line per step, then the verdict line: APP-READY, or
// NOT APP-READY naming the step and plugin, or the Note.
func Render(w io.Writer, r Report) {
	for _, s := range r.Steps {
		mark := map[string]string{ResultOK: "ok", ResultFailed: "FAILED", ResultSkipped: "skipped"}[s.Result]
		line := fmt.Sprintf("  %-9s %-7s %5dms", s.Name, mark, s.DurationMS)
		if s.Detail != "" {
			line += "  " + s.Detail
		}
		_, _ = fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
	switch {
	case r.AppReady:
		_, _ = fmt.Fprintln(w, "APP-READY")
	case r.Failed != "":
		_, _ = fmt.Fprintf(w, "NOT APP-READY: step %s failed for plugin %s\n", r.Failed, r.FailedFor)
	case r.Note != "":
		_, _ = fmt.Fprintf(w, "NOT VERIFIED: %s\n", r.Note)
	}
}

// StepError is the error a step returns to say which plugin it failed for.
type StepError struct {
	Plugin string
	Err    error
}

func (e *StepError) Error() string { return e.Err.Error() }
func (e *StepError) Unwrap() error { return e.Err }

// Runner times each step and stops at the first failure: once a step has
// failed, every later step is recorded as skipped.
type Runner struct {
	bundle string
	done   map[string]StepResult
	failed *Failure
	note   string
	now    func() time.Time
}

// Failure is the first failed step.
type Failure struct {
	Step   string
	Plugin string
	Err    error
}

// NewRunner returns a Runner for bundle.
func NewRunner(bundle string) *Runner {
	return &Runner{bundle: bundle, done: map[string]StepResult{}, now: time.Now}
}

// Failure returns the first failure, or nil.
func (r *Runner) Failure() *Failure { return r.failed }

// Step runs fn as the named step. After a failure it does not run fn and
// records the step as skipped. fn returns a detail text and an error; wrap the
// error in *StepError to name the plugin.
func (r *Runner) Step(name string, fn func() (string, error)) error {
	if r.failed != nil {
		r.Skip(name, "not run: step "+r.failed.Step+" failed")
		return nil
	}
	start := r.now()
	detail, err := fn()
	ms := r.now().Sub(start).Milliseconds()
	if err != nil {
		r.record(name, StepResult{Name: name, Result: ResultFailed, DurationMS: ms, Detail: firstLine(err)})
		f := &Failure{Step: name, Plugin: "(bundle)", Err: err}
		var se *StepError
		if errors.As(err, &se) && se.Plugin != "" {
			f.Plugin = se.Plugin
		}
		r.failed = f
		return err
	}
	r.record(name, StepResult{Name: name, Result: ResultOK, DurationMS: ms, Detail: detail})
	return nil
}

// Record stores a step that already ran outside the Runner (the fetch step
// runs before the plan is printed).
// A failed result makes it the run's first failure unless one is already set.
func (r *Runner) Record(name, result string, d time.Duration, detail string) {
	r.record(name, StepResult{Name: name, Result: result, DurationMS: d.Milliseconds(), Detail: detail})
	if result == ResultFailed && r.failed == nil {
		r.failed = &Failure{Step: name, Plugin: "(bundle)", Err: errors.New(detail)}
	}
}

// Skip records a step that did not run.
func (r *Runner) Skip(name, detail string) {
	r.record(name, StepResult{Name: name, Result: ResultSkipped, Detail: detail})
}

func (r *Runner) record(name string, s StepResult) { r.done[name] = s }

// Unverified marks the run as not proven app-ready without failing it (the
// first reason is kept).
func (r *Runner) Unverified(reason string) {
	if r.note == "" {
		r.note = reason
	}
}

// Report assembles the report in canonical order. A step that never ran is
// listed as skipped. AppReady is true only when no step failed and nothing
// marked the run Unverified, so a run that could not check a step is never
// called ready.
func (r *Runner) Report() Report {
	rep := Report{Bundle: r.bundle, Note: r.note}
	for _, name := range Order {
		s, ok := r.done[name]
		if !ok {
			s = StepResult{Name: name, Result: ResultSkipped, Detail: "not reached"}
		}
		rep.Steps = append(rep.Steps, s)
	}
	rep.AppReady = r.failed == nil && r.note == ""
	if r.failed != nil {
		rep.Failed, rep.FailedFor = r.failed.Step, r.failed.Plugin
	}
	return rep
}

func firstLine(err error) string {
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
