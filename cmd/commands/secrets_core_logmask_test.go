package commands

// Purpose: prove commandSecretValues names the secret values of an invocation
// (D-0468 D-0473) from the canon, with a non-secret command as the control.
// Inputs: the live RootCmd tree and synthetic commands. Outputs: pass/fail.

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/spf13/cobra"
)

const fakeSecret0468 = "FAKE-planted-0468-value"

func findCmdForMask(t *testing.T, path ...string) *cobra.Command {
	t.Helper()
	c, _, err := RootCmd.Find(path)
	if err != nil || c == nil || c == RootCmd {
		t.Fatalf("command %v not found: %v", path, err)
	}
	return c
}

func TestCommandSecretValues_Positionals(t *testing.T) {
	cases := []struct {
		name string
		path []string
		args []string
		want []string
	}{
		{"secrets set", []string{"secrets", "set"}, []string{"FAKEKEY", fakeSecret0468}, []string{fakeSecret0468}},
		{"config set", []string{"config", "set"}, []string{"somekey", fakeSecret0468}, []string{fakeSecret0468}},
		{"secrets set, value omitted", []string{"secrets", "set"}, []string{"FAKEKEY"}, nil},
		{"non-secret command", []string{"secrets", "list"}, []string{"FAKEKEY"}, nil},
	}
	for _, c := range cases {
		cmd := findCmdForMask(t, c.path...)
		got := commandSecretValues(cmd, c.args)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v (command path %q)", c.name, got, c.want, cmd.CommandPath())
		}
	}
}

// TestCommandSecretValues_ConfigSecretsSet covers the v1.5 spelling, where the
// secrets hub sits under config; a synthetic tree stands in for the moved one.
func TestCommandSecretValues_ConfigSecretsSet(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	root := &cobra.Command{Use: "nself"}
	cfg := &cobra.Command{Use: "config"}
	sec := &cobra.Command{Use: "secrets"}
	set := &cobra.Command{Use: "set <KEY> [VALUE]"}
	sec.AddCommand(set)
	cfg.AddCommand(sec)
	root.AddCommand(cfg)
	if got := commandSecretValues(set, []string{"FAKEKEY", fakeSecret0468}); !reflect.DeepEqual(got, []string{fakeSecret0468}) {
		t.Errorf("config secrets set: got %v", got)
	}
	if got := commandSecretValues(set, []string{"FAKEKEY"}); got != nil {
		t.Errorf("value omitted: got %v", got)
	}
}

func TestCommandSecretValues_Flags(t *testing.T) {
	cmd := &cobra.Command{Use: "demo <name>"}
	cmd.Flags().String("api-token", "", "")
	cmd.Flags().String("note", "", "")
	cmd.Flags().StringSlice("secret-list", nil, "")
	cmd.Flags().Bool("skip-key-check", false, "")
	cmd.Flags().String("unset-password", "", "")
	for flag, v := range map[string]string{"api-token": fakeSecret0468, "note": "plain-note-text", "skip-key-check": "true"} {
		if err := cmd.Flags().Set(flag, v); err != nil {
			t.Fatal(err)
		}
	}
	_ = cmd.Flags().Set("secret-list", "FAKE-list-one,FAKE-list-two")
	got := commandSecretValues(cmd, []string{"x"})
	want := []string{fakeSecret0468, "FAKE-list-one", "FAKE-list-two"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("flags: got %v, want %v (note and bool flags and unset flags must be absent)", got, want)
	}
}

func TestCommandSecretValues_NeverFails(t *testing.T) {
	if got := commandSecretValues(nil, []string{"x"}); got != nil {
		t.Errorf("nil command: %v", got)
	}
	if got := commandSecretValues(&cobra.Command{Use: "orphan"}, []string{"x"}); got != nil {
		t.Errorf("command without canon entry: %v", got)
	}
}

func TestUseArgNames(t *testing.T) {
	cases := map[string][]string{
		"set <KEY> [VALUE]":   {"KEY", "VALUE"},
		"run <a> [b...]":      {"a", "b..."},
		"x [flags] <one>":     {"one"},
		"bare":                nil,
		"get [name]... --opt": {"name..."},
	}
	for use, want := range cases {
		if got := useArgNames(use); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v, want %v", use, got, want)
		}
	}
}

// TestExecuteRestoresSecretArgvFirst pins the call site: Execute refuses a bad
// secret bundle before it prepares anything, and the variable is gone after.
func TestExecuteRestoresSecretArgvFirst(t *testing.T) {
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	os.Args = []string{"nself", "version"}
	t.Setenv(invoke.InvokedByEnv, "mcp")
	t.Setenv(invoke.SecretsEnv, "{not json")
	err := Execute()
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E421" {
		t.Fatalf("Execute with a malformed bundle = %v, want E421", err)
	}
	if _, still := os.LookupEnv(invoke.SecretsEnv); still {
		t.Error("the secret bundle must be unset after Execute starts")
	}
}
