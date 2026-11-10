// Package main implements sportgen, the Go port of scripts/sport/generate-core-services.sh,
// generate-f02.sh and generate-f17.sh. Usage: sportgen core-services|f02|f17.
// Every external command goes through the injected Runner.
package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

// Deps holds everything the generators touch outside pure logic.
type Deps struct {
	Runner Runner
	Stdout io.Writer
	Stderr io.Writer
	Root   string              // repo root (the cli/ directory)
	Env    func(string) string // environment lookup
	Now    func() time.Time    // clock; output uses UTC
}

func realDeps() Deps {
	root, _ := os.Getwd()
	return Deps{Runner: ExecRunner{Dir: root}, Stdout: os.Stdout, Stderr: os.Stderr,
		Root: root, Env: os.Getenv, Now: time.Now}
}

// Run executes sportgen with real dependencies.
func Run(args []string) int { return RunWith(realDeps(), args) }

// RunWith dispatches to a generator with injected dependencies.
func RunWith(d Deps, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(d.Stderr, "usage: sportgen <core-services|f02|f17>")
		return 2
	}
	switch args[0] {
	case "core-services":
		return runCoreServices(d)
	case "f02":
		return runF02(d)
	case "f17":
		return runF17(d)
	}
	fmt.Fprintf(d.Stderr, "unknown command: %s\n", args[0])
	return 2
}

func main() { os.Exit(Run(os.Args[1:])) }
