package verify

// Purpose: pin the three keys of the ci plugin's `evidence verify --json`
//          report that promote reads (data.class, data.result,
//          data.quarantined) against bytes the real plugin printed, not a
//          hand-written fixture (debt D-1217).
// Constraints: testdata/ci-evidence-verify-release.golden.json was printed by
//              plugins free/ci (cmd/evidence.go printVerdict via
//              output.WriteData) at plugins main 18b2e707; to refresh it,
//              rebuild the ci plugin, run `evidence verify <rev> --json` on a
//              release-class run and copy stdout.

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestEvidenceReportContractGolden(t *testing.T) {
	golden, err := os.ReadFile("testdata/ci-evidence-verify-release.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	// The raw keys are named here so a plugin rename fails this test instead
	// of turning every promote into E497.
	var raw struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(golden, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"class", "result", "quarantined"} {
		if _, ok := raw.Data[key]; !ok {
			t.Fatalf("plugin report has no data.%s; promote would refuse every release", key)
		}
	}
	run := func(context.Context, string, string) ([]byte, int, error) { return golden, 0, nil }
	if err := checkEvidence(context.Background(), run, "evidence.json", testRev); err != nil {
		t.Fatalf("the real plugin's passing release report was refused: %v", err)
	}
}
