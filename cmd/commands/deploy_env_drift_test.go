package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/spf13/cobra"
)

func TestDeployEnvPreviewFlag(t *testing.T) {
	if deployCmd.Flags().Lookup("preview") == nil {
		t.Fatal("deploy --preview must offer a read-only host .env key comparison")
	}
}

type envDriftRemote struct{ calls int }

func (r *envDriftRemote) Run(_ context.Context, command string) ([]byte, error) {
	r.calls++
	if !strings.Contains(command, "cat .env") {
		panic("preview attempted a write")
	}
	return []byte("SAME=equal\nCHANGED=host-secret\nHOST_ONLY=private\n"), nil
}
func (r *envDriftRemote) Rsync(context.Context, []string, string, string, string) error {
	panic("preview attempted rsync")
}
func (r *envDriftRemote) Label() string { return "test-host" }

func TestDeployEnvPreviewAndDoctorNamesOnly(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	isolateDeployEnv(t)
	compattest.Set(t, true)
	t.Setenv("NSELF_JSON_LEGACY", "")
	t.Setenv("NSELF_DEPLOY_HOST_STAGING", "u@host.example.test:/opt/nself")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SAME=equal\nCHANGED=local-secret\nLOCAL_ONLY=private-local\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := &envDriftRemote{}
	old := deployRemoteOpen
	deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) { return r, nil }
	t.Cleanup(func() { deployRemoteOpen = old })
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().AddFlagSet(deployCmd.Flags())
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("preview", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Flags().Set("preview", "false") })
	if err := runDeployBody(cmd, []string{"staging"}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"CHANGED", "LOCAL_ONLY", "HOST_ONLY"} {
		if !strings.Contains(out.String(), key) {
			t.Fatalf("missing key %s: %s", key, &out)
		}
	}
	if strings.Contains(out.String(), "secret") || strings.Contains(out.String(), "private") {
		t.Fatalf("leaked value: %s", &out)
	}
	checks := remoteEnvChecks(context.Background(), dir, "staging", "")
	if len(checks) != 1 || checks[0].Status != "warn" || !strings.Contains(out.String(), checks[0].Message) {
		t.Fatalf("doctor disagrees: %+v", checks)
	}
	deep := doctorRemoteEnvDeepChecks(context.Background(), dir, "staging", "config", false, nil)
	if len(deep) != 1 || deep[0].Message != checks[0].Message {
		t.Fatalf("deep doctor disagrees: %+v", deep)
	}
	if got := doctorRemoteEnvDeepChecks(context.Background(), dir, "staging", "database", false, nil); len(got) != 0 {
		t.Fatal("ignored --only")
	}
	if r.calls != 3 {
		t.Fatalf("expected three read-only comparisons, got %d", r.calls)
	}
	if got := remoteEnvChecks(context.Background(), dir, "staging", "other-server"); len(got) != 0 || r.calls != 3 {
		t.Fatal("server selector contacted an unselected host")
	}
	if got := remoteEnvChecks(context.Background(), dir, "local", ""); len(got) != 0 || r.calls != 3 {
		t.Fatal("local diagnostics contacted a remote host")
	}
	for _, deep := range []bool{false, true} {
		for _, display := range []bool{false, true} {
			stderr := captureStderr(t, func() {
				if deep {
					doctorRemoteEnvDeepChecks(context.Background(), dir, "staging", "config", display, nil)
				} else {
					checkDoctorRemoteEnv(context.Background(), dir, "staging", display)
				}
			})
			if display && !strings.Contains(stderr, checks[0].Message) || !display && stderr != "" || strings.Contains(stderr, "secret") {
				t.Fatalf("doctor display=%v deep=%v: %s", display, deep, stderr)
			}
		}
	}
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Flags().Set("json", "false") })
	jsonOut, err := captureStdout(t, func() error { return runDeployEnvPreview(cmd, dir, "staging", "") })
	if err != nil || !strings.Contains(jsonOut, "CHANGED") || strings.Contains(jsonOut, "secret") || strings.Contains(jsonOut, "private") {
		t.Fatalf("unsafe JSON preview: %s %v", jsonOut, err)
	}
	doc := oneEnvelope(t, jsonOut, "deploy")
	var data deployData
	if err := json.Unmarshal(doc["data"], &data); err != nil || !data.DryRun || data.Strategy != "preview" || data.Target != "staging" || len(data.Steps) != 1 {
		t.Fatalf("invalid preview data: %+v %v", data, err)
	}
}

func TestDeployEnvPreviewCLIOnly(t *testing.T) {
	reg, err := BuildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	row, ok := reg.Lookup("nself deploy")
	if !ok {
		t.Fatal("missing deploy registry row")
	}
	for _, transport := range []string{invoke.TransportHTTP, invoke.TransportMCP} {
		_, err := invoke.BuildArgvFor(row, invoke.Request{Flags: map[string]any{"preview": true}}, transport)
		if err == nil {
			t.Fatalf("%s accepted CLI-only preview flag", transport)
		}
	}
}
