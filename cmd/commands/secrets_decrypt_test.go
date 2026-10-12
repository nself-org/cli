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
