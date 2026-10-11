package commands

// Purpose: Implements `nself urls diff <envA> <envB>` — loads config for
// two environments and compares their resolved URLs side by side, plus the
// urlMap helper that flattens a urlsOutput into a name->url map for that
// comparison. Split out of urls.go (CLI-R12) to separate the diff
// subcommand from the cobra entry point (urls.go), the output builder
// (urls_build.go), and the conflict/display printers (urls_display.go).
// Inputs: a workdir and two environment names, plus a jsonOut flag.
// Outputs: a printed (or JSON) side-by-side URL diff.
// Constraints: pure move — no behavior changes.

import (
	"fmt"
	"os"
	"sort"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/ui"
)

// runURLsDiff loads two environments and compares their URLs side-by-side.
func runURLsDiff(workdir, envA, envB string, jsonOut bool) error {
	if envA == "" {
		envA = "dev"
	}

	// Load environment A.
	_ = os.Setenv("ENV", envA)
	cfgA, err := config.Load(workdir)
	if err != nil {
		return fmt.Errorf("loading env %q: %w", envA, err)
	}
	outA := buildURLOutput(cfgA, true)

	// Load environment B.
	_ = os.Setenv("ENV", envB)
	cfgB, err := config.Load(workdir)
	if err != nil {
		return fmt.Errorf("loading env %q: %w", envB, err)
	}
	outB := buildURLOutput(cfgB, true)

	if jsonOut {
		// Bare v1.4 shape: one listing per environment name. The envelope
		// data is the first listing plus `compared` (P7-SURF-11).
		diff := map[string]urlsOutput{envA: outA, envB: outB}
		return emitStateJSON("status urls", diff, urlsData{urlsOutput: outA, Compared: &urlsCompared{Env: envB, Listing: outB}})
	}

	// Build maps for comparison.
	mapA := urlMap(outA)
	mapB := urlMap(outB)

	// Collect all service names.
	nameSet := make(map[string]bool)
	for k := range mapA {
		nameSet[k] = true
	}
	for k := range mapB {
		nameSet[k] = true
	}
	names := make([]string, 0, len(nameSet))
	for k := range nameSet {
		names = append(names, k)
	}
	sort.Strings(names)

	fmt.Printf("%-20s  %-35s  %s\n",
		ui.C(ui.Bold, "Service"),
		ui.C(ui.Bold, envA),
		ui.C(ui.Bold, envB),
	)
	ui.Separator()

	for _, name := range names {
		a := mapA[name]
		b := mapB[name]
		if a == b {
			fmt.Printf("%-20s  %-35s  %s\n", name, a, b)
		} else {
			aDisp := a
			bDisp := b
			if a == "" {
				aDisp = ui.C(ui.Dim, "(none)")
			}
			if b == "" {
				bDisp = ui.C(ui.Dim, "(none)")
			}
			fmt.Printf("%-20s  %-35s  %s  %s\n",
				ui.C(ui.Yellow, name),
				aDisp,
				bDisp,
				ui.C(ui.Yellow, ui.IconWarning),
			)
		}
	}

	return nil
}

// urlMap flattens all URL entries into a name->url map for diff comparison.
func urlMap(out urlsOutput) map[string]string {
	m := make(map[string]string)
	for _, groups := range [][]serviceURL{
		out.RequiredServices,
		out.OptionalServices,
		out.CustomServices,
		out.FrontendApps,
		out.InternalRoutes,
	} {
		for _, e := range groups {
			m[e.Name] = e.URL
		}
	}
	return m
}
