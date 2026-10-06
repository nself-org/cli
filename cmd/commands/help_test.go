package commands

// Tests for the custom help command (help.go): `help --json [path...]`, the
// byte-identical human path, completion and installation.

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// helpEnvelope is the part of the envelope the tests read.
type helpEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Command       string `json:"command"`
	Data          struct {
		Verbs    []string `json:"verbs"`
		Commands []struct {
			Path string `json:"path"`
		} `json:"commands"`
		Counts struct {
			Commands    int      `json:"commands"`
			TopLevel    int      `json:"top_level"`
			Core        int      `json:"core"`
			CoreMissing []string `json:"core_missing"`
		} `json:"counts"`
	} `json:"data"`
	Error json.RawMessage `json:"error"`
}

// realHelp returns the real help command with --json parsed (or not), and
// restores the flag when the test ends. Nothing runs but parsing.
func realHelp(t *testing.T, jsonOn bool) *cobra.Command {
	t.Helper()
	reattachRealTree()
	RootCmd.InitDefaultHelpCmd()
	c, _, err := RootCmd.Find([]string{"help"})
	if err != nil || c == nil || c == RootCmd {
		t.Fatalf("help command not found: %v", err)
	}
	if jsonOn {
		if err := c.ParseFlags([]string{"--json"}); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Name == "json" {
				_ = f.Value.Set("false")
				f.Changed = false
			}
		})
	})
	return c
}

// runJSON runs `help --json args...` and returns stdout and the error.
func runJSON(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out, errBuf bytes.Buffer
	old := helpWriter
	helpWriter = func() output.Writer { return output.Writer{Out: &out, Err: &errBuf} }
	t.Cleanup(func() { helpWriter = old })
	resetRegistryCache()
	t.Cleanup(resetRegistryCache)
	err := runHelp(realHelp(t, true), args)
	if errBuf.Len() != 0 {
		t.Errorf("help --json wrote to stderr: %q", errBuf.String())
	}
	return out.String(), err
}

func decodeHelp(t *testing.T, s string) helpEnvelope {
	t.Helper()
	var env helpEnvelope
	if err := json.Unmarshal([]byte(s), &env); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, s)
	}
	return env
}

func inventoryLen(t *testing.T) int {
	t.Helper()
	b, err := os.ReadFile("../../.github/command-inventory.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(b, &entries); err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

func TestHelpJSONEmitsRegistryEnvelope(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		out, err := runJSON(t)
		if err != nil {
			t.Fatal(err)
		}
		env := decodeHelp(t, out)
		if env.SchemaVersion != "1" || env.Command != "help" || env.Error != nil {
			t.Fatalf("bad envelope header: %+v", env)
		}
		d := env.Data
		// The committed inventory documents the v1.5 surface (EPIC D3), so
		// v1.5 must match it exactly. In v1.4 the commands a move relocates are
		// still visible at their old top-level paths while v1.5 hides their
		// stubs, so the v1.4 count is higher by the names visible at the top
		// in v1.4 but not in v1.5 (a shim already hidden in both modes, like
		// uninstall, counts in neither).
		want := inventoryLen(t)
		if !compat.V15() {
			reg14, err := BuildRegistry(false)
			if err != nil {
				t.Fatal(err)
			}
			reg15, err := BuildRegistry(true)
			if err != nil {
				t.Fatal(err)
			}
			visibleTop := func(r *cmdregistry.Registry) map[string]bool {
				top := map[string]bool{}
				for _, c := range r.Commands {
					if c.Parent == "nself" && !c.Hidden && c.Name != "help" {
						top[c.Name] = true
					}
				}
				return top
			}
			v14, v15 := visibleTop(reg14), visibleTop(reg15)
			extra := 0
			for name := range v14 {
				if !v15[name] {
					extra++
				}
			}
			want += extra
		}
		if d.Counts.TopLevel != want {
			t.Errorf("counts.top_level = %d, want %d (v1.5 inventory %d)", d.Counts.TopLevel, want, inventoryLen(t))
		}
		if d.Counts.Core+len(d.Counts.CoreMissing) != len(d.Verbs) {
			t.Errorf("core %d + missing %d != verbs %d", d.Counts.Core, len(d.Counts.CoreMissing), len(d.Verbs))
		}
		if d.Counts.Commands != len(d.Commands) {
			t.Errorf("counts.commands = %d, commands has %d", d.Counts.Commands, len(d.Commands))
		}
	})
}

