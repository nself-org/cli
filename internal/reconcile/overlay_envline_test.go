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

// TestEnvFileWithLineNeverLosesNeighbours: for statement shapes the line
// splitter may not see (several statements on one line, "KEY: value", a key
// line inside a YAML-style value), the rewrite either errors or leaves every
// other key as it was. Never a silent loss.
func TestEnvFileWithLineNeverLosesNeighbours(t *testing.T) {
	olds := []string{
		"KEY=\"old\" OTHER=\"keep\"\n",
		"OTHER: \"prefix\nKEY=inside\nsuffix\"\nKEY=old\n",
		"OTHER='literal$\nKEY=inside\nsuffix'\nKEY=old\n",
		"export KEY=old\nOTHER=keep\n",
		"KEY: old\nOTHER=keep\n",
		"OTHER=keep\n",
	}
	for _, old := range olds {
		before, err := godotenv.UnmarshalBytes([]byte(old))
		if err != nil {
			t.Fatalf("fixture %q does not load: %v", old, err)
		}
		out, _, err := EnvFileWithLine([]byte(old), true, "KEY", "new-0000")
		if err != nil {
			if strings.Contains(err.Error(), "new-0000") || strings.Contains(err.Error(), "keep") {
				t.Errorf("%q: error holds a value: %v", old, err)
			}
			continue
		}
		after, err := godotenv.UnmarshalBytes(out)
		if err != nil {
			t.Fatalf("%q: output does not load: %v", old, err)
		}
		if after["KEY"] != "new-0000" || len(after) != len(before)+boolInt(before["KEY"] == "") {
			t.Errorf("%q: keys changed: before %v after %v", old, before, after)
		}
		for k, v := range before {
			if k != "KEY" && after[k] != v {
				t.Errorf("%q: %s changed from %q to %q", old, k, v, after[k])
			}
		}
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestEnvFileWithLineKeepsDependentValues: rotating a secret that another
// value expands must work, and the dependent value follows the new secret.
func TestEnvFileWithLineKeepsDependentValues(t *testing.T) {
	old := "POSTGRES_PASSWORD=oldpw1\nDATABASE_URL=postgres://u:${POSTGRES_PASSWORD}@db/x\n"
	out, found, err := EnvFileWithLine([]byte(old), true, "POSTGRES_PASSWORD", "newpw2")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	m, err := godotenv.UnmarshalBytes(out)
	if err != nil {
		t.Fatal(err)
	}
	if m["POSTGRES_PASSWORD"] != "newpw2" || m["DATABASE_URL"] != "postgres://u:newpw2@db/x" {
		t.Fatalf("got %v", m)
	}
}
