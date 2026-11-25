package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// reply is one canned answer of the scripted runner.
type reply struct {
	out    string
	stderr string
	code   int // non-zero makes the call fail with a RunError
	hook   func(args []string)
}

// scriptRunner answers by the longest matching "name args" prefix and records
// every call. It never runs a real command. The last reply of a rule repeats.
type scriptRunner struct {
	calls []string
	rules map[string][]reply
}

func (s *scriptRunner) Run(name string, args ...string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	s.calls = append(s.calls, line)
	best := ""
	for p := range s.rules {
		if strings.HasPrefix(line, p) && len(p) > len(best) {
			best = p
		}
	}
	rs := s.rules[best]
	if best == "" || len(rs) == 0 {
		return "", &RunError{Code: 1, Stderr: "unexpected call: " + line}
	}
	rep := rs[0]
	if len(rs) > 1 {
		s.rules[best] = rs[1:]
	}
	if rep.hook != nil {
		rep.hook(args)
	}
	if rep.code != 0 {
		return rep.out, &RunError{Code: rep.code, Stderr: rep.stderr}
	}
	return rep.out, nil
}

func (s *scriptRunner) count(prefix string) int {
	n := 0
	for _, c := range s.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// relEnv is a Release over a temp dir with captured stdout and counted sleeps.
type relEnv struct {
	r      *Release
	run    *scriptRunner
	out    *bytes.Buffer
	sleeps int
}

func newRel(t *testing.T, rules map[string][]reply) *relEnv {
	t.Helper()
	e := &relEnv{run: &scriptRunner{rules: rules}, out: &bytes.Buffer{}}
	work := t.TempDir()
	e.r = &Release{
		Run: e.run, Out: e.out, Poll: 30 * time.Second,
		Sleep:   func(time.Duration) { e.sleeps++ },
		Version: "1.5.0", Tag: "v1.5.0", Work: work, State: filepath.Join(work, "state"),
		Step: "S4", ID: "merge-admin",
	}
	return e
}

func one(r reply) []reply { return []reply{r} }
