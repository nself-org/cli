package manifestv2

import (
	"io"
	"sync"
)

// ResetWarn re-arms the once-per-process deprecation notice and redirects it.
func ResetWarn(w io.Writer) {
	warnOnce = sync.Once{}
	warnWriter = w
}
