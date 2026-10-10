package commands

// config_explain_test.go — `nself config explain` (P7-SURF-06).
//
// Purpose: prove the command reports the key, default, bound flags, machine
//          parameters, the nself.yaml row and the winning source for each
//          cascade layer, redacts values without --reveal, resolves a flag to
//          its key, and fails with E434 for an unknown key or an unbound flag.
// Constraints: runs the command body against a temp project directory; the
//          registry is the real command tree.

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

// explainRun runs `config explain <args...>` in dir and returns stdout and the
// envelope document (nil in human mode).
func explainRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := &cobra.Command{Use: "explain"}
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Bool("reveal", false, "")
	if err := prepareConfigExplain(cmd, args); err != nil {
		return "", err
	}
	return captureStdout(t, func() error { return runConfigExplain(cmd, args) })
}

func explainCode(t *testing.T, err error) string {
	t.Helper()
	var ce *errs.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("want a structured error, got %v", err)
	}
	return ce.Code
}

func TestConfigExplain(t *testing.T) {
	t.Setenv("NSELF_LEGACY_ENV_ORDER", "")
	t.Setenv("ENV", "")
	_ = os.Unsetenv("BASE_DOMAIN")

	layers := []struct {
		name  string
		files map[string]string
		proc  string
		want  string // effective source
		value string // effective value
	}{
		{"env file wins", map[string]string{".env": "BASE_DOMAIN=env.test\n"}, "", ".env", "env.test"},
		{"env.dev wins", map[string]string{".env": "BASE_DOMAIN=env.test\n", ".env.dev": "BASE_DOMAIN=dev.test\n"}, "", ".env.dev", "dev.test"},
		{"env.local wins", map[string]string{".env": "BASE_DOMAIN=env.test\n", ".env.dev": "BASE_DOMAIN=dev.test\n", ".env.local": "BASE_DOMAIN=local.test\n"}, "", ".env.local", "local.test"},
		{"a file replaces the process environment", map[string]string{".env": "BASE_DOMAIN=env.test\n"}, "proc.test", ".env", "env.test"},
		{"process environment wins when no file sets it", map[string]string{".env": "PROJECT_NAME=demo\n"}, "proc.test", "process environment", "proc.test"},
	}
	for _, tc := range layers {
		t.Run(tc.name, func(t *testing.T) {
			envExplainProject(t, tc.files)
			if tc.proc != "" {
				t.Setenv("BASE_DOMAIN", tc.proc)
			}
			out, _ := pilotBuffers(t)

			// Redacted, JSON: one envelope, value null, source named.
			if _, err := explainRun(t, "BASE_DOMAIN", "--json"); err != nil {
				t.Fatalf("explain --json: %v", err)
			}
			doc := singleDoc(t, out.String())
			if doc["command"] != "config explain" {
				t.Fatalf("command = %v", doc["command"])
			}
			data := doc["data"].(map[string]any)
			eff := data["effective"].(map[string]any)
			if eff["source"] != tc.want {
				t.Errorf("effective source = %v, want %s", eff["source"], tc.want)
			}
			if eff["value"] != nil || data["revealed"] != false {
				t.Errorf("value must be redacted without --reveal: %v", eff)
			}
			if strings.Contains(out.String(), tc.value) && tc.value != "" {
				t.Errorf("envelope leaks the value %q:\n%s", tc.value, out.String())
			}
			if data["env_name"] != "BASE_DOMAIN" || data["nself_yaml"] == "" {
				t.Errorf("env_name/nself_yaml missing: %v", data)
			}
			flags := data["flags"].([]any)
			f0 := flags[0].(map[string]any)
			if f0["command"] != "nself init" || f0["flag"] != "domain" || f0["mcp_tool"] != "nself_init" ||
				f0["mcp_parameter"] != "flags.domain" || f0["http_route"] != "/v1/commands/init" {
				t.Errorf("init --domain surface fields = %v", f0)
			}

			// Revealed human output shows the winning value.
			human, err := explainRun(t, "BASE_DOMAIN", "--reveal")
			if err != nil {
				t.Fatalf("explain --reveal: %v", err)
			}
			if !strings.Contains(human, tc.value) || !strings.Contains(human, "not a source") {
				t.Errorf("human output lacks the value or the nself.yaml row:\n%s", human)
			}
			redacted, err := explainRun(t, "BASE_DOMAIN")
			if err != nil {
				t.Fatalf("explain: %v", err)
			}
			if strings.Contains(redacted, tc.value) {
				t.Errorf("human output leaks %q without --reveal:\n%s", tc.value, redacted)
			}
		})
	}
}

func TestConfigExplainByFlag(t *testing.T) {
	t.Setenv("NSELF_LEGACY_ENV_ORDER", "")
	envExplainProject(t, map[string]string{".env": "BASE_DOMAIN=env.test\n"})
	out, _ := pilotBuffers(t)
	if _, err := explainRun(t, "init", "--domain", "--json"); err != nil {
		t.Fatalf("explain init --domain: %v", err)
	}
	data := singleDoc(t, out.String())["data"].(map[string]any)
	if data["key"] != "BASE_DOMAIN" || data["resolved_from"] != "init --domain" {
		t.Errorf("flag form = key %v resolved_from %v", data["key"], data["resolved_from"])
	}
}

func TestConfigExplainErrors(t *testing.T) {
	t.Setenv("NSELF_LEGACY_ENV_ORDER", "")
	envExplainProject(t, map[string]string{".env": "PROJECT_NAME=demo\n"})
	pilotBuffers(t)
	cases := [][]string{
		{"NOT_A_REAL_KEY_XYZ"},         // unknown key
		{"init", "--no-such-flag"},     // flag the command does not have
		{"exec", "--env"},              // exempt flag: not a key
		{"no-such-command", "--debug"}, // not a command
	}
	for _, args := range cases {
		_, err := explainRun(t, args...)
		if err == nil {
			t.Errorf("%v: want an error", args)
			continue
		}
		if c := explainCode(t, err); c != "E434" {
			t.Errorf("%v: code %s, want E434 (%v)", args, c, err)
		}
	}
	if _, err := explainRun(t); err == nil || explainCode(t, err) != "E401" {
		t.Errorf("no arguments: want E401, got %v", err)
	}
}

func TestConfigExplainReusesEnvExplain(t *testing.T) {
	// `config env explain VAR` and `config explain VAR` share config.ExplainKey,
	// so the winner they report is the same file.
	t.Setenv("NSELF_LEGACY_ENV_ORDER", "")
	envExplainProject(t, map[string]string{".env": "BASE_DOMAIN=a\n", ".env.local": "BASE_DOMAIN=b\n"})
	pilotBuffers(t)
	old, err := captureStdout(t, func() error { return runEnvExplain(envExplainRoot(t), []string{"BASE_DOMAIN"}) })
	if err != nil {
		t.Fatal(err)
	}
	cur, err := explainRun(t, "BASE_DOMAIN")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(old, ".env.local wins") || !strings.Contains(cur, ".env.local wins") {
		t.Errorf("winners differ:\n--- env explain\n%s\n--- config explain\n%s", old, cur)
	}
}
