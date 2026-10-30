package childerr

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestTailBoundsAndRedacts(t *testing.T) {
	tail := &Tail{}
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
	if n := len(strings.Split(got, " | ")); n > MaxLines {
		t.Fatalf("tail has %d lines, want <= %d", n, MaxLines)
	}
	if len(got) > MaxLines*(MaxLine+10) {
		t.Fatalf("tail too long: %d", len(got))
	}
}

func TestFailureCarriesTail(t *testing.T) {
	base := errors.New("exit status 1")
	f := &Failure{Err: base, Tail: "boom"}
	if !errors.Is(f, base) || !strings.Contains(f.Error(), "exit status 1; child stderr: boom") {
		t.Fatalf("unexpected: %v", f)
	}
	if (&Failure{Err: base}).Error() != "exit status 1" {
		t.Fatal("empty tail must not add text")
	}
}

// TestHelperChild is the child of TestRunReturnsStderr: it runs only when the
// word "childfail" follows "--" on its command line.
func TestHelperChild(t *testing.T) {
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != "childfail" {
		return
	}
	fmt.Fprintln(os.Stderr, "docker: compose plugin missing")
	os.Exit(1)
}

func TestRunReturnsStderr(t *testing.T) {
	c := exec.Command(os.Args[0], "-test.run=^TestHelperChild$", "--", "childfail")
	err := Run(c)
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("want an ExitError underneath, got %v", err)
	}
	if !strings.Contains(err.Error(), "docker: compose plugin missing") {
		t.Fatalf("stderr not in error: %v", err)
	}
	if err := Run(exec.Command(os.Args[0], "-test.run=^TestHelperChild$")); err != nil {
		t.Fatalf("a passing child must not fail: %v", err)
	}
}
