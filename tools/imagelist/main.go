package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/compose"
)

func run(args []string, stdout io.Writer, images []compose.Ref) error {
	fs := flag.NewFlagSet("imagelist", flag.ContinueOnError)
	platformsFlag := fs.Bool("platforms", false, "print name, ref and platforms (TSV)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if len(images) == 0 {
		return fmt.Errorf("imagelist: the image lock is empty")
	}

	for _, r := range images {
		if *platformsFlag {
			var plats []string
			for p := range r.Platforms {
				plats = append(plats, p)
			}
			sort.Strings(plats)
			_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\n", r.Name, r.String(), strings.Join(plats, ","))
		} else {
			_, _ = fmt.Fprintf(stdout, "%s\t%s\n", r.Name, r.String())
		}
	}
	return nil
}

func main() {
	if err := compose.LockError(); err != nil {
		fmt.Fprintln(os.Stderr, "imagelist:", err)
		os.Exit(1)
	}
	if err := run(os.Args[1:], os.Stdout, compose.LockedImages()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
