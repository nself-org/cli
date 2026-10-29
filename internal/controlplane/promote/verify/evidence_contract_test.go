package verify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goldenRunner replays a golden captured from the real ci plugin
// (testdata/README.md) as the plugin's stdout with the given exit status.
func goldenRunner(t *testing.T, name string, exit int) EvidenceRunner {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return func(context.Context, string, string) ([]byte, int, error) { return raw, exit, nil }
}

// D-1217: the report keys read here (data.class, data.result, data.quarantined,
// error.message) are the ones the real plugin prints.
func TestEvidenceReaderMatchesPluginGoldens(t *testing.T) {
	ctx := context.Background()
	t.Run("keys decode from the real report", func(t *testing.T) {
		for _, name := range []string{"evidence-verify-pass.golden.json", "evidence-verify-release-class.golden.json"} {
			raw, err := os.ReadFile(filepath.Join("testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			var rep evidenceReport
			if err := json.Unmarshal(raw, &rep); err != nil || rep.Data.Class == "" || rep.Data.Result == "" || rep.Data.Quarantined == nil {
				t.Errorf("%s: class %q result %q quarantined %v: a key the reader needs is missing", name, rep.Data.Class, rep.Data.Result, rep.Data.Quarantined)
			}
		}
	})
	t.Run("a passing default-class report is refused for its class", func(t *testing.T) {
		err := checkEvidence(ctx, goldenRunner(t, "evidence-verify-pass.golden.json", 0), "evidence.json", "rev")
		if err == nil || !strings.Contains(err.Error(), `class "default" result "pass"`) {
			t.Fatalf("err = %v, want E497 naming class default result pass", err)
		}
	})
	t.Run("a release-class passing report is accepted when the plugin exits 0", func(t *testing.T) {
		if err := checkEvidence(ctx, goldenRunner(t, "evidence-verify-release-class.golden.json", 0), "evidence.json", "rev"); err != nil {
			t.Fatalf("err = %v, want nil: class/result/quarantined were not read from the real report", err)
		}
	})
	t.Run("a refusal carries the plugin's error message", func(t *testing.T) {
		err := checkEvidence(ctx, goldenRunner(t, "evidence-verify-rejected.golden.json", 11), "evidence.json", "rev")
		if codeOf(err) != "E497" || !strings.Contains(err.Error(), "evidence rejected") {
			t.Fatalf("err = %v, want E497 with the plugin's message", err)
		}
	})
}