func TestHelpJSONSubtree(t *testing.T) {
	out, err := runJSON(t, "config")
	if err != nil {
		t.Fatal(err)
	}
	env := decodeHelp(t, out)
	if len(env.Data.Commands) < 2 {
		t.Fatalf("config subtree has %d commands", len(env.Data.Commands))
	}
	for _, c := range env.Data.Commands {
		if c.Path != "nself config" && !strings.HasPrefix(c.Path, "nself config ") {
			t.Errorf("path %q outside the config subtree", c.Path)
		}
	}
	// A nested path, written as separate words, names only that command.
	out, err = runJSON(t, "config", "get")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range decodeHelp(t, out).Data.Commands {
		if !strings.HasPrefix(c.Path, "nself config get") {
			t.Errorf("path %q outside nself config get", c.Path)
		}
	}
}

func TestHelpJSONUnknownPathIsE401(t *testing.T) {
	for _, args := range [][]string{{"nosuchcmd"}, {"status", "bogus"}, {"nself"}} {
		out, err := runJSON(t, args...)
		var coded *errs.CLIError
		if !errors.As(err, &coded) || coded.Code != "E401" {
			t.Errorf("help --json %v: want E401, got %v", args, err)
		}
		if out != "" {
			t.Errorf("help --json %v wrote stdout on error: %q", args, out)
		}
	}
}

// The human path must be cobra's own help: same text, same unknown-topic
// message and usage.
func TestHelpTextMatchesCobra(t *testing.T) {
	c := realHelp(t, false)
	status, _, _ := RootCmd.Find([]string{"status"})

	var want, got bytes.Buffer
	RootCmd.SetOut(&got)
	t.Cleanup(func() { RootCmd.SetOut(nil) })
	if err := runHelp(c, []string{"status"}); err != nil {
		t.Fatal(err)
	}
	RootCmd.SetOut(&want)
	_ = status.Help()
	if got.String() != want.String() || got.Len() == 0 {
		t.Errorf("help status differs from cobra's Help()\n got: %q\nwant: %q", got.String(), want.String())
	}

	got.Reset()
	want.Reset()
	RootCmd.SetOut(&got)
	RootCmd.SetErr(&got)
	t.Cleanup(func() { RootCmd.SetErr(os.Stderr) })
	if err := runHelp(c, []string{"nosuchcmd"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.String(), "Unknown help topic [`nosuchcmd`]\n") || !strings.Contains(got.String(), "Usage:") {
		t.Errorf("unknown topic output = %q", got.String())
	}
}

func TestHelpCommandIsInstalledWithCobraMetadata(t *testing.T) {
	c := realHelp(t, false)
	if c.Use != "help [command]" || c.Short != "Help about any command" || c.RunE == nil || c.Hidden {
		t.Fatalf("unexpected help command: %+v", c)
	}
	if !strings.Contains(c.Long, "Simply type nself help [path to command]") {
		t.Errorf("Long = %q", c.Long)
	}
	if c.Parent() != RootCmd {
		t.Error("help is not a child of RootCmd")
	}
}

func TestHelpCompletionListsRootCommandsAndHelp(t *testing.T) {
	c := realHelp(t, false)
	comps, dir := helpCompletion(c, nil, "")
	if dir != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v", dir)
	}
	names := map[string]bool{}
	for _, s := range comps {
		names[strings.SplitN(s, "\t", 2)[0]] = true
	}
	if !names["help"] || !names["status"] {
		t.Errorf("completions miss help or status: %d entries", len(comps))
	}
	if sub, _ := helpCompletion(c, []string{"config"}, "g"); len(sub) == 0 {
		t.Error("no completions below `config`")
	}
	if none, d := helpCompletion(c, []string{"nosuchcmd"}, ""); none != nil || d != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("unknown parent: %v %v", none, d)
	}
}

// help is an envelope command in both modes (it is additive, ADR 0021 D8).
func TestHelpIsEnvelopeInBothModes(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		resetRegistryCache()
		t.Cleanup(resetRegistryCache)
		reg, err := commandRegistry()
		if err != nil {
			t.Fatal(err)
		}
		e, ok := reg.Lookup("nself help")
		if !ok || e.JSON != "envelope" || e.Canon != "builtin" {
			t.Fatalf("help entry = %+v (v15=%v)", e, compat.V15())
		}
		if refuseUnsupportedJSON(realHelp(t, true)) != nil {
			t.Error("help --json refused")
		}
	})
}
