package commands

// Tests for P7-DEPL-29: the v1.4 `nself deploy <target>` push ships the merged
// .env snapshot, so its host key is strict (E487 on an unpinned key) and every
// ssh and rsync argv comes from the deploy transport builder.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// pushFixture is a project dir with a compose file and an env file, fake ssh
// and rsync that log their argv, fake keyscan tools, and a temp HOME.
func pushFixture(t *testing.T) (dir, log, bin, key string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses local shell fixtures")
	}
	compattest.Set(t, false)
	dir = t.TempDir()
	t.Chdir(dir)
	isolateDeployEnv(t)
	for n, b := range map[string]string{"docker-compose.yml": "services: {}\n", ".env": "FAKE_VALUE=not-a-secret\n"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bin = t.TempDir()
	log = filepath.Join(bin, "calls.log")
	for n, b := range map[string]string{
		"ssh":   "#!/bin/sh\necho \"ssh $*\" >> '" + log + "'\nexit 0\n",
		"rsync": "#!/bin/sh\necho \"rsync $*\" >> '" + log + "'\nexit 0\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, n), []byte(b), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	key = filepath.Join(bin, "id_test")
	if err := os.WriteFile(key, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_DEPLOY_SSH_KEY", key)
	t.Setenv("PATH", bin)
	return dir, log, bin, key
}

// TestRemoteDeployPushStrictV14: in v1.4 an unpinned host fails with E487
// before any ssh or rsync call; a pinned host is reached only with strict argv.
func TestRemoteDeployPushStrictV14(t *testing.T) {
	const host = "u@push.example.test"
	t.Run("unpinned host", func(t *testing.T) {
		dir, log, bin, _ := pushFixture(t)
		pinFakeHostKeys(t, bin) // fake scan tools and an empty pin file: no host is trusted
		err := remoteDeployPush(context.Background(), dir, host, "qa", true)
		if err == nil || !strings.Contains(err.Error(), "E487") {
			t.Fatalf("want E487, got %v", err)
		}
		if b, _ := os.ReadFile(log); len(b) != 0 {
			t.Fatalf("ssh or rsync ran before the host key check:\n%s", b)
		}
	})
	t.Run("pinned host", func(t *testing.T) {
		dir, log, bin, _ := pushFixture(t)
		pinFakeHostKeys(t, bin, host)
		if err := remoteDeployPush(context.Background(), dir, host, "qa", true); err != nil {
			t.Fatalf("push: %v", err)
		}
		b, _ := os.ReadFile(log)
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		if len(lines) < 5 {
			t.Fatalf("too few connections for the assertion to mean anything:\n%s", b)
		}
		for _, l := range lines {
			if strings.Contains(l, "accept-new") || !strings.Contains(l, "StrictHostKeyChecking=yes") {
				t.Errorf("connection is not strict: %s", l)
			}
		}
	})
}

// TestRemoteDeployPushUsesTransportBuilder: the rsync "-e" command of the push
// is the one deploy.Transport builds, and the push code builds no argv itself.
func TestRemoteDeployPushUsesTransportBuilder(t *testing.T) {
	const host = "u@push.example.test"
	srcPath, err := filepath.Abs("deploy_remote.go") // before the fixture changes directory
	if err != nil {
		t.Fatal(err)
	}
	dir, log, bin, key := pushFixture(t)
	pinFakeHostKeys(t, bin, host)
	if err := remoteDeployPush(context.Background(), dir, host, "qa", true); err != nil {
		t.Fatalf("push: %v", err)
	}
	spec, err := remote.ParseHostSpec(host)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := controlplane.HostKeyOptions(context.Background(), "qa", "qa-app", controlplane.DeriveTier("qa", "remote"), spec.String(), true)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := deploy.NewTransport(spec, key, policy)
	if err != nil {
		t.Fatal(err)
	}
	want, err := tr.RsyncArgv(nil, "/opt/nself/", filepath.Join(dir, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	var first string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "rsync ") {
			first = strings.TrimPrefix(l, "rsync ")
			break
		}
	}
	// want[:4] is "-az -e <ssh command> --": the part every rsync of the push shares.
	if prefix := strings.Join(want[:4], " ") + " "; !strings.HasPrefix(first, prefix) {
		t.Errorf("rsync argv does not start with the transport builder's:\n got %s\nwant %s...", first, prefix)
	}
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{`"-e"`, `"-o"`, "exec.Command", "StrictHostKeyChecking"} {
		if strings.Contains(string(src), banned) {
			t.Errorf("deploy_remote.go builds its own transport argv (%s): use internal/deploy", banned)
		}
	}
}
