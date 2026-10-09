package cmdregistry

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/spf13/cobra"
)

func surfaceFixture(t *testing.T) (*cobra.Command, *canon.File) {
	t.Helper()
	b, err := os.ReadFile("../canon/testdata/surface/valid.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f, err := canon.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	r := &cobra.Command{Use: "nself"}
	reset := &cobra.Command{Use: "reset", RunE: noop}
	reset.Flags().Bool("yes", false, "")
	config := &cobra.Command{Use: "config"}
	config.AddCommand(&cobra.Command{Use: "export", RunE: noop}, &cobra.Command{Use: "set <key> <value>", RunE: noop})
	logs := &cobra.Command{Use: "logs", RunE: noop}
	logs.Flags().Bool("follow", false, "")
	logs.Flags().Bool("reveal", false, "")
	r.AddCommand(reset, config, logs)
	return r, f
}

func surfaceRegistry(t *testing.T) *Registry {
	t.Helper()
	r, f := surfaceFixture(t)
	reg, err := Build(r, f, nil, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestRegistryConfirmField(t *testing.T) {
	c, ok := surfaceRegistry(t).Lookup("reset")
	if !ok || c.Confirm == nil || len(c.Confirm.Flags) != 1 || c.Confirm.Flags[0] != "yes" || c.Confirm.Plan != nil {
		t.Fatalf("confirm: %+v", c)
	}
}

func TestRegistrySurfaceAndSecret(t *testing.T) {
	r := surfaceRegistry(t)
	export, _ := r.Lookup("config export")
	set, _ := r.Lookup("config set")
	config, _ := r.Lookup("config")
	if export.Surface != "cli-only" || config.Surface != "all" || len(set.Args) != 2 || !set.Args[1].Secret {
		t.Fatalf("surface/secret: %+v %+v %+v", export, config, set)
	}
}

func TestRegistryCLIOnlyFlag(t *testing.T) {
	c, _ := surfaceRegistry(t).Lookup("logs")
	for _, f := range c.Flags {
		if f.Name == "reveal" && !f.CLIOnly {
			t.Fatal("reveal is not cli_only")
		}
		if f.Name == "follow" && (f.Output == nil || *f.Output != "stream") {
			t.Fatal("follow is not stream")
		}
	}
}

func TestToolName(t *testing.T) {
	if got := ToolName("deploy promote-env"); got != "nself_deploy_promote_env" {
		t.Fatal(got)
	}
}
func TestRoutePath(t *testing.T) {
	if got := RoutePath("config get"); got != "/v1/commands/config/get" {
		t.Fatal(got)
	}
}
func TestEffectiveSideEffect(t *testing.T) {
	d := "destructive"
	c := &Command{SideEffect: "read", Flags: []Flag{{Name: "volumes", SideEffect: &d}}}
	if EffectiveSideEffect(c, map[string]bool{"volumes": true}) != d || EffectiveSideEffect(c, nil) != "read" {
		t.Fatal("escalation failed")
	}
	if EffectiveSideEffect(nil, nil) != "" {
		t.Fatal("nil command has an effective class")
	}
}
func TestEffectiveOutput(t *testing.T) {
	c, _ := surfaceRegistry(t).Lookup("logs")
	if EffectiveOutput(c, map[string]bool{"follow": true}) != "stream" || EffectiveOutput(c, nil) != "document" {
		t.Fatal("output override failed")
	}
	if EffectiveOutput(nil, nil) != "" {
		t.Fatal("nil command has effective output")
	}
}
func TestValidateToolNames(t *testing.T) {
	for _, paths := range [][]string{{"a-b c", "a b-c"}, {strings.Repeat("x", 65)}, {"BadName"}} {
		r := &Registry{}
		for _, p := range paths {
			r.Commands = append(r.Commands, Command{Path: p})
		}
		if ValidateToolNames(r) == nil {
			t.Fatalf("accepted %v", paths)
		}
	}
	if ValidateToolNames(nil) == nil {
		t.Fatal("nil registry accepted")
	}
}

func TestPluginSurfaceInvalid(t *testing.T) {
	r, f := surfaceFixture(t)
	plugin := &cobra.Command{Use: "broken", RunE: noop, Annotations: map[string]string{annPluginKey: "broken", annSourceKey: sourceInstalled, annConfirmKey: `{"flags":[]}`, annSurfaceKey: "sometimes"}}
	r.AddCommand(plugin)
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = writer
	reg, buildErr := Build(r, f, nil, BuildOptions{})
	os.Stderr = oldStderr
	_ = writer.Close()
	warnings, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if buildErr != nil || readErr != nil {
		t.Fatalf("build: %v; stderr: %v", buildErr, readErr)
	}
	c, _ := reg.Lookup("broken")
	if c.Confirm != nil || c.Surface != "all" || strings.Count(string(warnings), "E437") != 1 {
		t.Fatalf("invalid plugin declaration not isolated: %+v %q", c, warnings)
	}
}

func TestRegistrySurfaceInvalid(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*cobra.Command, *canon.File)
		want   string
	}{
		{"secret arg", func(_ *cobra.Command, f *canon.File) {
			e := f.Commands["config set"]
			e.SecretArgs = []string{"missing"}
			f.Commands["config set"] = e
		}, "secret_args"},
		{"confirm absent", func(_ *cobra.Command, f *canon.File) {
			e := f.Commands["reset"]
			e.Confirm = &canon.Confirm{Flags: []string{"missing"}}
			f.Commands["reset"] = e
		}, "confirm.flags"},
		{"confirm type", func(r *cobra.Command, f *canon.File) {
			c, _, _ := r.Find([]string{"reset"})
			c.Flags().String("text", "", "")
			e := f.Commands["reset"]
			e.Confirm = &canon.Confirm{Flags: []string{"text"}}
			f.Commands["reset"] = e
		}, "confirm.flags"},
		{"hub", func(_ *cobra.Command, f *canon.File) {
			e := f.Commands["config"]
			e.Confirm = &canon.Confirm{Flags: []string{}}
			f.Commands["config"] = e
		}, "not runnable"},
		{"id flag", func(r *cobra.Command, f *canon.File) {
			c, _, _ := r.Find([]string{"reset"})
			c.Flags().Bool("plan", false, "")
			c.Flags().Bool("id", false, "")
			e := f.Commands["reset"]
			e.Confirm = &canon.Confirm{Plan: &canon.PlanForm{Flag: "plan", IDFlag: "id"}}
			f.Commands["reset"] = e
		}, "must be string"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, f := surfaceFixture(t)
			tc.mutate(r, f)
			_, err := Build(r, f, nil, BuildOptions{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s: %v", tc.want, err)
			}
		})
	}
}

func TestPluginSurfaceFields(t *testing.T) {
	r, f := surfaceFixture(t)
	plugin := &cobra.Command{Use: "plug", RunE: noop, Annotations: map[string]string{annPluginKey: "plug", annSourceKey: sourceInstalled, annConfirmKey: `{"flags":[]}`, annSurfaceKey: "cli-only", annFlagsKey: `[{"name":"token","type":"string","secret":true,"cli_only":true}]`, annArgsKey: `[{"name":"value","secret":true}]`}}
	revoke := &cobra.Command{Use: "revoke", RunE: noop, Annotations: map[string]string{annPluginKey: "plug", annSourceKey: sourceInstalled, annSideKey: "destructive", annConfirmKey: `{"flags":["yes"]}`, annFlagsKey: `[{"name":"yes","type":"bool"}]`}}
	revoke.Flags().Bool("yes", false, "")
	remote := &cobra.Command{Use: "remote", RunE: noop, Annotations: map[string]string{annPluginKey: "plug", annSourceKey: sourceInstalled, annSideKey: "remote", annConfirmKey: `{"flags":[]}`}}
	plugin.AddCommand(revoke, remote)
	r.AddCommand(plugin)
	reg, err := Build(r, f, nil, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	c, _ := reg.Lookup("plug")
	if c.Confirm == nil || c.Surface != "cli-only" || !c.Flags[0].Secret || !c.Flags[0].CLIOnly || !c.Args[0].Secret {
		t.Fatalf("plugin fields: %+v", c)
	}
	rc, _ := reg.Lookup("plug revoke")
	mc, _ := reg.Lookup("plug remote")
	if rc.Confirm == nil || len(rc.Confirm.Flags) != 1 || rc.Confirm.Flags[0] != "yes" || rc.SideEffect != "destructive" || mc.Confirm == nil || len(mc.Confirm.Flags) != 0 || mc.SideEffect != "remote" {
		t.Fatalf("plugin confirmation: %+v %+v", rc, mc)
	}
	plugin.Annotations[annConfirmKey] = `{"flags":["missing"]}`
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = writer
	reg, err = Build(r, f, nil, BuildOptions{})
	os.Stderr = oldStderr
	_ = writer.Close()
	warnings, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	c, _ = reg.Lookup("plug")
	if c.Confirm != nil {
		t.Fatalf("invalid plugin confirm retained: %+v", c.Confirm)
	}
	if strings.Count(string(warnings), "E437") != 1 {
		t.Fatalf("want one E437 warning, got %q", warnings)
	}
}
