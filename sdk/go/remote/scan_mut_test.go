package remote

// Purpose: mutation-killing tests for scan.go: the exact port boundaries of
// ScanHostKeys. A stub ssh-keyscan on PATH; no network.

import (
	"context"
	"strconv"
	"testing"
)

func TestScanHostKeys_PortBoundaries(t *testing.T) {
	log := stubTools(t, map[string]string{"ssh-keyscan": "echo '203.0.113.7 " + keyA + "'"})
	for _, p := range []int{1, 2, 65534, 65535} {
		if _, err := ScanHostKeys(context.Background(), "203.0.113.7", p); err != nil {
			t.Errorf("port %d refused: %v", p, err)
		}
	}
	calls := readCalls(t, log)
	if len(calls) != 4 {
		t.Fatalf("%d scans, want 4", len(calls))
	}
	for i, p := range []int{1, 2, 65534, 65535} {
		if got := calls[i].args[3]; got != strconv.Itoa(p) {
			t.Errorf("call %d port arg = %q, want %d", i, got, p)
		}
	}
	n := countExecs(t)
	for _, p := range []int{0, -1, 65536, 70000} {
		if _, err := ScanHostKeys(context.Background(), "203.0.113.7", p); err == nil {
			t.Errorf("port %d accepted", p)
		}
	}
	if *n != 0 {
		t.Fatalf("%d execs for refused ports", *n)
	}
}
