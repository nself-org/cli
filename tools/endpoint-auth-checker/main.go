// endpoint-auth-checker — static analysis tool that enforces auth middleware on every HTTP route.
//
// Usage:
//
//	endpoint-auth-checker --dirs plugins/,plugins-pro/paid/,web/backend/ --fail-on-unknown --annotation-format github
//
// Exit codes:
//
//	0 — no violations (or --fail-on-unknown not set)
//	1 — one or more routes lack a recognized auth middleware
//	2 — scan error (missing or unreadable directory, or no .go file scanned)
//
// Scope: the tool scans only the directories it is given and skips vendor/ and
// testdata/ below each root. Its CI workflow runs it on its own fixtures only;
// cli's own HTTP routes are not scanned there (see the workflow header).
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	dirsFlag := flag.String("dirs", "", "Comma-separated list of directories to scan (required)")
	failFlag := flag.Bool("fail-on-unknown", false, "Exit 1 if any route lacks recognized auth middleware")
	fmtFlag := flag.String("annotation-format", "text", "Output format: github | text | json")
	flag.Parse()

	if *dirsFlag == "" {
		fmt.Fprintln(os.Stderr, "endpoint-auth-checker: --dirs is required")
		os.Exit(2)
	}

	dirs := parseDirs(*dirsFlag)

	format := AnnotationFormat(*fmtFlag)
	switch format {
	case FormatGitHub, FormatText, FormatJSON:
	default:
		fmt.Fprintf(os.Stderr, "endpoint-auth-checker: unknown --annotation-format %q (use github|text|json)\n", *fmtFlag)
		os.Exit(2)
	}

	routes, err := ScanDirs(dirs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "endpoint-auth-checker: scan error: %v\n", err)
		os.Exit(2)
	}

	failures := ClassifyAll(routes)
	code := Report(failures, format, *failFlag)
	os.Exit(code)
}

func parseDirs(raw string) []string {
	var dirs []string
	for _, d := range strings.Split(raw, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	return dirs
}
