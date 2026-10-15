package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestManifestFindingsOmitScalarValues pins privacy at the validator boundary:
// both APIs and both renderings retain diagnostics without echoing values.
func TestManifestFindingsOmitScalarValues(t *testing.T) {
	cases := []struct {
		name, yaml, secret, path, tag string
	}{
		{"root", "fake-root-secret\n", "fake-root-secret", "", "str"},
		{"map", "plugins: fake-map-secret\n", "fake-map-secret", "plugins", "str"},
		{"list", "bundles: fake-list-secret\n", "fake-list-secret", "bundles", "str"},
		{"block", "plugins: |\n  fake-block-secret\n  second-line\n", "fake-block-secret", "plugins", "str"},
		{"custom tag", "app: !private fake-tag-secret\n", "fake-tag-secret", "app", "!private"},
		{"number", "app: 8675309123\n", "8675309123", "app", "int"},
		{"merge", "<<: fake-merge-secret\n", "fake-merge-secret", "", "str"},
	}
	compattest.Both(t, func(t *testing.T) {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				file := filepath.Join(t.TempDir(), "nself.yaml")
				if err := os.WriteFile(file, []byte(tc.yaml), 0o600); err != nil {
					t.Fatal(err)
				}
				for _, fs := range [][]Finding{ValidateManifestBytes("nself.yaml", []byte(tc.yaml)), ValidateManifestFile(file)} {
					if len(fs) != 1 {
						t.Fatalf("expected one finding, got %+v", fs)
					}
					f := fs[0]
					if f.File != "nself.yaml" || f.Line != 1 || f.Column < 1 || f.Path != tc.path || f.Code != CodeManifestType || f.Severity != wantSeverity() || f.Fix == "" || !strings.Contains(f.Message, tc.tag) {
						t.Errorf("missing diagnostic context: %+v", f)
					}
					encoded, err := json.Marshal(f)
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(f.String(), tc.secret) || strings.Contains(string(encoded), tc.secret) {
						t.Errorf("finding echoes scalar %q: %+v", tc.secret, f)
					}
				}
			})
		}
	})
}
