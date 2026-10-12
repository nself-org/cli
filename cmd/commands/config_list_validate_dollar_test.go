package commands

// config_list_validate_dollar_test.go — D-0552: `config validate` warns, by
// file, line and key only, about a '$' the loader would expand.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWarnUnquotedDollar(t *testing.T) {
	t.Setenv("ENV", "dev")
	dir := t.TempDir()
	body := "A=1\nADMIN_PASSWORD_HASH=$2a$10$fakefakefakefakefake\nOK='$2a$10$fakefakefakefakefake'\nREF=${A}\n"
	if err := os.WriteFile(filepath.Join(dir, ".env.secrets"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	warnUnquotedDollar(&out, dir)
	got := out.String()
	if !strings.HasPrefix(got, "⚠ .env.secrets:2: ADMIN_PASSWORD_HASH holds an unquoted '$' that the loader expands;") {
		t.Errorf("warning = %q", got)
	}
	if strings.Count(got, "\n") != 1 || strings.Contains(got, "fakefake") {
		t.Errorf("want exactly one warning with no value, got %q", got)
	}
}
