package verify

import (
	"strings"
	"testing"
)

func TestSchemaMajor(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"ci.release-manifest/v1", 1, true},
		{"ci.release-manifest/v2", 2, true},
		{"ci.release-manifest/v0", 0, true},
		{"ci.release-manifest/v-1", 0, false},
		{"ci.release-manifest/v01", 0, false},
		{"ci.release-manifest/v", 0, false},
		{"ci.release-manifest/vx", 0, false},
		{"other/v1", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, ok := schemaMajor(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("schemaMajor(%q) = %d, %v; want %d, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestDecodeManifestMajorMessage(t *testing.T) {
	_, err := decodeManifest([]byte(`{"schema":"ci.release-manifest/v0"}`))
	if codeOf(err) != "E492" || !strings.Contains(err.Error(), "major 0") {
		t.Fatalf("v0: %v, want the major named", err)
	}
}

func TestClip(t *testing.T) {
	forty, long := strings.Repeat("a", 40), strings.Repeat("a", 41)
	if clip(forty) != forty {
		t.Errorf("a 40 byte string was clipped: %q", clip(forty))
	}
	if got := clip(long); got != forty+"..." {
		t.Errorf("clip(41 bytes) = %q", got)
	}
}
