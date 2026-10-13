package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/postgres/imageswitch"
	"github.com/spf13/cobra"
)

// imgRuntime is a docker whose postgres is not running (or is already pgvector).
type imgRuntime struct {
	image   string
	running bool
}

func (r imgRuntime) Inspect(context.Context, string) (imageswitch.ContainerInfo, error) {
	return imageswitch.ContainerInfo{Image: r.image, Running: r.running, DataVolume: "p_postgres_data", Networks: []string{"p_network"}}, nil
}
func (imgRuntime) ExecSQL(context.Context, string, string, string, string) (string, error) {
	return "", nil
}
func (imgRuntime) RunOneShot(context.Context, docker.RunSpec) (string, string, error) {
	return "", "", nil
}
func (imgRuntime) RemoveContainer(context.Context, string) error                 { return nil }
func (imgRuntime) VolumeCreate(context.Context, string, map[string]string) error { return nil }
func (imgRuntime) VolumeExists(context.Context, string) (bool, error)            { return false, nil }
func (imgRuntime) VolumeRemove(context.Context, string) error                    { return nil }

// imageSwitchFixture runs the command against a fake project with the given runtime.
func imageSwitchFixture(t *testing.T, rt imageswitch.Runtime, flags map[string]string) (string, error) {
	t.Helper()
	restoreProcessEnv(t)
	makeNSelfProject(t, "PROJECT_NAME=p\nPOSTGRES_PASSWORD=FAKE-IMAGE-SWITCH-POSTGRES-PASSWORD-00000\nHASURA_GRAPHQL_ADMIN_SECRET=FAKE-IMAGE-SWITCH-HASURA-ADMIN-SECRET-000000\n")
	old := imageSwitchDepsFor
	t.Cleanup(func() { imageSwitchDepsFor = old })
	imageSwitchDepsFor = func(*cobra.Command, string, bool) imageswitch.Deps { return imageswitch.Deps{Runtime: rt} }
	var out bytes.Buffer
	dbImageSwitchCmd.SetOut(&out)
	dbImageSwitchCmd.SetErr(&out)
	dbImageSwitchCmd.SetContext(context.Background())
	for name, v := range flags {
		if err := dbImageSwitchCmd.Flags().Set(name, v); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for name := range flags {
			f := dbImageSwitchCmd.Flags().Lookup(name)
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	})
	err := runDBImageSwitch(dbImageSwitchCmd, nil)
	return out.String(), err
}

