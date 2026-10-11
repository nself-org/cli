package commands

import (
	"bytes"
	"strings"
	"testing"
)

// TestPrintCloneNextSteps pins the clone-template hint to commands that exist:
// metadata apply lives under `nself db hasura` and takes no --file flag, and
// the legacy Flutter starter is never offered as a command to run (D-0127).
func TestPrintCloneNextSteps(t *testing.T) {
	tests := []struct {
		name    string
		plugins []string
		want    []string
		notWant []string
	}{
		{
			name:    "with plugins",
			plugins: []string{"chat", "realtime"},
			want: []string{
				"    cd /tmp/app\n",
				"    nself start\n",
				"    nself db migrate\n",
				"    nself db hasura metadata apply\n",
				"    nself plugin install chat realtime\n",
				"legacy starter app; see README.md",
			},
			notWant: []string{"--file", "flutter run", "nself hasura metadata"},
		},
		{
			name:    "no plugins",
			plugins: nil,
			want:    []string{"    nself db hasura metadata apply\n"},
			notWant: []string{"plugin install", "--file", "flutter run"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printCloneNextSteps(&buf, "/tmp/app", tt.plugins)
			got := buf.String()
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("output missing %q:\n%s", w, got)
				}
			}
			for _, nw := range tt.notWant {
				if strings.Contains(got, nw) {
					t.Errorf("output contains stale %q:\n%s", nw, got)
				}
			}
		})
	}
}
