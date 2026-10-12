package config

// validator_env_dollar_test.go — the unquoted-'$' scanner (D-0552).
//
// Each case also checks the verdict against godotenv itself: a value is flagged
// exactly when the loader reads it back as something other than the literal
// text the owner wrote.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

func TestScanUnquotedDollar(t *testing.T) {
	const bcrypt = "$2a$10$abcdefghijklmnopqrstuv"
	tests := []struct {
		name      string
		src       string
		wantLines []int  // lines flagged
		literal   string // KEY's literal text; "" skips the godotenv parity check
	}{
		{"bcrypt unquoted", "ADMIN_PASSWORD_HASH=" + bcrypt + "\n", []int{1}, bcrypt},
		{"bcrypt single-quoted", "ADMIN_PASSWORD_HASH='" + bcrypt + "'\n", nil, bcrypt},
		{"bcrypt double-quoted", "ADMIN_PASSWORD_HASH=\"" + bcrypt + "\"\n", []int{1}, bcrypt},
		{"bcrypt escaped", "ADMIN_PASSWORD_HASH=\\$2a\\$10\\$abcdefghijklmnopqrstuv\n", nil, bcrypt},
		{"escaped in double quotes", "ADMIN_PASSWORD_HASH=\"\\$2a\\$10\\$abcdefgh\"\n", nil, "$2a$10$abcdefgh"},
		{"braced reference", "DATABASE_URL=postgres://${POSTGRES_USER}@db/x\n", nil, ""},
		{"braced reference, double-quoted", "DATABASE_URL=\"postgres://${POSTGRES_USER}@db/x\"\n", nil, ""},
		{"bare reference", "DATABASE_URL=postgres://$POSTGRES_USER@db/x\n", []int{1}, "postgres://$POSTGRES_USER@db/x"},
		{"command substitution, capitals", "K=$(CMD)\n", []int{1}, "$(CMD)"},
		{"command substitution, lowercase is left alone by godotenv", "K=$(cmd)\n", nil, "$(cmd)"},
		{"lowercase variable is left alone by godotenv", "K=$name\n", nil, "$name"},
		{"lone dollar", "K=cost$ 5\n", nil, "cost$ 5"},
		{"export prefix", "export K=" + bcrypt + "\n", []int{1}, bcrypt},
		{"yaml style", "K: " + bcrypt + "\n", []int{1}, bcrypt},
		{"comments and blanks", "# K=" + bcrypt + "\n\n   \nOK=1\n", nil, ""},
		{"inline comment hides the dollar", "K=abc # costs $2a\n", nil, "abc"},
		{"dollar before the inline comment", "K=$2a # note\n", []int{1}, "$2a"},
		{"second line", "A=1\nK=" + bcrypt + "\n", []int{2}, bcrypt},
		{"CRLF", "A=1\r\nK=" + bcrypt + "\r\n", []int{2}, bcrypt},
		{"multi-line double-quoted, dollar on the continuation", "K=\"first\n" + bcrypt + "\nlast\"\nZ=1\n", []int{2}, ""},
		{"multi-line double-quoted, no dollar", "K=\"first\nsecond\"\nZ=$2a\n", []int{3}, ""},
		{"multi-line single-quoted is skipped", "K='first\n" + bcrypt + "\nlast'\nZ=1\n", nil, ""},
		{"key after a closed multi-line value", "K=\"a\nb\"\nL=" + bcrypt + "\n", []int{3}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := scanUnquotedDollar(".env.secrets", []byte(tc.src))
			var lines []int
			for _, f := range got {
				lines = append(lines, f.Line)
				if f.File != ".env.secrets" || f.Key == "" {
					t.Errorf("bad finding %+v", f)
				}
			}
			if len(lines) != len(tc.wantLines) {
				t.Fatalf("flagged lines %v, want %v", lines, tc.wantLines)
			}
			for i := range lines {
				if lines[i] != tc.wantLines[i] {
					t.Fatalf("flagged lines %v, want %v", lines, tc.wantLines)
				}
			}
			if tc.literal == "" {
				return
			}
			m, err := godotenv.UnmarshalBytes([]byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			var key string
			for _, k := range []string{"ADMIN_PASSWORD_HASH", "DATABASE_URL", "K"} {
				if _, ok := m[k]; ok {
					key = k
				}
			}
			changed := m[key] != tc.literal
			if changed != (len(tc.wantLines) > 0) {
				t.Errorf("godotenv read %q for literal %q (changed=%v) but flagged=%v", m[key], tc.literal, changed, len(tc.wantLines) > 0)
			}
		})
	}
}

func TestDollarFindingHoldsNoValue(t *testing.T) {
	got := scanUnquotedDollar(".env", []byte("K=$2a$10$fake-planted-s3cret-0444\n"))
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	if s := strings.Join([]string{got[0].File, got[0].Key}, " "); strings.Contains(s, "planted") {
		t.Errorf("finding holds the value: %q", s)
	}
}

func TestUnquotedDollarFindingsCascade(t *testing.T) {
	t.Setenv(LegacyEnvOrderVar, "")
	dir := t.TempDir()
	writeEnvFiles(t, dir, map[string]string{
		".env":         "A=1\n",
		".env.secrets": "HASH=$2a$10$abcdefghij\nOK='$2a$10$abcdefghij'\n",
		".env.prod":    "OTHER=$2a$10$abcdefghij\n", // not in the dev cascade
	})
	got := UnquotedDollarFindings(dir, "dev")
	if len(got) != 1 || got[0].File != ".env.secrets" || got[0].Line != 1 || got[0].Key != "HASH" {
		t.Fatalf("dev cascade: %+v", got)
	}
	got = UnquotedDollarFindings(dir, "prod")
	if len(got) != 2 {
		t.Fatalf("prod cascade: %+v", got)
	}
	if got := UnquotedDollarFindings(filepath.Join(dir, "missing"), "dev"); len(got) != 0 {
		t.Errorf("no files: %+v", got)
	}
}
