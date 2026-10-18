// Command facts writes and checks the public facts artifact
// (.github/facts.json, contract:cli.public-facts v1) and fills the README facts
// blocks.
//
// Purpose: one small, versioned projection of .github/VERSION,
// .github/command-registry.json and (when present) .github/openapi.json that
// sites and sweeps read, so a public number cannot disagree with
// `nself help --json`.
//
// Inputs: -root (repository root, default "."), -check, -readme <file>.
// Outputs: default writes <root>/.github/facts.json; `-readme F` also rewrites
// the facts blocks of F. With -check nothing is written: exit 0 current, 1 a
// file is stale (the message names it), 2 a source is unreadable or invalid.
//
// Constraints: deterministic, no timestamps. With -check -readme, both
// facts.json and the README blocks are verified. A README without facts blocks
// is a no-op.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const factsPath = ".github/facts.json"

func main() {
	root := flag.String("root", ".", "repository root")
	check := flag.Bool("check", false, "verify files are current; exit 1 if not")
	readme := flag.String("readme", "", "README file whose GENERATED:facts blocks to fill (or verify with -check)")
	flag.Parse()
	os.Exit(run(*root, *check, *readme, os.Stdout, os.Stderr))
}

func say(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

// readOptional returns nil, nil for a missing file.
func readOptional(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// run executes one write or check and returns the exit code.
func run(root string, check bool, readme string, stdout, stderr io.Writer) int {
	gh := filepath.Join(root, ".github")
	version, err := os.ReadFile(filepath.Join(gh, "VERSION"))
	if err != nil {
		say(stderr, "facts: %v\n", err)
		return 2
	}
	registry, err := os.ReadFile(filepath.Join(gh, "command-registry.json"))
	if err != nil {
		say(stderr, "facts: %v\n", err)
		return 2
	}
	openapi, err := readOptional(filepath.Join(gh, "openapi.json"))
	if err != nil {
		say(stderr, "facts: %v\n", err)
		return 2
	}
	f, err := Build(version, registry, openapi)
	if err != nil {
		say(stderr, "facts: %v\n", err)
		return 2
	}
	want, err := Marshal(f)
	if err != nil {
		say(stderr, "facts: %v\n", err)
		return 2
	}
	target := filepath.Join(root, factsPath)
	stale := false
	have, err := readOptional(target)
	if err != nil {
		say(stderr, "facts: %v\n", err)
		return 2
	}
	if !bytes.Equal(have, want) {
		if check {
			say(stderr, "facts: %s is stale; run `make cmd-inventory` and commit the result\n", factsPath)
			stale = true
		} else if err := os.WriteFile(target, want, 0o644); err != nil {
			say(stderr, "facts: %v\n", err)
			return 2
		}
	}
	if readme != "" {
		old, err := os.ReadFile(readme)
		if err != nil {
			say(stderr, "facts: %v\n", err)
			return 2
		}
		filled, err := FillReadme(string(old), f)
		if err != nil {
			say(stderr, "facts: %s: %v\n", readme, err)
			return 2
		}
		if filled != string(old) {
			if check {
				say(stderr, "facts: %s has stale facts blocks; run `go run ./tools/facts -readme %s`\n", readme, readme)
				stale = true
			} else if err := os.WriteFile(readme, []byte(filled), 0o644); err != nil {
				say(stderr, "facts: %v\n", err)
				return 2
			}
		}
	}
	if stale {
		return 1
	}
	if check {
		say(stdout, "facts: current\n")
	} else {
		say(stdout, "facts: wrote %s\n", factsPath)
	}
	return 0
}
