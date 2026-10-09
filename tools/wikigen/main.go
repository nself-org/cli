// Command wikigen generates one wiki page per top-level CLI command.
//
// Purpose: the wiki had cmd-*.md pages for most but not all commands, written by
// hand at different times, so flag tables and subcommand lists drifted from the
// binary and new commands shipped undocumented. Everything cobra already knows
// (title, synopsis, flags with defaults, subcommands, nav) is now generated; the
// parts that need a human (description, examples, see-also) live in PROSE blocks
// that regeneration never touches.
//
// Pages live at the wiki root, not in commands/: GitHub Wiki flattens the
// namespace, so .github/wiki/cmd-db.md and .github/wiki/commands/cmd-db.md would
// be the same published page.
//
// Mode: documents the v1.5 surface. main sets NSELF_V15=1 in its own process
// (compat.V15 reads the environment on every call) and calls
// commands.PrepareTreeForGeneration, so the tree is the relocated v1.5 tree with
// builtin families only, never an installed plugin. Depth-1 commands the canon
// moved keep their old page as a stub (stub.go).
//
// Inputs: -dir (wiki commands directory), -check (verify, write nothing),
// -report (list pages still carrying placeholder prose).
//
// Outputs: cmd-<name>.md per command; exit 1 under -check when stale.
//
// Constraints: never discards human prose. A page's PROSE blocks are read back
// and re-emitted verbatim; pages predating the markers have their Description,
// Examples and See Also sections migrated into blocks.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/cmd/commands"
	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

func main() {
	dir := flag.String("dir", ".github/wiki", "wiki directory holding cmd-*.md pages")
	sidebar := flag.String("sidebar", ".github/wiki/_Sidebar.md", "wiki sidebar to keep complete")
	llms := flag.String("llms", ".github/wiki/llms.txt", "AI-consumable single-file command reference")
	check := flag.Bool("check", false, "verify pages are current; write nothing")
	report := flag.Bool("report", false, "list pages still carrying placeholder prose")
	flag.Parse()

	registry, undo, err := prepareWikiRegistry()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer undo()
	cmds := topLevelCommands()
	if len(cmds) == 0 {
		fmt.Fprintln(os.Stderr, "no commands found")
		os.Exit(1)
	}

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", *dir, err)
		os.Exit(1)
	}

	var stale, placeholders []string
	pagesWritten := 0
	auxWritten := 0

	for _, c := range cmds {
		entry, ok := registry[c.CommandPath()]
		if !ok {
			fmt.Fprintf(os.Stderr, "wiki registry lacks %s\n", c.CommandPath())
			os.Exit(1)
		}
		path := filepath.Join(*dir, pageName(c.Name()))
		existing := readExisting(*dir, c.Name())
		page := renderPage(c, entry, registry, existing)

		if hasPlaceholder(page) {
			placeholders = append(placeholders, pageName(c.Name()))
		}

		if !pageStale(path, page) {
			continue
		}
		if *check {
			stale = append(stale, pageName(c.Name()))
			continue
		}
		if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		pagesWritten++
	}

	raw, err := canon.LoadRaw()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load canon: %v\n", err)
		os.Exit(1)
	}
	stubStale, stubsWritten, err := writeStubs(*dir, stubRows(raw.Rows), *check)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	stale = append(stale, stubStale...)
	pagesWritten += stubsWritten

	errCodesChanged, err := writeErrorCodesPage(filepath.Join(*dir, "error-codes.md"), errs.Registry, *check)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error-codes.md: %v\n", err)
		os.Exit(1)
	}
	if errCodesChanged {
		if *check {
			stale = append(stale, "error-codes.md")
		} else {
			auxWritten++
		}
	}

	exitCodesChanged, err := writeExitCodesPage(filepath.Join(*dir, "Exit-Codes.md"), *check)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Exit-Codes.md: %v\n", err)
		os.Exit(1)
	}
	if exitCodesChanged {
		if *check {
			stale = append(stale, "Exit-Codes.md")
		} else {
			auxWritten++
		}
	}

	taxonomyChanged, err := writeStatusTaxonomyPage(filepath.Join(*dir, "Status-Taxonomy.md"), *check)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Status-Taxonomy.md: %v\n", err)
		os.Exit(1)
	}
	if taxonomyChanged {
		if *check {
			stale = append(stale, "Status-Taxonomy.md")
		} else {
			auxWritten++
		}
	}

	sidebarChanged, err := writeSidebar(*sidebar, cmds, *check)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if sidebarChanged {
		if *check {
			stale = append(stale, filepath.Base(*sidebar))
		} else {
			auxWritten++
		}
	}

	llmsChanged, err := writeLLMsTxt(*llms, cmds, *check)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if llmsChanged {
		if *check {
			stale = append(stale, filepath.Base(*llms))
		} else {
			auxWritten++
		}
	}

	if *check {
		if len(stale) > 0 {
			fmt.Fprintf(os.Stderr, "%d wiki command page(s) are stale — run `make wiki-commands`:\n  %s\n",
				len(stale), strings.Join(stale, "\n  "))
			os.Exit(1)
		}
		fmt.Printf("All %d wiki command pages are current.\n", len(cmds))
	} else {
		fmt.Printf("%d command pages checked, %d written, %d already current; %d index file(s) updated.\n",
			len(cmds), pagesWritten, len(cmds)-pagesWritten, auxWritten)
	}

	if *report {
		sort.Strings(placeholders)
		fmt.Printf("\n%d of %d pages still need human prose:\n", len(placeholders), len(cmds))
		for _, p := range placeholders {
			fmt.Println("  " + p)
		}
	}
}

// topLevelCommands returns the visible top-level commands, sorted by name.
func topLevelCommands() []*cobra.Command {
	var out []*cobra.Command
	for _, c := range commands.RootCmd.Commands() {
		if c.Hidden || c.Name() == "help" {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// pageName is the canonical filename for a command page.
func pageName(cmd string) string { return "cmd-" + cmd + ".md" }

// pageStale is the same comparison used by -check and by the fixture tests.
func pageStale(path, expected string) bool {
	current, err := os.ReadFile(path)
	return err != nil || string(current) != expected
}

// legacyPageNames lists the filenames a page may have had before the naming
// convention settled on cmd-<name>.md, newest first.
func legacyPageNames(cmd string) []string {
	return []string{cmd + ".md"}
}
