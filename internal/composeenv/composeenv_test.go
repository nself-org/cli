package composeenv

import (
	"strings"
	"testing"
)

func unquote(s string) string { return strings.Trim(s, `"`) }

// D-1149: a file written by another ENV yields nothing; no marker still reads.
func TestLiteral(t *testing.T) {
	const body = "POSTGRES_PASSWORD=\"fake-pg\"\nEMPTY=\nREF=${X}\n"
	for _, tc := range []struct {
		name, file, env, key, want string
	}{
		{"same env", Marker + "prod\n" + body, "prod", "POSTGRES_PASSWORD", "fake-pg"},
		{"other env", Marker + "dev\n" + body, "prod", "POSTGRES_PASSWORD", ""},
		{"marker after key", body + Marker + "dev\n", "prod", "POSTGRES_PASSWORD", ""},
		{"no marker", body, "prod", "POSTGRES_PASSWORD", "fake-pg"},
		{"empty value", body, "prod", "EMPTY", ""},
		{"reference", body, "prod", "REF", ""},
		{"first literal wins", "K=one\nK=two\n", "dev", "K", "one"},
	} {
		got, ok := Literal([]byte(tc.file), tc.env, tc.key, unquote)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("%s: Literal = %q, %v; want %q", tc.name, got, ok, tc.want)
		}
	}
}

func TestEnvName(t *testing.T) {
	if EnvName("") != "dev" || EnvName("prod") != "prod" {
		t.Error("EnvName: an unset env is dev, any other passes through")
	}
}
