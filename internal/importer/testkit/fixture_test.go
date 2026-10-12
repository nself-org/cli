package testkit

import (
	"context"
	"strings"
	"testing"
)

func TestStartTargetCommands(t *testing.T) {
	old := runNself
	defer func() { runNself = old }()
	var calls []string
	dir := t.TempDir()
	runNself = func(_ context.Context, d string, args ...string) error {
		if d != dir {
			t.Fatalf("wrong dir %s", d)
		}
		calls = append(calls, strings.Join(args, " "))
		return nil
	}
	if err := StartTarget(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls, "\n"); got != "init --non-interactive\nstart" {
		t.Fatalf("commands: %s", got)
	}
}

func TestRunNselfMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := runNself(context.Background(), t.TempDir(), "start"); err == nil {
		t.Fatal("missing binary must fail")
	}
}

func TestStartTargetStopsOnFailure(t *testing.T) {
	old := runNself
	defer func() { runNself = old }()
	n := 0
	runNself = func(context.Context, string, ...string) error { n++; return context.Canceled }
	if err := StartTarget(context.Background(), t.TempDir()); err == nil || n != 1 {
		t.Fatalf("err=%v calls=%d", err, n)
	}
}
