package commands

// Purpose: pin the two cmdlog call sites (D-0473) that the unit tests of
// cmdlog and commandSecretValues cannot see: Execute must hand the restored
// secret values to cmdlog.MaskValues, and PersistentPreRunE must pass the
// command's secret values to Begin.
// Inputs: the built nself binary (pilotBinary) with a temp HOME, and the
// in-process restoreSecretArgv. Outputs: pass/fail on the log file content.
// Constraints: synthetic values only.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdlog"
	"github.com/nself-org/cli/internal/invoke"
)

const fakeCallsite0473 = "FAKE-planted-callsite-0473"

// TestRootLogsNoSecretPositional runs the real command tree: the value of
// `secrets set` must not reach ~/.nself/logs/nself.log, the key name must (the
// positive control that the line was written at all).
func TestRootLogsNoSecretPositional(t *testing.T) {
	bin := pilotBinary(t)
	home, proj := t.TempDir(), t.TempDir()
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "NSELF_") && !strings.HasPrefix(kv, "HOME=") && !strings.HasPrefix(kv, "USERPROFILE=") {
			env = append(env, kv)
		}
	}
	env = append(env, "HOME="+home, "USERPROFILE="+home, "NSELF_CMD_LOG_ENABLED=true")
	cmd := exec.Command(bin, "secrets", "set", "FAKEKEY", fakeCallsite0473)
	cmd.Dir, cmd.Env = proj, env
	_ = cmd.Run() // the exit code is not the point: the log line is written before RunE

	data, err := os.ReadFile(filepath.Join(home, ".nself", "logs", "nself.log"))
	if err != nil {
		t.Fatalf("no command log was written: %v", err)
	}
	log := string(data)
	if !strings.Contains(log, "FAKEKEY") {
		t.Fatalf("positive control: the log line lacks the key name: %s", log)
	}
	if strings.Contains(log, fakeCallsite0473) {
		t.Errorf("the secret value reached the command log: %s", log)
	}
	if !strings.Contains(log, "[REDACTED]") {
		t.Errorf("the value should be shown as [REDACTED]: %s", log)
	}
}

// TestRestoreSecretArgvMasksRestoredValues: after a hand-off restore, the
// restored value is on os.Args; a log line built from it must still hide it.
func TestRestoreSecretArgvMasksRestoredValues(t *testing.T) {
	saved := os.Args
	savedSecrets := restoredInvokeSecrets
	t.Cleanup(func() { os.Args, restoredInvokeSecrets = saved, savedSecrets })
	ph := "nself-invoke-secret:abcd1234abcd1234:0"
	os.Args = []string{"nself", "secrets", "set", "FAKEKEY", ph}
	t.Setenv(invoke.InvokedByEnv, "mcp")
	t.Setenv(invoke.SecretsEnv, `{"nonce":"abcd1234abcd1234","values":["`+fakeCallsite0473+`"]}`)
	if err := restoreSecretArgv(); err != nil {
		t.Fatal(err)
	}
	if os.Args[len(os.Args)-1] != fakeCallsite0473 {
		t.Fatalf("positive control: the value was not restored: %q", os.Args)
	}

	dir := t.TempDir()
	cmdlog.New(dir).Begin(os.Args)(0, nil)
	data, err := os.ReadFile(filepath.Join(dir, "nself.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "FAKEKEY") {
		t.Fatalf("positive control: no log line: %s", data)
	}
	if strings.Contains(string(data), fakeCallsite0473) {
		t.Errorf("restoreSecretArgv did not register the value with cmdlog: %s", data)
	}
}
