package commands

// secrets_decrypt_test.go — debt D-0216, the plaintext opt-in of
// `config secrets decrypt-on-deploy`.
//
// Purpose: prove that v1.5 refuses to print decrypted secrets to a non-terminal
//   stdout without --reveal, prints them with --reveal, keeps the values out of
//   the --json envelope, and that v1.4 prints as before.
// Inputs:  the real nself binary (runPilot) against a project with a real age
//   store holding one fake secret.
// Outputs: test failures. Skipped (loudly) when age is not installed.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

const decryptFakeValue = "fake-secret-value-0001"

// decryptArgv returns the mode's spelling of a secrets subcommand: top-level
// `secrets` in v1.4, under `config` in v1.5.
func decryptArgv(v15 bool, rest ...string) []string {
	if v15 {
		return append([]string{"config", "secrets"}, rest...)
	}
	return append([]string{"secrets"}, rest...)
}

// decryptProject builds a project with a real age store holding DECRYPT_ITEM.
func decryptProject(t *testing.T, v15 bool) string {
	t.Helper()
	requireAgeLoud(t)
	proj := cfgFixtureProject(t)
	for _, argv := range [][]string{decryptArgv(v15, "init"), decryptArgv(v15, "set", "DECRYPT_ITEM", decryptFakeValue)} {
		if r := runPilot(t, proj, v15, nil, argv...); r.code != 0 {
			t.Fatalf("%v: exit %d\n%s", argv, r.code, r.stderr)
		}
	}
	return proj
}

func TestDecryptOnDeployPlaintextOptIn(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		v15 := os.Getenv("NSELF_V15") == "1"
		proj := decryptProject(t, v15)
		dec := decryptArgv(v15, "decrypt-on-deploy")
		withFlags := func(extra ...string) []string { return append(append([]string{}, dec...), extra...) }

		// Non-terminal stdout, no --reveal: refused in v1.5, printed in v1.4.
		r := runPilot(t, proj, v15, nil, dec...)
		if v15 {
			if r.code != 1 || r.stdout != "" {
				t.Errorf("v1.5 no --reveal: exit %d stdout %q, want exit 1 and empty stdout", r.code, r.stdout)
			}
			if !strings.Contains(r.stderr, "E401") || !strings.Contains(r.stderr, "--reveal") {
				t.Errorf("v1.5 no --reveal: stderr %q lacks E401 and the --reveal fix", r.stderr)
			}
			if strings.Contains(r.stdout+r.stderr, decryptFakeValue) {
				t.Error("v1.5 refusal leaked the store value")
			}
		} else if r.code != 0 || !strings.Contains(r.stdout, "DECRYPT_ITEM="+decryptFakeValue) {
			t.Errorf("v1.4 unchanged: exit %d stdout %q, want KEY=VALUE printed", r.code, r.stdout)
		}

		// --reveal prints KEY=VALUE in both modes.
		r = runPilot(t, proj, v15, nil, withFlags("--reveal")...)
		if r.code != 0 || !strings.Contains(r.stdout, "DECRYPT_ITEM="+decryptFakeValue) {
			t.Errorf("--reveal: exit %d stdout %q, want KEY=VALUE printed", r.code, r.stdout)
		}

		// --reveal with --json is E402 in both modes.
		r = runPilot(t, proj, v15, nil, withFlags("--reveal", "--json")...)
		if r.code != 1 || !strings.Contains(r.stderr+r.stdout, "E402") || strings.Contains(r.stdout+r.stderr, decryptFakeValue) {
			t.Errorf("--reveal --json: exit %d stdout %q stderr %q, want E402 and no value", r.code, r.stdout, r.stderr)
		}
	})
}

