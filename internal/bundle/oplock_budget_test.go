package bundle

// D-0850: the install/remove operation-lock tests re-execute the test binary once
// per bundle member. Under the loaded gate each probe took long enough that the
// old whole-test guard (5s) fired before a five-member bundle finished. The guard
// is only a deadlock net, so it must outlast every probe the tests can run.

import "testing"

// maxOpLockProbes is the most probes one operation-lock test runs: a bundle's
// members plus the final "acquired" probe, with headroom for bigger bundles.
const maxOpLockProbes = 16

func TestOperationLockGuardOutlastsProbes(t *testing.T) {
	if need := maxOpLockProbes * opLockProbeTimeout; opLockGuardTimeout < need {
		t.Fatalf("guard %v fires before %d probes of up to %v each (%v)", opLockGuardTimeout, maxOpLockProbes, opLockProbeTimeout, need)
	}
	if opLockProbeChildTimeout >= opLockProbeTimeout {
		t.Fatalf("child -test.timeout %v must stay under the parent probe timeout %v", opLockProbeChildTimeout, opLockProbeTimeout)
	}
}
