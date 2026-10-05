// Command covfloor checks per-package statement coverage against floors kept
// as data (contract cap:cli.coverage-floors, EPIC P7-GUARD decision G7).
//
//	go run -mod=vendor ./tools/covfloor -profile coverage.out -floors .github/coverage-floors.txt [-strict] [-json]
//	go run -mod=vendor ./tools/covfloor -profile coverage.out -floors .github/coverage-floors.txt -write
//
// The profile is the single `go test -coverprofile` output of ./...; a package
// is the module-relative directory of its files, and its coverage is covered
// statements over all statements of those files. The floors file holds one
// `<package> <integer floor>  # comment` line per package.
//
// It fails closed. Exit 1 when a package is below its floor, when a floors line
// names a package with no statements in the profile (stale), when the profile
// or the floors file is missing, empty or malformed, and, with -strict, when a
// profiled package has no floors line. Without -strict such UNFLOORED packages
// are only printed. -write appends the missing packages at
// max(0, floor(measured)-2) and never changes an existing line. Exit 2 is a
// usage error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, time.Now()))
}

func run(args []string, stdout, stderr io.Writer, now time.Time) int {
	fs := flag.NewFlagSet("covfloor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profile := fs.String("profile", "", "Go coverage profile (go test -coverprofile ./...)")
	floorsFile := fs.String("floors", "", "floors file (<package> <floor>  # comment)")
	module := fs.String("module", "", "module path (default: read from -gomod)")
	gomod := fs.String("gomod", "go.mod", "go.mod to read the module path from")
	write := fs.Bool("write", false, "append packages without a floor at max(0, floor(pct)-2)")
	strict := fs.Bool("strict", false, "fail when a profiled package has no floors line")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	date := fs.String("date", now.UTC().Format("2006-01-02"), "date written into -write comments")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *profile == "" || *floorsFile == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "covfloor: -profile and -floors are required")
		return 2
	}
	mod := *module
	if mod == "" {
		var err error
		if mod, err = moduleOf(*gomod); err != nil {
			fmt.Fprintln(stderr, "covfloor:", err)
			return 1
		}
	}
	cov, err := ReadProfile(*profile, mod)
	if err != nil {
		fmt.Fprintln(stderr, "covfloor:", err)
		return 1
	}
	fl, err := ReadFloors(*floorsFile)
	if err != nil {
		fmt.Fprintln(stderr, "covfloor:", err)
		return 1
	}
	if *write {
		missing := map[string]float64{}
		have := fl.Map()
		for p, c := range cov {
			if _, ok := have[p]; !ok {
				missing[p] = c.Percent()
			}
		}
		if len(missing) > 0 {
			fl.Append(missing, *date)
			if err := fl.Write(*floorsFile); err != nil {
				fmt.Fprintln(stderr, "covfloor:", err)
				return 1
			}
		}
		fmt.Fprintf(stderr, "covfloor: wrote %d new floor line(s)\n", len(missing))
	}
	floors := fl.Map()
	rep := Evaluate(cov, floors)
	if *asJSON {
		out, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, "covfloor:", err)
			return 1
		}
		fmt.Fprintln(stdout, string(out))
	} else {
		rep.PrintText(stdout, floors, *strict)
	}
	if !rep.OK(*strict) {
		return 1
	}
	return 0
}

// moduleOf reads the module path from a go.mod file.
func moduleOf(gomod string) (string, error) {
	data, err := os.ReadFile(gomod)
	if err != nil {
		return "", fmt.Errorf("module path: %w (pass -module)", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "module" {
			return strings.Trim(f[1], `"`), nil
		}
	}
	return "", fmt.Errorf("module path: no module line in %s", gomod)
}
