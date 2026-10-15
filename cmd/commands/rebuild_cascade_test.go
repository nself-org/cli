package commands

// Purpose: D-0752 cascade regressions for automatic rebuild and plugin disable.
// Inputs: isolated projects and a read-only Docker shim. Outputs: refusal and
// unchanged artifacts, followed by an explicitly confirmed build.
// Constraints: no daemon, network, or real plugin state is used.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

func TestRebuildCascade(t *testing.T) {
	callers := []struct {
		name string
		run  func(string) error
	}{
		{"start_up", func(dir string) error { _, err := autoRebuildIfNeeded(dir, startOpts{}); return err }},
		{"restart_all", func(dir string) error {
			return restartFull(context.Background(), docker.NewCompose(), dir, nil, false, false, 1, 2)
		}},
		{"restart_smart", func(dir string) error {
			return restartSmart(context.Background(), docker.NewCompose(), dir, nil, false, false)
		}},
		{"plugin_disable", func(string) error { return runPluginDisable(&cobra.Command{}, []string{"demo"}) }},
	}
	for _, scenario := range []string{"prod_class_refusal", "hand_edit_refused_then_force"} {
		for _, caller := range callers {
			t.Run(scenario+"/"+caller.name, func(t *testing.T) {
				t.Cleanup(build.SnapshotEnv())
				isolateDoctorHome(t)
				t.Setenv("NSELF_V15", "1")
				t.Setenv("ENV", "")
				dir := t.TempDir()
				secrets, err := os.ReadFile(filepath.Join(l03Fixtures, "secrets.env"))
				if err != nil {
					t.Fatal(err)
				}
				t.Chdir(dir)
				env := "PROJECT_NAME=fx\nENV=dev\nBASE_DOMAIN=example.test\nSSL_MODE=none\nMINIO_ENABLED=false\n" + string(secrets)
				l03Write(t, filepath.Join(dir, ".env"), env, 0o600)
				cmd := &cobra.Command{}
				if _, err := runBuildApply(cmd, dir, build.BuildOptions{}, buildPlanFlags{yes: true}, false, false, true); err != nil {
					t.Fatal(err)
				}
				if scenario == "prod_class_refusal" {
					env = strings.ReplaceAll(env, "ENV=dev", "ENV=prod")
				}
				env = strings.ReplaceAll(env, "example.test", "changed.test")
				l03Write(t, filepath.Join(dir, ".env"), env, 0o600)
				conf := filepath.Join(dir, "nginx", "sites", "auth.conf")
				if scenario == "hand_edit_refused_then_force" {
					body, err := os.ReadFile(conf)
					if err != nil {
						t.Fatal(err)
					}
					l03Write(t, conf, string(body)+"\n# operator edit\n", 0o644)
				}
				pluginPath := filepath.Join(os.Getenv("NSELF_PLUGIN_DIR"), "demo")
				if err := os.MkdirAll(pluginPath, 0o755); err != nil {
					t.Fatal(err)
				}
				stub := t.TempDir()
				l03Write(t, filepath.Join(stub, "docker"), rebuildDockerStub, 0o755)
				t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
				before := doctorTreeHash(t, dir)
				dockerAt, lookErr := exec.LookPath("docker")
				psOut, psErr := docker.NewCompose().ComposePs(context.Background(), dir)
				t.Logf("DIAG docker=%s lookErr=%v ps=%v psErr=%v ENV=%q", dockerAt, lookErr, psOut, psErr, os.Getenv("ENV"))
				err = caller.run(dir)
				if detail := errs.Describe(err); detail == nil || detail.Code != "E403" {
					t.Errorf("got %v, want E403", err)
				}
				if doctorTreeHash(t, dir) != before {
					t.Error("refusal changed project artifacts")
				}
				if _, err := os.Stat(filepath.Join(pluginPath, ".disabled")); !os.IsNotExist(err) {
					t.Error("refusal wrote disable marker")
				}
				if t.Failed() {
					return
				}
				if _, err := runBuildApply(cmd, dir, build.BuildOptions{}, buildPlanFlags{yes: true}, true, false, true); err != nil {
					t.Fatal(err)
				}
				body, err := os.ReadFile(filepath.Join(dir, "docker-compose.yml"))
				if err != nil || !strings.Contains(string(body), "changed.test") {
					t.Fatalf("confirmed build did not apply domain: %v", err)
				}
				body, err = os.ReadFile(conf)
				if err != nil || strings.Contains(string(body), "# operator edit") {
					t.Fatalf("forced build did not replace hand edit: %v", err)
				}
				if caller.name == "start_up" {
					before = doctorTreeHash(t, dir)
					if _, err := autoRebuildIfNeeded(dir, startOpts{}); err != nil {
						t.Fatal(err)
					}
					if doctorTreeHash(t, dir) != before {
						t.Fatal("start after confirmed build changed artifacts")
					}
				}
				if caller.name == "plugin_disable" {
					before = doctorTreeHash(t, dir)
					cmd.Flags().Bool("yes", true, "")
					cmd.Flags().Bool("force", true, "")
					if err := runPluginDisable(cmd, []string{"demo"}); err != nil {
						t.Fatal(err)
					}
					if _, err := os.Stat(filepath.Join(pluginPath, ".disabled")); err != nil {
						t.Fatal(err)
					}
					if doctorTreeHash(t, dir) != before {
						t.Fatal("disable preflight changed generated artifacts")
					}
				}
			})
		}
	}
}

const rebuildDockerStub = `#!/bin/sh
if [ "$1" = inspect ] && [ "$3" = '{{json .}}' ]; then
  printf '%s\n' '{"Name":"/fx-postgres","State":{"Status":"running","StartedAt":"2000-01-01T00:00:00Z"}}'
  exit 0
fi
case " $* " in
  *" ps "*) printf '%s\n' '[{"Name":"fx-postgres","Service":"postgres","State":"running"}]'; exit 0 ;;
  *" --hash "*) exit 0 ;;
esac
echo 'mutation blocked by test shim' >&2
exit 1
`
