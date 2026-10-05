// Command perfbench is the performance harness of the nself CLI
// (contract:cli.perfbench v1; EPIC P7-GUARD, decision G8). It is stdlib-only
// and imports nothing from the cli module's internal packages, so it can build
// and measure the binary of any sha.
//
//	go run -mod=vendor ./tools/perfbench run [-scenario cold-start] [-bin p] [-runs 30] [-json]
//	go run -mod=vendor ./tools/perfbench healthy -report <golden-path-report.json> [-json]
//	go run -mod=vendor ./tools/perfbench ab -base <bin> -head <bin> [-runs 40] [-json]
//
// With no subcommand (the first argument is a flag, or there are no
// arguments) `run` is implied.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

// dispatch routes args to a subcommand and returns the process exit code.
func dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runCmd(args, stdout, stderr)
	}
	switch args[0] {
	case "run":
		return runCmd(args[1:], stdout, stderr)
	case "healthy":
		return healthyCmd(args[1:], stdout, stderr)
	case "ab":
		return abCmd(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "perfbench: unknown subcommand %q (have: run, healthy, ab)\n", args[0])
	return 2
}
