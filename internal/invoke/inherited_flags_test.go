package invoke

// Purpose: D-0835 regression. A flag a command inherits from a non-root
// parent's persistent set (config --env) must be settable by a machine request.

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/spf13/cobra"
)

// inheritRegistry is `config get` and `config show` under a parent that declares
// persistent env, reveal and a hidden flag; get redeclares reveal locally.
func inheritRegistry(t *testing.T) *cmdregistry.Registry {
	t.Helper()
	file, err := canon.Parse([]byte("schema_version: 1\nverbs: [config]\ncommands:\n  config: {canon: core}\n  config get: {side_effect: read}\n  config show: {side_effect: read}\n  help: {canon: builtin, side_effect: read}\n"))
	if err != nil {
		t.Fatal(err)
	}
	root := &cobra.Command{Use: "nself", Short: "fixture"}
	root.PersistentFlags().String("rootflag", "", "root persistent")
	cfg := &cobra.Command{Use: "config", Short: "config"}
	cfg.PersistentFlags().String("env", "", "Target environment")
	cfg.PersistentFlags().Bool("reveal", false, "reveal")
	cfg.PersistentFlags().String("secret-ish", "", "hidden")
	_ = cfg.PersistentFlags().MarkHidden("secret-ish")
	get := &cobra.Command{Use: "get <key>", Short: "get", RunE: noop}
	get.Flags().String("format", "", "local")
	show := &cobra.Command{Use: "show", Short: "show", RunE: noop}
	cfg.AddCommand(get, show)
	root.AddCommand(cfg)
	root.InitDefaultHelpCmd()
	reg, err := cmdregistry.Build(root, file, map[string]any{"config get": struct{}{}, "config show": struct{}{}}, cmdregistry.BuildOptions{V15: true})
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func flagNames(fs []cmdregistry.Flag) string {
	var n []string
	for _, f := range fs {
		n = append(n, f.Name)
	}
	return strings.Join(n, ",")
}

func TestInheritedPersistentFlagsExposed(t *testing.T) {
	reg := inheritRegistry(t)
	get := mustCmd(t, reg, "config get")
	merged := reg.WithInheritedFlags(get)
	if got := flagNames(ExposedFlags(merged, TransportMCP)); got != "env,format,reveal" {
		t.Errorf("exposed flags = %q, want env,format,reveal (no root, hidden or json flags)", got)
	}
	if got := flagNames(ExposedFlags(get, TransportMCP)); got != "format" {
		t.Errorf("the registry command itself must stay unchanged, got %q", got)
	}
	schema := ParamsSchemaIn(reg, get)
	props := schema["properties"].(map[string]any)["flags"].(map[string]any)["properties"].(map[string]any)
	if _, ok := props["env"]; !ok {
		t.Errorf("params schema has no flags.env: %v", props)
	}
}

func TestInvokeSetsInheritedFlag(t *testing.T) {
	useStub(t)
	reg := inheritRegistry(t)
	res, err := Invoke(context.Background(), reg, "config get", req(t, `{"args":["K"],"flags":{"env":"prod"}}`), Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != "" || !strings.Contains(string(res.Stdout), `"--env=prod"`) {
		t.Fatalf("code=%q stdout=%s", res.ErrorCode, res.Stdout)
	}
	for _, doc := range []string{`{"flags":{"rootflag":"x"}}`, `{"flags":{"secret-ish":"x"}}`} {
		res, err = Invoke(context.Background(), reg, "config show", req(t, doc), Options{Dir: t.TempDir()})
		if err != nil || res.ErrorCode != "E420" {
			t.Errorf("%s: err=%v code=%q, want E420", doc, err, res.ErrorCode)
		}
	}
}
