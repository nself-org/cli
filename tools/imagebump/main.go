// Command imagebump proposes pin bumps for internal/compose/images.yaml.
//
// Purpose: ADR 0030 §4 / cap:cli.image-pin-bump. Pins move only through a
// reviewed weekly PR. For every non-plugin entry of the authored image list,
// imagebump picks the newest registry tag the policy allows
// (.github/image-bump-policy.yaml: regex + track patch|minor|manual), rewrites
// that entry's `version`, then runs `go run ./tools/imagelock -resolve` so the
// generated lock follows. Plugin entries are skipped: their pins move with
// plugin releases.
//
//	imagebump [--dry-run] [--policy <file>] [--lock <images.yaml>]
//
// Inputs: the policy, the authored list (--lock, images.yaml format), tags from
// `scripts/images/mirror-images.sh --list-tags <repository>` (a digest-pinned
// crane container; no Go registry client). A --lock file with a top-level
// `fixture_tags: {<repository>: [tags]}` map takes tags from there instead;
// that is only allowed with --dry-run.
// Outputs: stdout, one line per proposed bump sorted by name:
//
//	BUMP <name> <old> -> <new>
//
// Diagnostics go to stderr. Without --dry-run the changed `version` values are
// rewritten in place (every other byte kept) and imagelock -resolve always runs,
// even with zero bumps, so floating tags refresh their digests.
// Exit: 0 ok; 1 failed (policy gap, version outside its regex, registry read,
// rewrite, resolve); 2 usage or refused input.
// Constraints: run from the cli repository root.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// mirrorScript is the registry reader (crane in a pinned container).
const mirrorScript = "scripts/images/mirror-images.sh"

// deps are the external effects, replaceable in tests.
type deps struct {
	lister  TagLister
	resolve func(yamlPath string, out io.Writer) int
}

func say(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// run is the production entry point: real registry reads, real imagelock.
func run(args []string, stdout, stderr io.Writer) int {
	return execute(args, stdout, stderr, deps{lister: &execLister{script: mirrorScript}, resolve: runResolve})
}

// runResolve runs the lock generator. Its output goes to out (stderr) so stdout
// stays BUMP lines only. Returns the generator's exit code.
func runResolve(yamlPath string, out io.Writer) int {
	cmd := exec.Command("go", "run", "./tools/imagelock", "-resolve", "-yaml", yamlPath)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		say(out, "imagebump: cannot run imagelock: %v\n", err)
		return 1
	}
	return 0
}

// execute parses flags, plans the bumps, prints them and (unless --dry-run)
// applies them. d supplies the registry reader and the lock generator.
func execute(args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("imagebump", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dry := fs.Bool("dry-run", false, "print proposed bumps; write nothing")
	policyPath := fs.String("policy", ".github/image-bump-policy.yaml", "bump policy")
	lockPath := fs.String("lock", "internal/compose/images.yaml", "authored image list (images.yaml format)")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return 2
	}
	raw, err := os.ReadFile(*lockPath)
	if err != nil {
		say(stderr, "imagebump: %v\n", err)
		return 1
	}
	lf, err := parseLockFile(raw)
	if err != nil {
		say(stderr, "imagebump: %s: %v\n", *lockPath, err)
		return 1
	}
	lister := d.lister
	if lf.FixtureTags != nil {
		if !*dry {
			say(stderr, "imagebump: fixture_tags in %s is refused without --dry-run\n", *lockPath)
			return 2
		}
		lister = fixtureLister(lf.FixtureTags)
	}
	praw, err := os.ReadFile(*policyPath)
	if err != nil {
		say(stderr, "imagebump: %v\n", err)
		return 1
	}
	pol, err := loadPolicy(praw)
	if err != nil {
		say(stderr, "imagebump: %s: %v\n", *policyPath, err)
		return 1
	}
	bumps, problems := planBumps(lf.Images, pol, newCachedLister(lister), stderr)
	for _, p := range problems {
		say(stderr, "imagebump: %v\n", p)
	}
	if len(problems) > 0 {
		return 1
	}
	for _, b := range bumps {
		say(stdout, "BUMP %s %s -> %s\n", b.Name, b.Old, b.New)
	}
	say(stderr, "imagebump: %d bump(s) proposed\n", len(bumps))
	if *dry {
		return 0
	}
	if len(bumps) > 0 {
		out, err := rewriteVersions(raw, bumps)
		if err != nil {
			say(stderr, "imagebump: %v\n", err)
			return 1
		}
		if err := writeKeepMode(*lockPath, out); err != nil {
			say(stderr, "imagebump: %v\n", err)
			return 1
		}
	}
	return d.resolve(*lockPath, stderr)
}

// writeKeepMode replaces path's content, keeping its file mode.
func writeKeepMode(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	return os.WriteFile(path, data, mode)
}
