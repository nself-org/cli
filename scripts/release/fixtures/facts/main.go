// Fixture stand-in for cli tools/facts (test.sh only): rewrites .github/facts.json,
// or with -readme the "cli version:" line of the given file, from .github/VERSION.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	readme := flag.String("readme", "", "README to refresh")
	flag.Parse()
	raw, err := os.ReadFile(".github/VERSION")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ver := strings.TrimSpace(string(raw))
	if *readme == "" {
		os.WriteFile(".github/facts.json", []byte(fmt.Sprintf("{\"version\":%q}\n", ver)), 0o644)
		return
	}
	body, err := os.ReadFile(*readme)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out := regexp.MustCompile(`cli version: [0-9.]+`).ReplaceAllString(string(body), "cli version: "+ver)
	os.WriteFile(*readme, []byte(out), 0o644)
}
