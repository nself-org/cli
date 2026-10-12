// Purpose: ADR 0021 gate for the pin store's ancestor walk (D-0675).
// Inputs:  the pin dir and the error from checkAncestors / checkBeforeMkdir.
// Outputs: the error under compat.V15; otherwise nil plus one warning line
//          per pin dir per process on ancestorWarnOut (stderr).
// Constraints: v1.4.13 had no ancestor walk, so on v1.4 a group/world
//              writable or foreign-owned folder above the pin dir must not
//              stop a backup or deploy that worked before. The pin dir's
//              own owner and mode check (checkDir) is not gated here. Every
//              walk error is gated, not only a policy refusal: a Stat or
//              EvalSymlinks error on an ancestor also refuses under V15 and
//              only warns on v1.4, because v1.4.13 never walked at all.

package remote

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/nself-org/cli/sdk/go/v2/compat"
)

// ancestorWarnOut receives the v1.4 warning. A var so tests can capture it.
var ancestorWarnOut io.Writer = os.Stderr

// ancestorWarned records the pin dirs already warned about in this process.
var (
	ancestorWarnMu sync.Mutex
	ancestorWarned = map[string]bool{}
)

// ancestorGate returns err under v1.5. On v1.4 it warns once for dir and
// returns nil, so the operation goes on as it did on v1.4.13.
func ancestorGate(dir string, err error) error {
	if err == nil {
		return nil
	}
	// compat.V15(D-0484): pin store ancestor walk warns once and continues -> refuses
	if compat.V15() {
		return err
	}
	ancestorWarnMu.Lock()
	defer ancestorWarnMu.Unlock()
	if !ancestorWarned[dir] {
		ancestorWarned[dir] = true
		_, _ = fmt.Fprintf(ancestorWarnOut, "warning: %v (allowed for now; nself v1.5 refuses it)\n", err)
	}
	return nil
}
