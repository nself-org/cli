package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// Purpose: flag and dry-run tests for `nself backup schedule` (P7-PROD-71).
// Inputs: the package-level backupScheduleCmd with flags set programmatically
// and a temp project directory as the current directory.
// Outputs: none (t.Error on a missing flag, wrong type or wrong unit text).

// resetBackupFlagSet returns every flag of a command to its default after a test, so
// repeatable (stringArray) flags do not accumulate across tests.
func resetBackupFlagSet(t *testing.T, fs *pflag.FlagSet) {
	t.Helper()
	t.Cleanup(func() {
		fs.VisitAll(func(f *pflag.Flag) {
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
			} else {
				_ = f.Value.Set(f.DefValue)
			}
			f.Changed = false
		})
	})
}

// TestBackupScheduleFlags checks the flag surface and runs the command with
// --dry-run from a project directory: the unit must run in that directory
// with the absolute running binary, carry every repeated --recipient and the
// heartbeat remote, and have no EnvironmentFile line.
func TestBackupScheduleFlags(t *testing.T) {
	fs := backupScheduleCmd.Flags()
	for name, typ := range map[string]string{
		"cron": "string", "to": "string", "unit-dir": "string", "dry-run": "bool",
		"recipient": "stringArray", "env-file": "string", "heartbeat-to": "string",
	} {
		f := fs.Lookup(name)
		if f == nil {
			t.Errorf("flag --%s is not registered on `backup schedule`", name)
			continue
		}
		if f.Value.Type() != typ {
			t.Errorf("--%s has type %s, want %s", name, f.Value.Type(), typ)
		}
	}
	if f := fs.Lookup("env-file"); f != nil && f.DefValue != "" {
		t.Errorf("--env-file default = %q: there must be no default env file", f.DefValue)
	}

	resetBackupFlagSet(t, fs)
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte("PROJECT_NAME=sched\nENV=dev\nPOSTGRES_PASSWORD=supersecretvalue123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)
	for k, v := range map[string]string{"cron": "30 2 * * *", "to": "r2:bkt/nself-web", "heartbeat-to": "r2hb:hb", "dry-run": "true"} {
		if err := fs.Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range []string{"age1qqqq", "age1zzzz"} {
		if err := fs.Set("recipient", r); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	backupScheduleCmd.SetOut(&out)
	defer backupScheduleCmd.SetOut(nil)
	if err := runBackupSchedule(backupScheduleCmd, nil); err != nil {
		t.Fatalf("runBackupSchedule: %v", err)
	}
	got := out.String()
	wd, _ := os.Getwd()
	exe, _ := os.Executable()
	realExe, _ := filepath.EvalSymlinks(exe)
	for _, want := range []string{
		"WorkingDirectory=" + wd,
		"ExecStart=" + realExe + " backup stream --to r2:bkt/nself-web --recipient age1qqqq --recipient age1zzzz --heartbeat-to r2hb:hb",
		"OnCalendar=*-*-* 02:30:00 UTC",
		"Persistent=true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("unit lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "EnvironmentFile") {
		t.Errorf("no EnvironmentFile line expected without --env-file:\n%s", got)
	}
}
