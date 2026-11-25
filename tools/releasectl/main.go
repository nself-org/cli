// Package main implements the releasectl CLI tool.
package main

import (
	"fmt"
	"os"
)

// Run executes the releasectl command logic.
func Run(args []string) int { return RunWith(realDeps(), args) }

// RunWith executes the command logic with injected dependencies.
func RunWith(d Deps, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(d.Stderr, "usage: releasectl <create|retract|gate|run> [args...]\n")
		return 2
	}

	switch args[0] {
	case "create":
		return runCreate(d, args[1:])
	case "retract":
		return runRetract(d, args[1:])
	case "gate":
		return runGate(d, args[1:])
	case "run":
		return RunRelease(d, args[1:])
	default:
		fmt.Fprintf(d.Stderr, "unknown command: %s\n", args[0])
		return 2
	}
}

func main() {
	os.Exit(Run(os.Args[1:]))
}
