// Fixture stand-in for cli tools/facts (test.sh only). From .github/VERSION it
// writes .github/facts.json, or with -readme refreshes the "cli version:" line of
// that file. With -check it writes nothing and exits 1 when either is stale.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func main() {
	readme := flag.String("readme", "", "README to refresh")
	check := flag.Bool("check", false, "report stale files instead of writing")
	flag.Parse()
	raw, err := os.ReadFile(".github/VERSION")
	if err != nil {
		fail(err)
	}
	ver := strings.TrimSpace(string(raw))
	wantJSON := fmt.Sprintf("{\"version\":%q}\n", ver)
	if *check {
		got, err := os.ReadFile(".github/facts.json")
		if err != nil || string(got) != wantJSON {
			fail(fmt.Errorf("facts.json is stale for %s", ver))
		}
	} else if *readme == "" {
		if err := os.WriteFile(".github/facts.json", []byte(wantJSON), 0o644); err != nil {
			fail(err)
		}
		return
	}
	if *readme == "" {
		return
	}
	body, err := os.ReadFile(*readme)
	if err != nil {
		fail(err)
	}
	out := regexp.MustCompile(`cli version: [0-9.]+`).ReplaceAllString(string(body), "cli version: "+ver)
	if *check {
		if out != string(body) {
			fail(fmt.Errorf("%s is stale for %s", *readme, ver))
		}
		return
	}
	if err := os.WriteFile(*readme, []byte(out), 0o644); err != nil {
		fail(err)
	}
}
