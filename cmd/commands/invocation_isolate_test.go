package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

// isolationFixture runs a decorated command with the selected registry class.
func isolationFixture(t *testing.T, kind string, plugin bool, body func()) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "nself", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("json", false, "json")
	cmd := &cobra.Command{Use: "fixture", RunE: func(*cobra.Command, []string) error { body(); return nil }}
	if plugin {
		cmd.Annotations = map[string]string{"nself.plugin": "fixture", "nself.mount.source": "installed"}
	}
	root.AddCommand(cmd)
	old := jsonEntryFor
	jsonEntryFor = func(*cobra.Command) (*cmdregistry.Command, error) {
		return &cmdregistry.Command{JSON: kind}, nil
	}
	t.Cleanup(func() { jsonEntryFor = old; output.ResetState() })
	installInvocationDecorator(root)
	return root
}

// TestInvocationIsolatesEnvelope proves incidental UI text cannot precede the
// envelope and that plain human output still reaches stdout.
func TestInvocationIsolatesEnvelope(t *testing.T) {
	real := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = real; r.Close(); w.Close() }()
	root := isolationFixture(t, canon.JSONEnvelope, false, func() {
		ui.Info("planted notice")
		if err := output.EmitData(output.Default(), "fixture", map[string]string{"ok": "yes"}); err != nil {
			t.Fatal(err)
		}
	})
	root.SetArgs([]string{"fixture", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc["command"] != "fixture" {
		t.Fatalf("wrong document: %v", doc)
	}
	var extra any
	if err := json.NewDecoder(r).Decode(&extra); err == nil {
		t.Fatal("extra stdout JSON")
	}
	if os.Stdout != w {
		t.Fatal("stdout not restored after command")
	}
	humanR, humanW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer humanR.Close()
	defer humanW.Close()
	os.Stdout = humanW
	humanRoot := isolationFixture(t, canon.JSONEnvelope, false, func() {
		fmt.Fprint(os.Stdout, "human output\n")
	})
	humanRoot.SetArgs([]string{"fixture"})
	if err := humanRoot.Execute(); err != nil {
		t.Fatal(err)
	}
	if os.Stdout != humanW {
		t.Fatal("human stdout was redirected")
	}
	if err := humanW.Close(); err != nil {
		t.Fatal(err)
	}
	humanBytes, err := io.ReadAll(humanR)
	if err != nil {
		t.Fatal(err)
	}
	if string(humanBytes) != "human output\n" {
		t.Fatalf("human output missed real stdout: %q", humanBytes)
	}
}

// TestInvocationDecoratorRaceWindow runs the decorator path under -race. The
// fixture stays on one goroutine for the entire stdout swap window.
func TestInvocationDecoratorRaceWindow(t *testing.T) {
	before := os.Stdout
	root := isolationFixture(t, canon.JSONEnvelope, false, func() {
		if os.Stdout != os.Stderr {
			t.Error("decorator did not isolate stdout")
		}
	})
	root.SetArgs([]string{"fixture", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if os.Stdout != before {
		t.Fatal("decorator did not restore stdout")
	}
}

// TestInvocationSkipsLegacyJSON preserves bare legacy stdout in both modes.
func TestInvocationSkipsLegacyJSON(t *testing.T) {
	old := os.Stdout
	root := isolationFixture(t, canon.JSONLegacy, false, func() {
		if os.Stdout != old {
			t.Error("legacy stdout redirected")
		}
		fmt.Fprintln(os.Stdout, `{"legacy":true}`)
	})
	root.SetArgs([]string{"fixture", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestInvocationSkipsPluginProxy proves installed mounts own their stdout.
func TestInvocationSkipsPluginProxy(t *testing.T) {
	old := os.Stdout
	root := isolationFixture(t, canon.JSONEnvelope, true, func() {
		if os.Stdout != old {
			t.Error("plugin stdout redirected")
		}
	})
	root.SetArgs([]string{"fixture", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}
