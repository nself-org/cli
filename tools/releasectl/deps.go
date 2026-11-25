package main

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"time"
)

// Deps holds everything the commands touch outside pure logic.
type Deps struct {
	Runner      Runner
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       *bufio.Reader
	Interactive bool                // stdin is a terminal
	Root        string              // repo root
	Env         func(string) string // environment lookup
	HaveGH      func() bool         // gh CLI present
	Now         func() time.Time
	Sleep       func(time.Duration) // poll sleeps; nil means time.Sleep
	Have        func(string) bool   // tool on PATH (command -v); nil means all present
}

// realDeps wires production dependencies.
func realDeps() Deps {
	root, _ := os.Getwd()
	fi, err := os.Stdin.Stat()
	tty := err == nil && fi.Mode()&os.ModeCharDevice != 0
	return Deps{
		Runner: ExecRunner{}, Stdout: os.Stdout, Stderr: os.Stderr,
		Stdin: bufio.NewReader(os.Stdin), Interactive: tty, Root: root,
		Env:    os.Getenv,
		HaveGH: func() bool { _, e := exec.LookPath("gh"); return e == nil },
		Now:    time.Now, Sleep: time.Sleep,
		Have: func(t string) bool { _, e := exec.LookPath(t); return e == nil },
	}
}

// readLine reads one trimmed line from stdin.
func (d Deps) readLine() string {
	s, _ := d.Stdin.ReadString('\n')
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
