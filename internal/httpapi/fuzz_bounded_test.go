package httpapi

// Purpose: prove the shared fuzz server keeps constant state per input (D-1114).
// Inputs: many fuzz bodies run in one process. Outputs: TestFuzzServerRetainsNothingAcrossInputs.
// Constraints: the fuzz server is process-wide, so this reads its recorders directly.

import "testing"

// TestFuzzServerRetainsNothingAcrossInputs runs both fuzz bodies many times and
// checks that neither the invocation history nor the log grows with the number
// of inputs, which is what made long fuzz runs leak and seen() copy O(n).
func TestFuzzServerRetainsNothingAcrossInputs(t *testing.T) {
	checkRoutePath(t, "/v1/commands/config/get")
	checkHostHeader(t, "localhost:3857")
	oneLog := len(fuzzLog.String())
	for i := 0; i < 200; i++ {
		checkRoutePath(t, "/v1/commands/config/get")
		checkHostHeader(t, "localhost:3857")
	}
	if n := fuzzInv.count(); n > 1 {
		t.Fatalf("fuzz invoker retains %d invocations after 400 inputs, want at most the last input's", n)
	}
	if n := len(fuzzLog.String()); n > 2*oneLog+1 {
		t.Fatalf("fuzz log holds %d bytes after 400 inputs, one input wrote %d", n, oneLog)
	}
}
