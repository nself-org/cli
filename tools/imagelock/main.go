// Command imagelock generates and checks internal/compose/images.lock.json.
//
// Purpose: ADR 0030 / contract:cli.image-lock v1. images.yaml is authored; the
// lock is generated from it (and optionally from the plugins repo's
// images.json) and embedded in the binary.
//
//	imagelock -resolve [-plugins images.json]   network: rewrite the lock
//	imagelock -check                            offline: verify the lock
//
// Inputs: -yaml (default internal/compose/images.yaml), -lock (default
// internal/compose/images.lock.json), -plugins (plugins images.json v1).
// Outputs: the lock file (-resolve) or problems on stderr with exit 1 (-check).
// Constraints: run from the cli repository root; no Go registry client (the
// docker CLI does the reads, through internal/docker).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nself-org/cli/internal/compose"
)

// say writes a formatted line; a failed write to the terminal is not actionable.
func say(w *os.File, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("imagelock", flag.ContinueOnError)
	fs.SetOutput(stderr)
	resolve := fs.Bool("resolve", false, "resolve every image against its registry and rewrite the lock (network)")
	check := fs.Bool("check", false, "verify the lock offline")
	yamlPath := fs.String("yaml", "internal/compose/images.yaml", "authored image list")
	lockPath := fs.String("lock", "internal/compose/images.lock.json", "generated lock")
	plugins := fs.String("plugins", "", "plugins images.json v1 to merge (with -resolve)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *resolve == *check || (*check && *plugins != "") {
		say(stderr, "imagelock: give exactly one of -resolve or -check (-plugins goes with -resolve)\n")
		return 2
	}
	raw, err := os.ReadFile(*yamlPath)
	if err != nil {
		say(stderr, "imagelock: %v\n", err)
		return 1
	}
	sources, err := LoadSources(raw)
	if err != nil {
		say(stderr, "imagelock: %v\n", err)
		return 1
	}
	if *check {
		data, err := os.ReadFile(*lockPath)
		if err != nil {
			say(stderr, "imagelock: %v\n", err)
			return 1
		}
		if problems := Check(data, sources); len(problems) > 0 {
			for _, p := range problems {
				say(stderr, "imagelock: -check: %s\n", p)
			}
			return 1
		}
		say(stdout, "imagelock: lock is current (%d entries from images.yaml)\n", len(sources))
		return 0
	}
	if *plugins != "" {
		pj, err := os.ReadFile(*plugins)
		if err == nil {
			sources, err = MergePlugins(sources, pj)
		}
		if err != nil {
			say(stderr, "imagelock: %v\n", err)
			return 1
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	lf, err := Resolve(ctx, sources, dockerIndexer{})
	if err != nil {
		say(stderr, "imagelock: %v\n", err)
		return 1
	}
	out, err := compose.MarshalLock(lf)
	if err == nil {
		err = os.WriteFile(*lockPath, out, 0o644)
	}
	if err != nil {
		say(stderr, "imagelock: %v\n", err)
		return 1
	}
	say(stdout, "imagelock: wrote %s (%d entries)\n", *lockPath, len(lf.Images))
	return 0
}
