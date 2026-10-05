// Command schemagen writes and checks the generated JSON Schemas under schemas/.
//
// Purpose: one generator for every machine contract the CLI publishes
// (envelope, error object, command registry, each envelope command's data).
// Schemas are inferred from Go types with google/jsonschema-go, the library
// mcp-go already uses, so CLI and MCP schemas come from one place.
//
// Inputs: -dir (default schemas), -check. Specs registered via Register.
// Outputs: schemas/*.json and schemas/index.json (default), or an exit status
// in -check mode: 0 current, 1 differs, 2 generator or I/O failure.
//
// Constraints: -check compares only *.json files. A *.json that is neither
// registered nor the index fails it; a non-JSON file (schemas/embed.go from
// P7-SURF-30) is ignored. Read-only in -check mode.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	dir := flag.String("dir", "schemas", "schemas directory")
	check := flag.Bool("check", false, "verify the committed *.json files are current; exit 1 if not")
	flag.Parse()
	os.Exit(run(defaultSet, *dir, *check, os.Stdout, os.Stderr))
}

// say writes one formatted line; a failed diagnostic write has no recovery.
func say(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

// run executes one generation or check against dir and returns the exit code.
func run(ss *specSet, dir string, check bool, stdout, stderr io.Writer) int {
	if probs := ss.problemList(); len(probs) > 0 {
		for _, p := range probs {
			say(stderr, "schemagen: problem: %s\n", p)
		}
		return 1
	}
	want, err := generate(ss)
	if err != nil {
		say(stderr, "schemagen: %v\n", err)
		return 2
	}
	have, err := readJSONFiles(dir)
	if err != nil {
		say(stderr, "schemagen: %v\n", err)
		return 2
	}
	if check {
		diffs := compare(want, have)
		for _, d := range diffs {
			say(stderr, "schemagen: %s\n", d)
		}
		if len(diffs) > 0 {
			say(stderr, "schemagen: schemas are stale; run `make schemas` and commit the result\n")
			return 1
		}
		say(stdout, "schemagen: %d files current\n", len(want))
		return 0
	}
	if err := write(dir, want, have); err != nil {
		say(stderr, "schemagen: %v\n", err)
		return 2
	}
	say(stdout, "schemagen: wrote %d files to %s\n", len(want), dir)
	return 0
}

// readJSONFiles returns every *.json under dir, keyed by slash path relative
// to dir. A missing dir is an empty set.
func readJSONFiles(dir string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && p == dir {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".json") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = b
		return nil
	})
	return out, err
}

// compare lists missing, changed and extra files, sorted by path.
func compare(want, have map[string][]byte) []string {
	var diffs []string
	for p, w := range want {
		h, ok := have[p]
		switch {
		case !ok:
			diffs = append(diffs, "missing "+p)
		case !bytes.Equal(h, w):
			diffs = append(diffs, "differs "+p)
		}
	}
	for p := range have {
		if _, ok := want[p]; !ok {
			diffs = append(diffs, "extra "+p+" (not generated)")
		}
	}
	sort.Strings(diffs)
	return diffs
}

// write stores every wanted file and removes stale generated *.json files.
func write(dir string, want, have map[string][]byte) error {
	for p := range have {
		if _, ok := want[p]; !ok {
			if err := os.Remove(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
				return err
			}
		}
	}
	for p, b := range want {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}
