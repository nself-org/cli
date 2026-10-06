// Command servicecatalog renders the nSelf service catalog as markdown.
//
// Purpose:     publish internal/compose's required/optional split as a page
//
//	that is generated, never hand-written, so the docs cannot drift
//	from the generator the way SPORT F02's command count did.
//
// Inputs:      -format markdown|json.
// Outputs:     the catalog on stdout.
// Constraints: reads internal/compose directly — one source of truth, shared
//
//	with `nself service list --core`.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/compose"
)

func main() {
	format := flag.String("format", "markdown", "output format: markdown, json")
	flag.Parse()

	// The page documents the v1.4 default images whatever NSELF_V15 or
	// IMAGE_PINNING the caller has set, so it is the same in both compat modes;
	// the digest-pinned v1.5 references are on Image-Lock.md (P7-LIVE-17).
	_ = os.Setenv(compose.EnvImagePinning, compose.PinningLegacy)
	entries := compose.ServiceCatalog()

	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(entries); err != nil {
			fmt.Fprintf(os.Stderr, "encode: %v\n", err)
			os.Exit(1)
		}
	case "markdown":
		printMarkdown()
	default:
		fmt.Fprintf(os.Stderr, "unknown -format %q\n", *format)
		os.Exit(2)
	}
}

func printMarkdown() {
	fmt.Println("## Required services")
	fmt.Println()
	fmt.Println("Present in every stack. There is no switch to turn these off: without all four,")
	fmt.Println("an nSelf backend cannot serve a request.")
	fmt.Println()
	fmt.Println("| Service | Purpose | Version override | Default image |")
	fmt.Println("|---|---|---|---|")
	for _, e := range compose.CoreServices() {
		fmt.Printf("| `%s` | %s | `%s` | `%s` |\n", e.Name, e.Purpose, e.VersionEnv, e.DefaultImage)
	}

	fmt.Println()
	fmt.Println("## Optional services")
	fmt.Println()
	fmt.Println("Added to the stack only when their enabling variable is `true`.")
	fmt.Println()
	fmt.Println("| Service | Purpose | Enable with | Version override | Default image |")
	fmt.Println("|---|---|---|---|---|")
	for _, e := range compose.OptionalServices() {
		fmt.Printf("| `%s` | %s | `%s=true` | `%s` | `%s` |\n",
			e.Name, e.Purpose, e.EnableEnv, e.VersionEnv, e.DefaultImage)
	}
}
