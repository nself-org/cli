// Command catalog generates registry.json, catalog.json and counts.json from
// plugin manifests, releases.json and bundles.json (contract:plugins.catalog v1).
//
// Purpose: the one generator of the plugin projections (Epic P7-PLUG D3). Run it
// in a plugin repo pinned to a CLI tag:
//
//	go run github.com/nself-org/cli/tools/catalog@<cli tag> -mode free ...
//
// Inputs: see README.md; flags -mode free|licensed|catalog, -plugins, -releases,
// -bundles, -peer-registry, -free-registry, -licensed-registry, -out, -check.
// Outputs: free and licensed write registry.json; catalog writes catalog.json
// and counts.json. Exit 0 ok, 1 when -check finds a difference, 2 on bad input
// or failure. Output is deterministic: no clock, sorted keys, 2-space indent.
// Constraints: imports only internal/plugin/manifestv2, the model package and
// the standard library, so it runs from a module download with no network.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/tools/catalog/model"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// options holds the parsed flags.
type options struct {
	mode, plugins, releases, bundles, peer, freeReg, licReg, out string
	check, partial                                               bool
	from                                                         model.GeneratedFrom
}

func say(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

func parseFlags(args []string, stderr io.Writer) (*options, bool) {
	o := &options{}
	fs := flag.NewFlagSet("catalog", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.mode, "mode", "", "free | licensed | catalog")
	fs.StringVar(&o.plugins, "plugins", "", "free or paid tree root (one directory per plugin)")
	fs.StringVar(&o.releases, "releases", "", "releases.json")
	fs.StringVar(&o.bundles, "bundles", "", "bundles.json")
	fs.StringVar(&o.peer, "peer-registry", "", "the other tier's registry.json (sets tier_pair)")
	fs.StringVar(&o.freeReg, "free-registry", "", "free registry.json (catalog mode)")
	fs.StringVar(&o.licReg, "licensed-registry", "", "licensed registry.json (catalog mode)")
	fs.StringVar(&o.out, "out", "", "output directory")
	fs.BoolVar(&o.check, "check", false, "write nothing; exit 1 when an output differs")
	fs.BoolVar(&o.partial, "partial", false, "diagnostic: write what loads, list the rest, still exit 2")
	fs.StringVar(&o.from.PluginsRef, "plugins-ref", "", "plugins repo ref recorded in generated_from")
	fs.StringVar(&o.from.BundlesRef, "bundles-ref", "", "bundles repo ref recorded in generated_from")
	fs.StringVar(&o.from.GeneratedAt, "generated-at", "", "RFC 3339 time recorded in generated_from (default SOURCE_DATE_EPOCH, else the Unix epoch)")
	if fs.Parse(args) != nil {
		return nil, false
	}
	at, err := generatedAt(o.from.GeneratedAt, os.Getenv("SOURCE_DATE_EPOCH"))
	if err != nil {
		say(stderr, "catalog: %v\n", err)
		return nil, false
	}
	o.from.GeneratedAt = at
	return o, true
}

// generatedAt resolves the one timestamp: the flag, else SOURCE_DATE_EPOCH,
// else the Unix epoch. It never reads a clock.
func generatedAt(flagVal, epoch string) (string, error) {
	if flagVal != "" {
		if _, err := time.Parse(time.RFC3339, flagVal); err != nil {
			return "", fmt.Errorf("-generated-at %q is not RFC 3339", flagVal)
		}
		return flagVal, nil
	}
	n := int64(0)
	if epoch != "" {
		v, err := strconv.ParseInt(epoch, 10, 64)
		if err != nil {
			return "", fmt.Errorf("SOURCE_DATE_EPOCH %q is not an integer", epoch)
		}
		n = v
	}
	return time.Unix(n, 0).UTC().Format(time.RFC3339), nil
}

func run(args []string, stdout, stderr io.Writer) int {
	o, ok := parseFlags(args, stderr)
	if !ok {
		return 2
	}
	if o.out == "" {
		say(stderr, "catalog: -out is required\n")
		return 2
	}
	var out outputs
	var probs []string
	var err error
	switch o.mode {
	case "free":
		out, probs, err = runTier(o, freeTier, stderr)
	case "licensed":
		out, probs, err = runTier(o, licensedTier, stderr)
	case "catalog":
		out, probs, err = runCatalog(o)
	default:
		say(stderr, "catalog: -mode must be free, licensed or catalog\n")
		return 2
	}
	if err != nil {
		say(stderr, "catalog: %v\n", err)
		return 2
	}
	for _, p := range probs {
		say(stderr, "catalog: problem: %s\n", p)
	}
	if len(probs) > 0 && !o.partial {
		return 2
	}
	if o.check {
		diffs := out.differing(o.out)
		for _, d := range diffs {
			say(stderr, "catalog: %s differs from what would be written\n", d)
		}
		if len(diffs) > 0 {
			return 1
		}
		say(stdout, "catalog: %s current\n", strings.Join(out.names(), ", "))
		return 0
	}
	if err := out.write(o.out); err != nil {
		say(stderr, "catalog: %v\n", err)
		return 2
	}
	say(stdout, "catalog: wrote %s to %s\n", strings.Join(out.names(), ", "), o.out)
	if len(probs) > 0 {
		return 2
	}
	return 0
}
