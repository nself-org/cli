package readiness

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var ce *errs.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("want *errs.CLIError, got %T: %v", err, err)
	}
	return ce.Code
}

func TestReadinessWaitConverges(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		applied := 0
		if n >= 3 {
			applied = 4
		}
		fmt.Fprintf(w, `{"status":"ok","migrations":{"applied":%d,"expected":4}}`, applied)
	}))
	defer srv.Close()

	if err := Wait(context.Background(), "claw", srv.URL, 10*time.Second, WithInterval(10*time.Millisecond)); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got < 3 {
		t.Fatalf("returned after %d probes; must keep polling until applied == expected", got)
	}
}

func TestReadinessWaitTimeoutE118(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"migrations":{"applied":2,"expected":5}}`)
	}))
	defer srv.Close()

	start := time.Now()
	err := Wait(context.Background(), "mux", srv.URL, 150*time.Millisecond, WithInterval(10*time.Millisecond))
	if err == nil {
		t.Fatal("Wait returned nil for a plugin that never converges")
	}
	if c := codeOf(t, err); c != "E118" {
		t.Fatalf("code = %s, want E118", c)
	}
	for _, want := range []string{"mux", "2 of 5"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("wait was not bounded: %s", time.Since(start))
	}

	// Never reachable: still E118, names the plugin, keeps the cause.
	srv.Close()
	err = Wait(context.Background(), "auth", srv.URL, 100*time.Millisecond, WithInterval(10*time.Millisecond))
	if c := codeOf(t, err); c != "E118" || !strings.Contains(err.Error(), "auth") {
		t.Fatalf("unreachable: %v", err)
	}

	// Caller cancellation is not E118.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = Wait(ctx, "auth", srv.URL, time.Minute, WithInterval(10*time.Millisecond))
	var ce *errs.CLIError
	if err == nil || errors.As(err, &ce) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ctx: want context.Canceled, got %v", err)
	}

	// An unbounded wait is refused.
	if err := Wait(context.Background(), "auth", srv.URL, 0); err == nil {
		t.Fatal("zero timeout must be an error")
	}
}

func TestReadinessMissingMigrationsField(t *testing.T) {
	bodies := []string{
		`{"status":"ok"}`,
		`{"migrations":null}`,
		`{"migrations":{"applied":3}}`,
		`{"migrations":{"applied":-1,"expected":2}}`,
		`not json`,
	}
	for _, b := range bodies {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, b) }))
		start := time.Now()
		err := Wait(context.Background(), "ai", srv.URL, 30*time.Second, WithInterval(10*time.Millisecond))
		srv.Close()
		if err == nil {
			t.Fatalf("body %q: Wait returned nil", b)
		}
		if c := codeOf(t, err); c != "E119" {
			t.Fatalf("body %q: code = %s, want E119", b, c)
		}
		if !strings.Contains(err.Error(), "ai") {
			t.Errorf("body %q: error does not name the plugin: %v", b, err)
		}
		if time.Since(start) > 5*time.Second {
			t.Fatalf("body %q: E119 must fail at once, took %s", b, time.Since(start))
		}
	}
}

func TestReadinessTimeoutFromEnv(t *testing.T) {
	cases := []struct {
		env  string
		want time.Duration
		bad  bool
	}{
		{"", DefaultTimeout, false}, {"45", 45 * time.Second, false}, {"3m", 3 * time.Minute, false},
		{"0", 0, true}, {"-5", 0, true}, {"soon", 0, true},
	}
	for _, c := range cases {
		t.Setenv(EnvTimeout, c.env)
		got, err := TimeoutFromEnv()
		if (err != nil) != c.bad || got != c.want {
			t.Errorf("env %q: got %v, %v; want %v bad=%v", c.env, got, err, c.want, c.bad)
		}
	}
}
