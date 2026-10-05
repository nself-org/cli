package repoqa

import "testing"

// httpBasisTotal is the hard-coded total of testdata/http-client-allowlist.txt
// (rule D5). The allowlist may only shrink: lowering a line without lowering
// this constant fails, and raising either needs a reviewed edit of both.
const httpBasisTotal = 56

// TestHTTPFunnel freezes direct http.Client construction outside
// internal/httptimeout (EPIC G1, G4; debt D-0034).
func TestHTTPFunnel(t *testing.T) {
	runFunnelRatchet(t, funnelSpec{
		kind: "http", funnel: httpFunnelDir, list: "testdata/http-client-allowlist.txt",
		basisName: "httpBasisTotal", basisFile: "http_funnel_test.go", basis: httpBasisTotal,
		pick: func(h, _ countList) countList { return h },
	})
}