func TestDBImageSwitchCommand(t *testing.T) {
	if dbImageSwitchCmd.Parent() != dbImageCmd || dbImageCmd.Parent() != dbCmd {
		t.Fatal("db image switch is not under db image")
	}
	for _, flag := range []string{"to", "plan", "yes", "resume", "rollback", "json"} {
		if dbImageSwitchCmd.Flags().Lookup(flag) == nil {
			t.Fatal("missing flag", flag)
		}
	}
	t.Run("--to pgvector is required", func(t *testing.T) {
		_, err := imageSwitchFixture(t, imgRuntime{}, nil)
		if d := errs.Describe(err); d == nil || d.Code != "E401" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("--rollback excludes --plan", func(t *testing.T) {
		_, err := imageSwitchFixture(t, imgRuntime{}, map[string]string{"rollback": "true", "plan": "true"})
		if d := errs.Describe(err); d == nil || d.Code != "E401" {
			t.Fatalf("err = %v", err)
		}
	})
	if runtime.GOOS == "windows" {
		t.Skip("the image switch refuses native Windows in its preflight (E509)")
	}
	t.Run("a run without --yes is refused when not interactive", func(t *testing.T) {
		_, err := imageSwitchFixture(t, imgRuntime{image: "postgres:16-alpine", running: true}, map[string]string{"to": "pgvector"})
		if d := errs.Describe(err); d == nil || d.Code != "E403" || !strings.Contains(d.Remediation, "--yes") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("plan shows the failed check and E509", func(t *testing.T) {
		out, err := imageSwitchFixture(t, imgRuntime{image: "postgres:16-alpine", running: false}, map[string]string{"to": "pgvector", "plan": "true"})
		if d := errs.Describe(err); d == nil || d.Code != "E509" {
			t.Fatalf("err = %v", err)
		}
		if !strings.Contains(out, "[FAILED] postgres running") {
			t.Errorf("the plan must print the failed check:\n%s", out)
		}
	})
	t.Run("already pgvector changes nothing and succeeds", func(t *testing.T) {
		out, err := imageSwitchFixture(t, imgRuntime{image: "pgvector/pgvector:pg16", running: true}, map[string]string{"to": "pgvector", "yes": "true"})
		if err != nil || !strings.Contains(out, "already on pgvector") {
			t.Fatalf("out %q err %v", out, err)
		}
	})
	t.Run("rollback with no switch says so", func(t *testing.T) {
		_, err := imageSwitchFixture(t, imgRuntime{}, map[string]string{"rollback": "true", "yes": "true"})
		if err == nil || !strings.Contains(err.Error(), "no image switch to roll back") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestPgvectorImageForMajor(t *testing.T) {
	img, err := pgvectorImageForMajor(16)
	if err != nil || !strings.Contains(img, "pgvector") || !strings.Contains(img, "pg16") {
		t.Fatalf("image = %q, err = %v", img, err)
	}
	if _, err := pgvectorImageForMajor(10); err == nil {
		t.Error("Postgres 10 has no pgvector image")
	}
}

func TestImageSwitchGuard(t *testing.T) {
	restoreProcessEnv(t)
	dir := makeNSelfProject(t, "PROJECT_NAME=p\n")
	oldEntry := jsonEntryFor
	t.Cleanup(func() { jsonEntryFor = oldEntry })
	sides := map[string]string{"nself start": canon.SideEffectWrite, "nself deploy": canon.SideEffectRemote, "nself drop": canon.SideEffectDestructive, "nself status": canon.SideEffectRead}
	jsonEntryFor = func(cmd *cobra.Command) (*cmdregistry.Command, error) {
		side, ok := sides[cmd.CommandPath()]
		if !ok {
			side = canon.SideEffectDestructive // db image switch itself is destructive and must stay allowed
		}
		return &cmdregistry.Command{SideEffect: side, Output: canon.OutputDocument}, nil
	}
	root := &cobra.Command{Use: "nself"}
	start := &cobra.Command{Use: "start"}
	deploy := &cobra.Command{Use: "deploy"}
	drop := &cobra.Command{Use: "drop"}
	status := &cobra.Command{Use: "status"}
	db := &cobra.Command{Use: "db"}
	image := &cobra.Command{Use: "image"}
	sw := &cobra.Command{Use: "switch"}
	image.AddCommand(sw)
	db.AddCommand(image)
	root.AddCommand(start, deploy, drop, status, db)

	if err := imageSwitchGuard(start); err != nil {
		t.Fatalf("no switch in progress, got %v", err)
	}
	state := filepath.Join(dir, ".nself", "image-switch", "state.json")
	if err := os.MkdirAll(filepath.Dir(state), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state, []byte(`{"version":1,"phase":"switched"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	err := imageSwitchGuard(start)
	d := errs.Describe(err)
	if d == nil || d.Code != "E512" || !strings.Contains(d.Remediation, "--resume") || !strings.Contains(d.Remediation, "--rollback") {
		t.Fatalf("an incomplete switch must block a write command with E512 naming both ways out: %v", err)
	}
	for _, c := range []*cobra.Command{deploy, drop} {
		if d := errs.Describe(imageSwitchGuard(c)); d == nil || d.Code != "E512" {
			t.Errorf("a remote or destructive command must be blocked too: %s", c.Name())
		}
	}
	if err := imageSwitchGuard(status); err != nil {
		t.Errorf("a read command is never blocked: %v", err)
	}
	if err := imageSwitchGuard(sw); err != nil {
		t.Errorf("db image switch is the way out and is never blocked: %v", err)
	}
	if err := imageSwitchGuard(root); err != nil {
		t.Errorf("the root command is never blocked: %v", err)
	}
	// A finished switch blocks nothing.
	if err := os.WriteFile(state, []byte(`{"version":1,"phase":"done"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := imageSwitchGuard(start); err != nil {
		t.Errorf("a done switch must not block: %v", err)
	}
	// And enterInvocation applies the guard to a command that has a body.
	if err := os.WriteFile(state, []byte(`{"version":1,"phase":"dumped"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := enterInvocation(start); errs.Describe(err) == nil || errs.Describe(err).Code != "E512" {
		t.Errorf("enterInvocation must apply the guard: %v", err)
	}
}
