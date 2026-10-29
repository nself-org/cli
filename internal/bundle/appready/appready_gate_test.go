package appready

// appready_gate_test.go — P7-PLUG-19 fix round 1: the nil-Start gate (v1.4),
// ErrNoProject from Start, wrapped StepError and the shared per-step deadline.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRunNilStartSkipsWaitingSteps: with no Start hook nothing polls, the four
// steps are skipped as not verified, and the report never says APP-READY alone.
func TestRunNilStartSkipsWaitingSteps(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "a", nil)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { atomic.AddInt32(&hits, 1) }))
	defer srv.Close()
	h := fastHooks(Hooks{
		Reconcile: func(context.Context) error { return nil },
		HealthURL: func(Member) string { return srv.URL },
		ExportMetadata: func(context.Context) ([]byte, error) {
			t.Error("tracking ran without a started stack")
			return nil, nil
		},
		ServiceHealth: func(context.Context) error { t.Error("service health ran without a started stack"); return nil },
	})
	rn := NewRunner("task")
	out := Run(context.Background(), rn, h, dir, []string{"a"}, true)
	rep := rn.Report()
	if !out.Reconciled || rn.Failure() != nil || rep.AppReady || hits != 0 {
		t.Fatalf("outcome %+v report %+v hits=%d", out, rep, hits)
	}
	for _, s := range rep.Steps[5:] {
		if s.Result != ResultSkipped || s.Detail != NotVerifiedV15 {
			t.Errorf("step %+v, want skipped %q", s, NotVerifiedV15)
		}
	}
	var buf bytes.Buffer
	Render(&buf, rep)
	if !strings.Contains(buf.String(), "NOT VERIFIED") || !strings.Contains(buf.String(), "v1.5 behaviour, NSELF_V15=1") {
		t.Errorf("render lacks the not-verified note:\n%s", buf.String())
	}
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.TrimSpace(line) == "APP-READY" {
			t.Errorf("render claims APP-READY:\n%s", buf.String())
		}
	}
}

// TestRunStartNoProject: ErrNoProject from Start (membership unchanged, so
// reconcile never discovered it) skips the rest and is not a failure.
func TestRunStartNoProject(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "a", nil)
	h := fastHooks(Hooks{
		Reconcile:      func(context.Context) error { return nil },
		Start:          func(context.Context) error { return fmt.Errorf("start: %w", ErrNoProject) },
		ExportMetadata: func(context.Context) ([]byte, error) { t.Error("tracking ran outside a project"); return nil, nil },
	})
	rn := NewRunner("task")
	Run(context.Background(), rn, h, dir, []string{"a"}, false)
	rep := rn.Report()
	if rn.Failure() != nil || rep.AppReady || !strings.Contains(rep.Note, "not in an nSelf project") {
		t.Fatalf("report = %+v failure = %+v", rep, rn.Failure())
	}
	for _, s := range rep.Steps[5:] {
		if s.Result != ResultSkipped || s.Detail != "not in an nSelf project" {
			t.Errorf("step %+v", s)
		}
	}
}

// TestStepNamesPluginThroughWrapping: a StepError wrapped by fmt.Errorf still
// names its plugin.
func TestStepNamesPluginThroughWrapping(t *testing.T) {
	rn := NewRunner("b")
	_ = rn.Step(StepReadiness, func() (string, error) {
		return "", fmt.Errorf("outer: %w", &StepError{Plugin: "slowpoke", Err: errors.New("inner")})
	})
	if f := rn.Failure(); f == nil || f.Plugin != "slowpoke" {
		t.Fatalf("failure = %+v", f)
	}
}

// fakeClock is a controllable time source: it moves only when the test says so.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// healthServer answers a ready /health (and runs onReady once, on the first
// ready answer), or a never-ready one when ready is false. Readiness is set by
// the test, never by elapsed wall time.
func healthServer(t *testing.T, ready bool, onReady func()) *httptest.Server {
	t.Helper()
	var once sync.Once
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		applied := 0
		if ready {
			applied = 1
			if onReady != nil {
				once.Do(onReady)
			}
		}
		_, _ = fmt.Fprintf(w, `{"status":"ok","migrations":{"applied":%d,"expected":1}}`, applied)
	}))
	t.Cleanup(s.Close)
	return s
}

// TestReadinessSharesOneDeadline: the readiness step has one bound for all
// members. Member a becomes ready and, on the injected clock, spends 250ms of
// the 300ms step budget; member b then gets only the 50ms left, not a fresh
// 300ms. b's E118 names the budget it was given, so the check reads the
// deadline arithmetic, not how long the machine took.
func TestReadinessSharesOneDeadline(t *testing.T) {
	const budget, spent = 300 * time.Millisecond, 250 * time.Millisecond
	dir := t.TempDir()
	writeManifest(t, dir, "a", nil)
	writeManifest(t, dir, "b", nil)
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	urls := map[string]string{
		"a": healthServer(t, true, func() { clk.Advance(spent) }).URL,
		"b": healthServer(t, false, nil).URL,
	}
	h := Hooks{Timeout: budget, Interval: 10 * time.Millisecond, now: clk.Now, HealthURL: func(m Member) string { return urls[m.Name] }}
	ms, err := LoadMembers(dir, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = waitMembers(context.Background(), h, ms)
	var se *StepError
	if !errors.As(err, &se) || se.Plugin != "b" {
		t.Fatalf("want a readiness failure naming b, got %v", err)
	}
	if want := "after " + (budget - spent).String(); !strings.Contains(err.Error(), want) {
		t.Fatalf("b was given %q, want the shared remainder (%s): a per-member budget would say after %s", err, want, budget)
	}
}
