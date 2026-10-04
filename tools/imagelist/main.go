// Command imagelist prints every default service image `nself build` can emit.
//
// Purpose: give CI a single, authoritative list to check for pullability
// without restating the images anywhere. It reads them straight from the
// image lock (compose.LockedImages), so a new or changed pin is covered the
// moment it lands in internal/compose/images.yaml and the lock — there is no
// second list to forget to update.
//
// Inputs: none.
//
// Outputs: one "<service>\t<repository:version@digest>" line per lock entry on
// stdout, sorted by service name so the output is stable and diffable.
//
// Constraints: prints the lock references (the digest form), so the gate also
// proves each pinned digest still exists upstream. Images a user selects
// through env (POSTGRES_IMAGE, a custom CS_N_IMAGE) are that operator's choice
// and are deliberately out of scope.
//
// SPORT: consumed by .github/workflows/default-images-pullable.yml
package main

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/compose"
)

func main() {
	if err := compose.LockError(); err != nil {
		fmt.Fprintln(os.Stderr, "imagelist:", err)
		os.Exit(1)
	}
	images := compose.LockedImages()
	for _, r := range images {
		fmt.Printf("%s\t%s\n", r.Name, r)
	}
	if len(images) == 0 {
		fmt.Fprintln(os.Stderr, "imagelist: the image lock is empty")
		os.Exit(1)
	}
}
