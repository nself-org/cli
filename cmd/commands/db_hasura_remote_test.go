package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
)

func archiveFixture(t *testing.T, value string) []byte {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "metadata")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tables.yaml"), []byte(value), 0644); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := database.Pack(dir, &b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestDBHasuraNoEnvGolden(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(original)
	for _, tc := range []struct {
		name         string
		old, wrapped func(*cobra.Command, []string) error
		cmd          *cobra.Command
	}{
		{"export", runDBHasuraMetadataExport, runDBHasuraMetadataExportArchive, dbHasuraMetadataExportCmd},
		{"apply", runDBHasuraMetadataApply, runDBHasuraMetadataApplyArchive, dbHasuraMetadataApplyCmd},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if archive, _ := tc.cmd.Flags().GetString("archive"); archive != "" {
				t.Fatalf("default archive=%q", archive)
			}
			if target, err := resolveDBRemoteTarget(tc.cmd); err != nil || !target.Local {
				t.Fatalf("default target=%+v %v", target, err)
			}
			// Both paths read the same missing project; compare exact error and bytes.
			oldErr, newErr := tc.old(tc.cmd, nil), tc.wrapped(tc.cmd, nil)
			if (oldErr == nil) != (newErr == nil) || (oldErr != nil && oldErr.Error() != newErr.Error()) {
				t.Fatalf("default changed: old=%v new=%v", oldErr, newErr)
			}
		})
	}
}

func TestDBHasuraApplyRefProdClass(t *testing.T) {
	current, desired := archiveFixture(t, "tables: []\n"), archiveFixture(t, "tables: [one]\n")
	for _, env := range []string{"prod", "staging"} {
		p, err := metadataArchivePlan(env, "deadbeef", current, desired)
		if err != nil {
			t.Fatal(err)
		}
		if p.Empty || len(p.Artifacts) != 1 || p.Artifacts[0].Kind != reconcile.KindHasura {
			t.Fatalf("bad plan: %+v", p)
		}
		var rendered bytes.Buffer
		if err := reconcile.RenderHuman(&rendered, p); err != nil || !strings.Contains(rendered.String(), "hasura") {
			t.Fatalf("missing plan: %s %v", rendered.String(), err)
		}
		err = reconcile.Confirm(p, reconcile.ApplyOptions{}, true, nil)
		if d := errs.Describe(err); d == nil || d.Code != "E403" || errs.ExitCodeFor(err) != 4 {
			t.Fatalf("want E403 exit 4: %v", err)
		}
		if err := reconcile.Confirm(p, reconcile.ApplyOptions{Yes: true}, true, nil); err != nil {
			t.Fatal(err)
		}
	}
	same, err := metadataArchivePlan("prod", "deadbeef", current, current)
	if err != nil || !same.Empty {
		t.Fatalf("equal metadata must be empty: %+v %v", same, err)
	}
}

func TestDBHasuraRemotePartialFailure(t *testing.T) {
	// A failed export cannot reach the apply step or mutate the local checkout.
	old := runSSHStream
	defer func() { runSSHStream = old }()
	calls := 0
	runSSHStream = func(_ context.Context, _ []string, _ io.Reader, _ io.Writer) (string, error) {
		calls++
		return "broken remote", errors.New("ssh failed")
	}
	rt := dbRemoteTarget{SSHTarget: "fixture.invalid", RemotePath: "/project", EnvName: "staging", AllowVersionDrift: true}
	cmd := &cobra.Command{}
	cmd.Flags().String("message", "", "")
	if err := runDBHasuraRemoteSync(cmd, rt); err == nil || calls != 1 {
		t.Fatalf("partial failure not stopped: %v calls=%d", err, calls)
	}
}

func TestDBHasuraRemoteOldCLIExplainsUpgrade(t *testing.T) {
	old := runSSHStream
	defer func() { runSSHStream = old }()
	runSSHStream = func(_ context.Context, _ []string, _ io.Reader, _ io.Writer) (string, error) {
		return "Error: unknown flag: --archive", errors.New("exit 1")
	}
	rt := dbRemoteTarget{SSHTarget: "fixture.invalid", RemotePath: "/project", EnvName: "staging", AllowVersionDrift: true}
	err := runRemoteNselfStream(context.Background(), rt, nil, io.Discard, "db", "hasura", "metadata", "export", "--archive", "-")
	if err == nil || !strings.Contains(err.Error(), "upgrade the remote CLI") {
		t.Fatalf("missing remediation: %v", err)
	}
}

func TestDBHasuraRemoteRejectsOversizedStream(t *testing.T) {
	var data bytes.Buffer
	w := &boundedMetadataWriter{w: &data, remaining: 3}
	if _, err := w.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("d")); err == nil || data.String() != "abc" {
		t.Fatalf("limit bypassed: %q %v", data.String(), err)
	}
}

