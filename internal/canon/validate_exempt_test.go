package canon

import (
	"strings"
	"testing"
)

// TestExemptReason checks that a cli-only row needs a non-empty exempt_reason,
// and that other surfaces do not. Fixtures are in-memory documents so the test
// does not depend on the embedded fragments.
func TestExemptReason(t *testing.T) {
	cases := []struct {
		name    string
		doc     string
		wantErr string // empty means the document must parse
	}{
		{
			name:    "cli-only without reason fails",
			doc:     verbsHeader("a") + "  a b: {side_effect: read, surface: cli-only}\n",
			wantErr: `commands["a b"]: surface cli-only needs a non-empty exempt_reason`,
		},
		{
			name:    "cli-only with blank reason fails",
			doc:     verbsHeader("a") + "  a b: {side_effect: read, surface: cli-only, exempt_reason: \"   \"}\n",
			wantErr: `commands["a b"]: surface cli-only needs a non-empty exempt_reason`,
		},
		{
			name: "cli-only with reason passes",
			doc:  verbsHeader("a") + "  a b: {side_effect: read, surface: cli-only, exempt_reason: prints the secret}\n",
		},
		{
			name: "all surface without reason passes",
			doc:  verbsHeader("a") + "  a b: {side_effect: read, surface: all}\n",
		},
		{
			name: "no surface without reason passes",
			doc:  verbsHeader("a") + "  a b: {side_effect: read}\n",
		},
	}
	if len(cases) == 0 {
		t.Fatal("TestExemptReason has an empty case table")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.doc))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("want no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got none", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}
