// JSON data-type registration for the db_vector canon fragment
// (`db generate vector-search`, P7-ADOPT-07).
//
// Purpose: the v1 envelope data of the vector-search generator is
// vectorsearch.Result: the function, what was written, whether it is tracked
// and what happened to the on-disk metadata.
// Constraints: registers from init(); see registry_types.go for the rules.

package commands

import "github.com/nself-org/cli/internal/database/vectorsearch"

func init() {
	registerJSONType("db generate vector-search", vectorsearch.Result{})
}
