package commands

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/plugin/mount"
	"github.com/spf13/cobra"
)

func TestD0614ProjectAIOptOut(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("AI_AUTO_INSTALL", "")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=fx\nENV=dev\nAI_AUTO_INSTALL=false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := stubAIInstall(t, true, false)
	autoInstallAIIfNeeded(context.Background())
	if *calls != 0 {
		t.Fatalf("project opt-out ran installer %d times", *calls)
	}
}

func TestD0614OpsOnlyShippedCompose(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".nself"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".nself", "compose-files.txt"), []byte("docker-compose.yml\nplugin.yml\n"), 0600); err != nil {
		t.Fatal(err)
	}
	orig := opsControlplaneCheck
	t.Cleanup(func() { opsControlplaneCheck = orig })
	base := filepath.Join(dir, "docker-compose.yml")
	opsControlplaneCheck = func(_ context.Context, _ *controlplane.Inventory, _ string, files []string) error {
		if !reflect.DeepEqual(files, []string{base}) {
			t.Errorf("preflight files = %v, deploy ships only %s", files, base)
		}
		return nil
	}
	if err := opsPreflight(context.Background(), dir, "u@ops.example.test:/opt/nself", base); err != nil {
		t.Fatal(err)
	}
}

func TestD0614InstalledProxyJSONIntent(t *testing.T) {
	defer output.ResetState()
	for _, args := range [][]string{{"--json"}, {"--json", "--json=false"}, {"--", "--json"}} {
		output.ResetState()
		root := &cobra.Command{Use: "nself", SilenceErrors: true, SilenceUsage: true}
		root.PersistentFlags().Bool("json", false, "json")
		root.AddGroup(&cobra.Group{ID: groupPlugins, Title: "Plugins:"})
		root.AddCommand(installedNode(mount.Spec{Command: "fixture", Slug: "fixture", BinaryPath: filepath.Join(t.TempDir(), "missing")}))
		installInvocationDecorator(root)
		root.SetArgs(append([]string{"fixture"}, args...))
		if err := root.Execute(); err == nil {
			t.Fatal("missing binary unexpectedly started")
		}
		command, on, known := output.Invocation()
		if !known || command != "fixture" || on != output.JSONRequestedFromArgs(args) {
			t.Fatalf("args=%v invocation=(%q,%v,%v)", args, command, on, known)
		}
	}
}
