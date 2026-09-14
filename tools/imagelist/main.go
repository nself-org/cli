// Command imagelist prints every default service image `nself build` can emit.
//
// Purpose: give CI a single, authoritative list to check for pullability
// without restating the images anywhere. It reads them straight from
// compose.DefaultImageVersions, so a new or changed pin is covered the moment
// it lands — there is no second list to forget to update.
//
// Inputs: none.
//
// Outputs: one "<service>\t<image:tag>" line per service on stdout, sorted by
// service name so the output is stable and diffable.
//
// Constraints: prints only the DEFAULT pins. Images a user selects through env
// (POSTGRES_IMAGE, a custom CS_N_IMAGE) are that operator's choice and are
// deliberately out of scope.
//
// SPORT: consumed by .github/workflows/default-images-pullable.yml
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/nself-org/cli/internal/compose"
)

func main() {
	services := make([]string, 0, len(compose.DefaultImageVersions))
	for service := range compose.DefaultImageVersions {
		services = append(services, service)
	}
	sort.Strings(services)

	for _, service := range services {
		fmt.Printf("%s\t%s\n", service, compose.DefaultImageVersions[service])
	}

	if len(services) == 0 {
		fmt.Fprintln(os.Stderr, "imagelist: DefaultImageVersions is empty")
		os.Exit(1)
	}
}
