package commands

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

// TestDataLossGateJSONDeprecation checks the public envelope, including notices
// when stderr warnings are silenced and their absence for consent or previews.
func TestDataLossGateJSONDeprecation(t *testing.T) {
	for _, tc := range []struct {
		name, option string
		wantNotice   bool
	}{
		{"bare", "", true},
		{"quiet", "--quiet", true},
		{"warnings silenced", "--no-deprecation-warnings", true},
		{"confirmed", "--yes", false},
		{"preview", "--dry-run", false},
		{"text", "text", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NSELF_V15", "0")
			output.ResetState()
			t.Cleanup(output.ResetState)
			args := []string{"nself", "backup", "prune", "--json"}
			if tc.option == "text" {
				args = args[:3]
			} else if tc.option != "" {
				args = append(args, tc.option)
			}
			withArgs(t, args...)
			var stderr, stdout bytes.Buffer
			cmd := gateCmd(&stderr)
			if tc.option == "--yes" || tc.option == "--dry-run" {
				if err := cmd.Flags().Set(tc.option[2:], "true"); err != nil {
					t.Fatal(err)
				}
			}
			run := withDataLossConsent(func(*cobra.Command, []string) error { return nil }, "deletes old backups", "dry-run")
			if err := run(cmd, nil); err != nil {
				t.Fatal(err)
			}
			if err := output.EmitData(output.Writer{Out: &stdout}, "backup prune", nil); err != nil {
				t.Fatal(err)
			}
			var envelope output.Envelope
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if !tc.wantNotice {
				if envelope.Meta != nil {
					t.Fatalf("unexpected metadata: %+v", envelope.Meta)
				}
				return
			}
			want := output.Deprecation{Old: "backup prune", New: "backup prune --yes", RemovalAt: "v1.5.0"}
			if envelope.Meta == nil || len(envelope.Meta.Deprecations) != 1 || envelope.Meta.Deprecations[0] != want {
				t.Fatalf("want deprecation %+v, got envelope %s", want, stdout.String())
			}
		})
	}
}
