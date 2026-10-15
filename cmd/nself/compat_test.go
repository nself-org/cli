package main

import (
	"io"
	"os"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	sdkcompat "github.com/nself-org/cli/sdk/go/v2/compat"
)

// TestExecuteExportsResolvedCompatMode pins the startup boundary: SDK calls
// and child processes must receive the CLI's resolved mode before commands run.
func TestExecuteExportsResolvedCompatMode(t *testing.T) {
	for _, value := range []string{"", "0", "1", "true", "TRUE", "yes"} {
		t.Run("env="+value, func(t *testing.T) {
			t.Setenv(compat.EnvVar, value)
			want := compat.V15()
			wantEnv := "0"
			if want {
				wantEnv = "1"
			}
			called := false
			err := execute(func() error {
				called = true
				if got := os.Getenv(compat.EnvVar); got != wantEnv {
					t.Errorf("exported mode = %q, want %q", got, wantEnv)
				}
				if got := sdkcompat.V15(); got != want {
					t.Errorf("SDK mode = %v, want CLI mode %v", got, want)
				}
				return nil
			}, io.Discard)
			if err != nil || !called {
				t.Fatalf("execute: called=%v, err=%v", called, err)
			}
		})
	}
}

// TestExportCompatModeDefaultFlip models P7-SHIP without changing the shipped
// CLI default: the resolved mode wins over the SDK's independent env rule.
func TestExportCompatModeDefaultFlip(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  string
		mode bool
	}{
		{"v15-default-unset", "", true},
		{"v15-default-false-env", "0", true},
		{"v14-resolved-truthy-env", "true", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(compat.EnvVar, tc.env)
			if tc.name == "v15-default-unset" {
				if err := os.Unsetenv(compat.EnvVar); err != nil {
					t.Fatal(err)
				}
			}
			if err := exportCompatMode(tc.mode); err != nil {
				t.Fatal(err)
			}
			if got := sdkcompat.V15(); got != tc.mode {
				t.Fatalf("SDK mode = %v, want resolved mode %v", got, tc.mode)
			}
		})
	}
}
