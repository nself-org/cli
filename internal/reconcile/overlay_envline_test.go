package reconcile

import (
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

// TestEnvFileWithLineStatements covers old files whose values hold line
// breaks: the old value goes with all its lines, and a KEY= line inside
// another key's value is not a statement (S7).
func TestEnvFileWithLineStatements(t *testing.T) {
	tests := []struct {
		name, old string
		want      map[string]string
		absent    []string
	}{
		{
			name: "replace a multi-line value",
			old:  "K=\"-----BEGIN\nINJECTED=1\n-----END\"\nB=2\n",
			want: map[string]string{"K": "new-0000", "B": "2"}, absent: []string{"INJECTED"},
		},
		{
			name: "key line inside another value",
			old:  "A=\"x\nK=inner\ny\"\nK=old\n",
			want: map[string]string{"A": "x\nK=inner\ny", "K": "new-0000"},
		},
		{
			name: "only the inner line has the key",
			old:  "A=\"x\nK=inner\ny\"\n",
			want: map[string]string{"A": "x\nK=inner\ny", "K": "new-0000"},
		},
		{
			name: "single-quoted multi-line neighbour",
			old:  "# note\nA='p\nq'\nK=old\nZ=1\n",
			want: map[string]string{"A": "p\nq", "K": "new-0000", "Z": "1"},
		},
		{
			name: "neighbour that expands the key",
			old:  "K=1\nB=\"${K}x\"\n",
			want: map[string]string{"K": "new-0000"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := godotenv.UnmarshalBytes([]byte(tc.old)); err != nil {
				t.Fatalf("fixture does not load: %v", err)
			}
			out, _, err := EnvFileWithLine([]byte(tc.old), true, "K", "new-0000")
			if err != nil {
				t.Fatal(err)
			}
			m, err := godotenv.UnmarshalBytes(out)
			if err != nil {
				t.Fatalf("output does not load: %v\n%s", err, out)
			}
			for k, v := range tc.want {
				if m[k] != v {
					t.Errorf("%s = %q, want %q\n%s", k, m[k], v, out)
				}
			}
			for _, k := range tc.absent {
				if _, ok := m[k]; ok {
					t.Errorf("key %s appeared:\n%s", k, out)
				}
			}
		})
	}
}

// TestEnvFileWithLineBrokenOldFile proves a file that would not load gives an
// error without any value text.
func TestEnvFileWithLineBrokenOldFile(t *testing.T) {
	_, _, err := EnvFileWithLine([]byte("A=\"fake-secret-0000\n"), true, "K", "v")
	if err == nil {
		t.Fatal("expected an error: the old file has an unterminated quote")
	}
	if strings.Contains(err.Error(), "fake-secret-0000") || !strings.Contains(err.Error(), "K") {
		t.Fatalf("error text is wrong: %v", err)
	}
}
