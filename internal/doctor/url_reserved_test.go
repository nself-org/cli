package doctor

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Test passwords are fixtures, not secrets. Failures print names only.
const (
	reservedPW = "a/b@c:d#e?f%g+h i"
	plainPW    = "abcdefgh12345678"
)

const rawFragment = `services:
  cron:
    environment:
      - DATABASE_URL=postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}
      - REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379
`

const safeFragment = `services:
  cron:
    environment:
      - DATABASE_URL=postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}@postgres:5432/${POSTGRES_DB}
      - REDIS_URL=redis://:${REDIS_PASSWORD_URLENC:-${REDIS_PASSWORD}}@redis:6379
      - PGPASSWORD=${POSTGRES_PASSWORD}
`

func TestURLReservedPasswordNeedsEncoding(t *testing.T) {
	if !PasswordNeedsURLEncoding(reservedPW) || PasswordNeedsURLEncoding(plainPW) || PasswordNeedsURLEncoding("") {
		t.Fatal("PasswordNeedsURLEncoding misclassified a fixture")
	}
	for i, pw := range []string{"a/b", "a@b", "a:b", "a#b", "a?b", "a%b", "a b"} {
		if !PasswordNeedsURLEncoding(pw) {
			t.Errorf("reserved character case %d not flagged", i)
		}
	}
}

func TestURLReservedRawUseDetection(t *testing.T) {
	cases := []struct {
		name, frag, variable string
		want                 bool
	}{
		{"raw postgres in URL", rawFragment, "POSTGRES_PASSWORD", true},
		{"raw redis in URL", rawFragment, "REDIS_PASSWORD", true},
		{"nested safe form is not raw", safeFragment, "POSTGRES_PASSWORD", false},
		{"nested safe form redis", safeFragment, "REDIS_PASSWORD", false},
		{"raw outside a URL is fine", "      - PGPASSWORD=${POSTGRES_PASSWORD}\n", "POSTGRES_PASSWORD", false},
		{"commented raw use ignored", "# redis://:${REDIS_PASSWORD}@redis\n", "REDIS_PASSWORD", false},
		{"bare $VAR in URL is raw", "      - U=postgresql://u:$POSTGRES_PASSWORD@db/x\n", "POSTGRES_PASSWORD", true},
		{"one safe and one raw on a line is raw", "U=a://${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}@h b://${POSTGRES_PASSWORD}@h\n", "POSTGRES_PASSWORD", true},
		{"other variable with same prefix", "U=redis://:${REDIS_PASSWORD_OLD}@h\n", "REDIS_PASSWORD", false},
	}
	for _, c := range cases {
		if got := RawPasswordInURL(c.frag, c.variable); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestURLReservedFindings(t *testing.T) {
	pws := map[string]string{"POSTGRES_PASSWORD": reservedPW, "REDIS_PASSWORD": reservedPW}
	got := URLReservedFindings(pws, []string{safeFragment, rawFragment})
	if want := []string{"POSTGRES_PASSWORD", "REDIS_PASSWORD"}; !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}
	// Plain passwords: silent even with raw use.
	plain := map[string]string{"POSTGRES_PASSWORD": plainPW, "REDIS_PASSWORD": plainPW}
	if got := URLReservedFindings(plain, []string{rawFragment}); len(got) != 0 {
		t.Errorf("plain passwords must not be flagged, got %v", got)
	}
	// Reserved password, safe fragments only: silent.
	if got := URLReservedFindings(pws, []string{safeFragment}); len(got) != 0 {
		t.Errorf("safe fragments must not be flagged, got %v", got)
	}
	// Only the raw variable is named.
	one := map[string]string{"POSTGRES_PASSWORD": reservedPW, "REDIS_PASSWORD": plainPW}
	if got := URLReservedFindings(one, []string{rawFragment}); !reflect.DeepEqual(got, []string{"POSTGRES_PASSWORD"}) {
		t.Errorf("only the reserved password is flagged, got %v", got)
	}
}

// TestURLReservedMessagesNameVariableNeverValue is the secret-hygiene guard:
// the advisory text carries the variable name and the fix, and none of the
// password's characters runs make it into the output.
func TestURLReservedMessagesNameVariableNeverValue(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "docker-compose.plugin.yml")
	if err := os.WriteFile(p, []byte(rawFragment), 0o600); err != nil {
		t.Fatal(err)
	}
	pws := map[string]string{"POSTGRES_PASSWORD": reservedPW}
	msgs := URLReservedMessages([]string{p, filepath.Join(dir, "missing.yml")}, pws)
	if len(msgs) != 1 {
		t.Fatalf("want exactly one advisory, got %d", len(msgs))
	}
	m := msgs[0]
	if !strings.Contains(m, "POSTGRES_PASSWORD") || !strings.Contains(m, "POSTGRES_PASSWORD_URLENC") {
		t.Error("advisory must name the variable and its URLENC twin")
	}
	for _, frag := range []string{reservedPW, "a/b@c", "d#e?f", "f%g+h"} {
		if strings.Contains(m, frag) {
			t.Error("advisory leaked part of the password value")
		}
	}
	// No fragment, or only safe ones: silent.
	if got := URLReservedMessages(nil, pws); len(got) != 0 {
		t.Error("no fragments must be silent")
	}
}
