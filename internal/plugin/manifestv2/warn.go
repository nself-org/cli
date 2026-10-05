package manifestv2

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/nself-org/cli/internal/compat"
)

var (
	warnOnce   sync.Once
	warnWriter io.Writer = os.Stderr
)

// noteV1 prints the v1 deprecation notice once per process, only in v1.5 mode.
func noteV1(name string) {
	// compat.V15(P7-PLUG-01): v1 plugin.json read silently -> one deprecation line per process on stderr
	if !compat.V15() {
		return
	}
	warnOnce.Do(func() {
		_, _ = fmt.Fprintf(warnWriter, "nself: plugin manifest v1 is deprecated (first seen: %s); v1 is read until v1.6.0. Convert: go run github.com/nself-org/cli/tools/manifestv2migrate -in plugin.json -write\n", name)
	})
}
