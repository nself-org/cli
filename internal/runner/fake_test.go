package runner

// Purpose: a fake Executor shared by every *_test.go in this package, so
//   provision/verify logic is exercised without ever shelling out — the
//   G-012 brief requires tests never touch a real host.
// Inputs:  substring-matched rules registered by each test.
// Outputs: recorded command history (fakeExecutor.commands) for assertions
//   on exactly what would have been sent to a real host.

import (
	"context"
	"strings"
)

type fakeResponse struct {
	out string
	err error
}

type fakeRule struct {
	contains string
	resp     fakeResponse
}

// fakeExecutor implements Executor. Rules are checked in order; the first
// whose `contains` substring matches the command wins. commands records
// every command passed to Run, in order, for assertions.
type fakeExecutor struct {
	label    string
	rules    []fakeRule
	fallback fakeResponse
	commands []string
}

func (f *fakeExecutor) Run(_ context.Context, command string) (string, error) {
	f.commands = append(f.commands, command)
	for _, r := range f.rules {
		if strings.Contains(command, r.contains) {
			return r.resp.out, r.resp.err
		}
	}
	return f.fallback.out, f.fallback.err
}

func (f *fakeExecutor) Label() string { return f.label }

func (f *fakeExecutor) when(contains string, out string, err error) {
	f.rules = append(f.rules, fakeRule{contains: contains, resp: fakeResponse{out: out, err: err}})
}
