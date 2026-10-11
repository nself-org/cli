package reconcile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joho/godotenv"
	"github.com/nself-org/cli/internal/config"
)

// roundTripValues are synthetic values for the write-then-load tests. The
// names say what each one stresses in godotenv v1.5.1.
var roundTripValues = []struct{ name, value string }{
	{"plain", "plain"},
	{"space", "a b"},
	{"trailing backslash", `ends-in-backslash\`},
	{"literal backslash n", `a\nb`},
	{"double backslash", `back\\slash`},
	{"double quote", `has"dq`},
	{"single quote", `has'sq`},
	{"hash", "hash # not comment"},
	{"leading hash", "#leading"},
	{"dollar", "dollar$HOME and ${PATH} and $"},
	{"escaped dollar", `\$`},
	{"mixed", `mix "q" \ $X # end`},
	{"tab", "tab\there"},
	{"real newline", "line1\nline2"},
	{"real CR", "line1\rline2"},
	{"empty", ""},
	{"backtick", "back`tick"},
	{"ends in double quote", `ends-in-dq"`},
	{"only a double quote", `"`},
	{"leading and trailing quotes", `"quoted"`},
	{"ends in single quote", `it's'`},
	{"backslash then quote", `a\"b`},
	{"spaced trailing backslash", `a b\`},
	{"only backslash", `\`},
}

// TestEnvFileWithLineRoundTrip proves a written line loads back to the exact
// value, or fails with an error that does not hold the value.
func TestEnvFileWithLineRoundTrip(t *testing.T) {
	for _, tc := range roundTripValues {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := EnvFileWithLine(nil, false, "FAKE_SECRET", tc.value)
			if err != nil {
				if tc.value != "" && strings.Contains(err.Error(), tc.value) {
					t.Fatalf("error holds the value: %v", err)
				}
				if !strings.Contains(tc.name, "spaced trailing backslash") {
					t.Fatalf("value %q was rejected: %v", tc.value, err)
				}
				return
			}
			if strings.Count(string(out), "\n") != 1 {
				t.Fatalf("line is not a single line: %q", out)
			}
			m, err := godotenv.UnmarshalBytes(out)
			if err != nil {
				t.Fatalf("written line %q does not parse: %v", out, err)
			}
			if got := m["FAKE_SECRET"]; got != tc.value {
				t.Fatalf("round trip: wrote %q, read back %q", out, got)
			}
		})
	}
}

// TestEnvFileWithLineRejectsUnstorable pins the one rejected shape: a value
// ending in a backslash that also needs quoting. The error is secret-free.
func TestEnvFileWithLineRejectsUnstorable(t *testing.T) {
	const v = `fake-secret-0000 x\`
	_, _, err := EnvFileWithLine(nil, false, "FAKE_SECRET", v)
	if err == nil {
		t.Fatal("expected an error for a spaced value ending in a backslash")
	}
	if strings.Contains(err.Error(), "fake-secret-0000") || !strings.Contains(err.Error(), "FAKE_SECRET") {
		t.Fatalf("error text is wrong: %v", err)
	}
}

// TestEnvFileWithLineKeepsOtherLines checks the replace path still keeps
// comments and unrelated keys when the new value needs escaping.
func TestEnvFileWithLineKeepsOtherLines(t *testing.T) {
	old := []byte("# note\nOTHER=1\nFAKE_SECRET=old\n")
	out, found, err := EnvFileWithLine(old, true, "FAKE_SECRET", `p\w$d`)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	m, err := godotenv.UnmarshalBytes(out)
	if err != nil || m["FAKE_SECRET"] != `p\w$d` || m["OTHER"] != "1" || !strings.HasPrefix(string(out), "# note\n") {
		t.Fatalf("got %q (%v)", out, err)
	}
}

// TestEnvOverlayLoadRoundTrip loads each written value through config.Load
// with EnvOverlay, the path `config set` plans with.
func TestEnvOverlayLoadRoundTrip(t *testing.T) {
	for _, tc := range roundTripValues {
		if tc.value == "" || strings.ContainsAny(tc.value, "\r") || strings.HasSuffix(tc.value, " ") {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := EnvFileWithLine(nil, false, "FAKE_ROUNDTRIP_SECRET", tc.value)
			if err != nil {
				return
			}
			dir := t.TempDir()
			t.Setenv("FAKE_ROUNDTRIP_SECRET", "")
			_ = os.Unsetenv("FAKE_ROUNDTRIP_SECRET")
			_, err = config.LoadWithOptions(filepath.Clean(dir), config.LoadOptions{EnvOverlay: map[string][]byte{".env.secrets": out}})
			if err != nil {
				if strings.Contains(err.Error(), tc.value) {
					t.Fatalf("error holds the value: %v", err)
				}
			}
			if got := os.Getenv("FAKE_ROUNDTRIP_SECRET"); got != tc.value {
				t.Fatalf("overlay load: wrote %q, process env holds %q (err %v)", out, got, err)
			}
		})
	}
}