// TestDecryptOnDeployJSONRedacted: the v1.5 envelope names the key and never
// carries the value.
func TestDecryptOnDeployJSONRedacted(t *testing.T) {
	compattest.Set(t, true)
	proj := decryptProject(t, true)
	r := runPilot(t, proj, true, nil, "config", "secrets", "decrypt-on-deploy", "--json")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.stderr)
	}
	if strings.Contains(r.stdout+r.stderr, decryptFakeValue) {
		t.Errorf("the store value reached the output: %s", r.stdout)
	}
	data := validateEnvelope(t, r.stdout, "config secrets decrypt-on-deploy")
	values, _ := data["values"].(map[string]any)
	if values["DECRYPT_ITEM"] != "[REDACTED]" {
		t.Errorf("values = %v, want DECRYPT_ITEM: [REDACTED]", values)
	}
}

// TestDecryptOnDeployJSONMultilineValue: a multi-line value whose body has a
// line shaped like KEY= must not become an envelope key, and no part of the
// value may reach the output. The key names come from the store, not the text.
func TestDecryptOnDeployJSONMultilineValue(t *testing.T) {
	compattest.Set(t, true)
	proj := decryptProject(t, true)
	const tail = "FakeTailChunkAbc123=="
	multi := "-----BEGIN FAKE KEY-----\n" + tail + "\n-----END FAKE KEY-----"
	if r := runPilot(t, proj, true, nil, "config", "secrets", "set", "DECRYPT_PEM", "--", multi); r.code != 0 {
		t.Fatalf("set DECRYPT_PEM: exit %d\n%s", r.code, r.stderr)
	}
	r := runPilot(t, proj, true, nil, "config", "secrets", "decrypt-on-deploy", "--json")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.stderr)
	}
	out := r.stdout + r.stderr
	for _, leak := range []string{"FakeTailChunkAbc123", "BEGIN FAKE KEY", "END FAKE KEY", decryptFakeValue} {
		if strings.Contains(out, leak) {
			t.Errorf("%q reached the output: %s", leak, out)
		}
	}
	data := validateEnvelope(t, r.stdout, "config secrets decrypt-on-deploy")
	values, _ := data["values"].(map[string]any)
	if len(values) != 2 || values["DECRYPT_ITEM"] != "[REDACTED]" || values["DECRYPT_PEM"] != "[REDACTED]" {
		t.Errorf("values = %v, want exactly DECRYPT_ITEM and DECRYPT_PEM, both [REDACTED]", values)
	}
}

// TestDecryptOnDeployTerminalStdout: with stdout standing in for a terminal,
// v1.5 prints KEY=VALUE without --reveal; with a non-terminal stdout it refuses
// with E401 and prints nothing. In-process, so the terminal check is stubbed.
func TestDecryptOnDeployTerminalStdout(t *testing.T) {
	compattest.Set(t, true)
	proj := decryptProject(t, true)
	// The key lives under the HOME runPilot used for the setup commands.
	t.Setenv("HOME", filepath.Join(filepath.Dir(proj), "home"))
	t.Setenv("DEPLOY_AGE_KEY", "")
	t.Chdir(proj)
	orig := decryptStdoutIsTerminal
	t.Cleanup(func() { decryptStdoutIsTerminal = orig })
	run := func(isTerm bool) (string, error) {
		decryptStdoutIsTerminal = func() bool { return isTerm }
		return captureStdout(t, func() error { return secretsDecryptOnDeployCmd.RunE(secretsDecryptOnDeployCmd, nil) })
	}

	out, err := run(true)
	if err != nil {
		t.Fatalf("terminal stdout, no --reveal: %v, want it to print", err)
	}
	if !strings.Contains(out, "DECRYPT_ITEM="+decryptFakeValue) {
		t.Errorf("terminal stdout printed %q, want DECRYPT_ITEM=<value>", out)
	}

	out, err = run(false)
	if err == nil || !strings.Contains(err.Error(), "E401") || !strings.Contains(err.Error(), "--reveal") {
		t.Errorf("non-terminal stdout, no --reveal: err %v, want E401 naming --reveal", err)
	}
	if out != "" || (err != nil && strings.Contains(err.Error(), decryptFakeValue)) {
		t.Errorf("refusal printed or leaked the value: stdout %q err %v", out, err)
	}
}
