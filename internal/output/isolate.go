package output

import "os"

// isolatedStdout is the document stream while human writers are redirected.
// Commands run serially in one CLI process; the decorator restores this state
// before another invocation starts.
var isolatedStdout *os.File

// IsolateStdout redirects incidental stdout to stderr until restore is called.
// The first call retains the real stdout for Default; nested calls are no-ops.
func IsolateStdout() (restore func()) {
	if isolatedStdout != nil {
		return func() {}
	}
	isolatedStdout = os.Stdout
	os.Stdout = os.Stderr
	return func() {
		if isolatedStdout == nil {
			return
		}
		os.Stdout = isolatedStdout
		isolatedStdout = nil
	}
}
