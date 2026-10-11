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

// delayedHealth answers ready only once delay has passed since creation.
func delayedHealth(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	born := time.Now()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		applied := 0
		if time.Since(born) >= delay {
			applied = 1
		}
		_, _ = fmt.Fprintf(w, `{"status":"ok","migrations":{"applied":%d,"expected":1}}`, applied)
	}))
	t.Cleanup(s.Close)
	return s
}

// TestReadinessSharesOneDeadline: the readiness step has one bound for all
// members. The second member would be ready inside its own full timeout, but
// not inside the step's.
func TestReadinessSharesOneDeadline(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "a", nil)
	writeManifest(t, dir, "b", nil)
	urls := map[string]string{
		"a": delayedHealth(t, 150*time.Millisecond).URL,
		"b": delayedHealth(t, 400*time.Millisecond).URL,
	}
	h := Hooks{Timeout: 300 * time.Millisecond, Interval: 10 * time.Millisecond, HealthURL: func(m Member) string { return urls[m.Name] }}
	ms, err := LoadMembers(dir, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = waitMembers(context.Background(), h, ms)
	var se *StepError
	if !errors.As(err, &se) || se.Plugin != "b" {
		t.Fatalf("want a readiness failure naming b inside the shared 300ms, got %v", err)
	}
}