func TestDBHasuraRemoteIntegration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux sshd bind-mount fixture")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker unavailable")
	}
	if out, err := exec.Command("docker", "info", "--format", "{{.ServerVersion}}").CombinedOutput(); err != nil {
		t.Skipf("docker unavailable: %s", out)
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0777); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(root, "id_ed25519")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	pub, err := os.ReadFile(key + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	fixture := archiveFixture(t, "tables: [remote]\n")
	if err := os.WriteFile(filepath.Join(root, "current.tar"), fixture, 0644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'nself v1.0.0'; exit 0; fi\nif [ \"$4\" = \"export\" ]; then cat /fixtures/current.tar; exit; fi\nif [ \"$4\" = \"apply\" ]; then cat > /fixtures/applied.tar; exit; fi\nexit 31\n"
	if err := os.WriteFile(filepath.Join(root, "nself"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("nself-l12-%d", time.Now().UnixNano())
	run := exec.Command("docker", "run", "--rm", "-d", "--name", name, "-p", "127.0.0.1::2222", "-e", "USER_NAME=test", "-e", "PUBLIC_KEY="+strings.TrimSpace(string(pub)), "-v", root+":/fixtures", "-v", filepath.Join(root, "nself")+":/usr/local/bin/nself:ro", "linuxserver/openssh-server:latest")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("start sshd: %s %v", out, err)
	}
	defer exec.Command("docker", "rm", "-f", name).Run()
	portOut, err := exec.Command("docker", "port", name, "2222/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	endpoint := strings.TrimSpace(string(portOut))
	_, port, ok := strings.Cut(endpoint, ":")
	if !ok {
		t.Fatalf("port: %s", endpoint)
	}
	rt := dbRemoteTarget{SSHTarget: "ssh://test@127.0.0.1:" + port, KeyPath: key, RemotePath: "/fixtures", EnvName: "staging", AllowVersionDrift: true}
	var got bytes.Buffer
	var last error
	for i := 0; i < 30; i++ {
		got.Reset()
		last = runRemoteNselfStream(context.Background(), rt, nil, &got, "db", "hasura", "metadata", "export", "--archive", "-")
		if last == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if last != nil {
		t.Fatalf("sshd export: %v", last)
	}
	if !bytes.Equal(got.Bytes(), fixture) {
		t.Fatal("remote export bytes differ")
	}
	desired := archiveFixture(t, "tables: [desired]\n")
	if err := runRemoteNselfStream(context.Background(), rt, bytes.NewReader(desired), io.Discard, "db", "hasura", "metadata", "apply", "--archive", "-"); err != nil {
		t.Fatal(err)
	}
	applied, err := os.ReadFile(filepath.Join(root, "applied.tar"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(applied, desired) {
		t.Fatal("remote apply bytes differ")
	}
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(project, "hasura", "metadata"), 0755); err != nil {
		t.Fatal(err)
	}
	localFile := filepath.Join(project, "hasura", "metadata", "tables.yaml")
	if err := os.WriteFile(localFile, []byte("tables: [local]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, argv := range [][]string{{"init", project}, {"-C", project, "config", "user.email", "test@example.invalid"}, {"-C", project, "config", "user.name", "Test"}, {"-C", project, "add", "hasura/metadata"}, {"-C", project, "commit", "-m", "local"}} {
		if out, err := exec.Command("git", argv...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", argv, out, err)
		}
	}
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldCwd)
	syncCmd := &cobra.Command{}
	syncCmd.SetContext(context.Background())
	syncCmd.Flags().String("message", "remote snapshot", "")
	if err := runDBHasuraRemoteSync(syncCmd, rt); err != nil {
		t.Fatal(err)
	}
	gotLocal, err := os.ReadFile(localFile)
	if err != nil || string(gotLocal) != "tables: [remote]\n" {
		t.Fatalf("sync local=%q %v", gotLocal, err)
	}
	if out, err := exec.Command("git", "-C", project, "status", "--porcelain", "hasura/metadata").Output(); err != nil || len(out) != 0 {
		t.Fatalf("sync did not commit: %q %v", out, err)
	}
	if err := os.WriteFile(localFile, []byte("tables: [desired]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, argv := range [][]string{{"-C", project, "add", "hasura/metadata"}, {"-C", project, "commit", "-m", "desired"}} {
		if out, err := exec.Command("git", argv...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", argv, out, err)
		}
	}
	applyCmd := &cobra.Command{}
	applyCmd.SetContext(context.Background())
	applyCmd.Flags().Bool("yes", true, "")
	var planText bytes.Buffer
	applyCmd.SetOut(&planText)
	if err := runDBHasuraRemoteApplyRef(applyCmd, rt, "HEAD"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(planText.String(), "hasura/metadata/tables.yaml") {
		t.Fatalf("missing Hasura plan: %s", planText.String())
	}
	wantArchive, err := database.ArchiveMetadataRef(context.Background(), project, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	applied, err = os.ReadFile(filepath.Join(root, "applied.tar"))
	if err != nil || !bytes.Equal(applied, wantArchive) {
		t.Fatalf("apply-ref stream mismatch: %v", err)
	}
}
