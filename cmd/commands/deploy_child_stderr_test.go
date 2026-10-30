package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/deploy"
)

func TestChildStderrTailBoundsAndRedacts(t *testing.T) {
	tail := &childStderrTail{}
	for i := 0; i < 100; i++ {
		fmt.Fprintf(tail, "line %d\n", i)
	}
	fmt.Fprintf(tail, "connect postgres://app:s3cretpw@db:5432/x failed\n")
	fmt.Fprintf(tail, "HASURA_ADMIN_SECRET=topsecretvalue\n")
	fmt.Fprint(tail, strings.Repeat("x", 5000)+"\n")
	got := tail.String()
	for _, leak := range []string{"s3cretpw", "topsecretvalue"} {
		if strings.Contains(got, leak) {
			t.Fatalf("tail leaks %q: %s", leak, got)
		}
	}
	if strings.Contains(got, "line 10 ") || strings.Contains(got, "line 10|") {
		t.Fatalf("tail is not bounded: %s", got)
	}
	if n := len(strings.Split(got, " | ")); n > childStderrLines {
		t.Fatalf("tail has %d lines, want <= %d", n, childStderrLines)
	}
	if len(got) > childStderrLines*(childStderrLineMax+10) {
		t.Fatalf("tail too long: %d", len(got))
	}
}

func TestChildFailureCarriesTail(t *testing.T) {
	base := errors.New("exit status 1")
	f := &childFailure{err: base, tail: "boom"}
	if !errors.Is(f, base) || !strings.Contains(f.Error(), "exit status 1; child stderr: boom") {
		t.Fatalf("unexpected: %v", f)
	}
	if (&childFailure{err: base}).Error() != "exit status 1" {
		t.Fatal("empty tail must not add text")
	}
}

// TestRunCLISelfCapturingReturnsStderr re-executes the test binary as the
// child (runCLISelf spawns os.Executable) and checks the error carries stderr.
func TestRunCLISelfCapturingReturnsStderr(t *testing.T) {
	if os.Getenv("NSELF_TEST_CHILD_FAIL") == "1" {
		fmt.Fprintln(os.Stderr, "docker: compose plugin missing")
		os.Exit(1)
	}
	t.Setenv("NSELF_TEST_CHILD_FAIL", "1")
	err := runCLISelfCapturing(context.Background(), t.TempDir(), true, "-test.run=^TestRunCLISelfCapturingReturnsStderr$")
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("want an ExitError underneath, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "docker: compose plugin missing") {
		t.Fatalf("stderr not in error: %v", err)
	}
}

// TestScratchBuildFailureIncludesStderr: the deploy error names why the build failed.
func TestScratchBuildFailureIncludesStderr(t *testing.T) {
	old := deployScratchBuildFn
	t.Cleanup(func() { deployScratchBuildFn = old })
	deployScratchBuildFn = func(context.Context, string, bool, []deployStep) ([]deployStep, error) {
		return nil, &childFailure{err: errors.New("exit status 1"), tail: "no such file: docker-compose"}
	}
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.env", []byte("PROJECT_NAME=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runScratchBuild(context.Background(), dir, "qa", deploy.TargetSnapshot{})
	if err == nil || !strings.Contains(err.Error(), "scratch build failed") || !strings.Contains(err.Error(), "no such file: docker-compose") {
		t.Fatalf("got %v", err)
	}
}
