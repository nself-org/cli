package commands

// config_set_envline_test.go — setEnvFileLine must surface the encoder error
// of reconcile.EnvFileWithLine instead of writing an empty file (D-0404).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSetEnvFileLineRejectsUnstorableValue: a value a dotenv file cannot hold
// (a space plus a trailing backslash) on a missing file returns an error, does
// not create the file and does not echo the value.
func TestSetEnvFileLineRejectsUnstorableValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.secrets")
	updated, err := setEnvFileLine(path, "FAKE_SECRET", `fake-secret-0000 x\`)
	if err == nil {
		t.Fatal("expected an error")
	}
	if updated {
		t.Error("updated = true on error")
	}
	if strings.Contains(err.Error(), "fake-secret-0000") || !strings.Contains(err.Error(), "FAKE_SECRET") {
		t.Errorf("error text is wrong: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("file was created (stat err %v)", statErr)
	}
}

// TestSetEnvFileLineRejectsUnstorableValueExisting: the same value on an
// existing file errors and leaves the file as it was.
func TestSetEnvFileLineRejectsUnstorableValueExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.secrets")
	const old = "OTHER=1\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := setEnvFileLine(path, "FAKE_SECRET", `fake-secret-0000 x\`); err == nil || strings.Contains(err.Error(), "fake-secret-0000") {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != old {
		t.Errorf("file changed: %q", got)
	}
}
