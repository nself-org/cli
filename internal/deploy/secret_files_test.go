package deploy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileSecretsStageSyncAndRollback(t *testing.T) {
	needTools(t)
	rp := t.TempDir()
	m, compose, envs, snap := buildScratchFor(t, rp)
	rel := secretDir + "/POSTGRES_PASSWORD"
	// A password can contain a local path. Rewriting/checking it as config
	// would silently corrupt the credential or leak it in a path error.
	value := "literal='" + m.ScratchRoot + "'\n\n"
	writeFile(t, filepath.Join(m.ScratchRoot, rel), value, 0444)
	set, err := Stage(stageIn(m, compose, envs, snap, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(set.Files, rel) {
		t.Fatal("secret not staged")
	}
	if got := readFile(t, filepath.Join(set.StageDir, rel)); got != value {
		t.Fatal("staging changed raw credential bytes")
	}
	assertSecretModes(t, set.StageDir, rel)
	f := localRsyncRemote{newFakeRemote(t)}
	writeFile(t, filepath.Join(rp, rel), "old-value", 0444)
	ctx := context.Background()
	bk, err := CreatePreDeployTar(ctx, f, rp, NewBackupID(time.Now()), set.Files)
	if err != nil {
		t.Fatal(err)
	}
	if err := SyncArtifacts(ctx, f, set); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(rp, rel)); got != value {
		t.Fatal("sync changed raw credential bytes")
	}
	assertSecretModes(t, rp, rel)
	if err := RestorePreDeployTar(ctx, f, rp, bk.Tar, RestoreSpec{Changed: set.Files, Absent: bk.Absent}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(rp, rel)); got != "old-value" {
		t.Fatal("rollback failed to restore credential")
	}
	assertSecretModes(t, rp, rel)
}

func TestSSHFileSecretsRequireAndShipCanonicalEnv(t *testing.T) {
	read := installFakeSSHTools(t)
	root := t.TempDir()
	file := filepath.Join(root, secretDir, "POSTGRES_PASSWORD")
	writeFile(t, file, "fake-credential", 0444)
	tr, err := NewTransport(mustSpec(t, "deploy@203.0.113.7"), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	compose := filepath.Join(root, "docker-compose.yml")
	if _, err := tr.CopyFileSecrets(context.Background(), compose, "/opt/app"); err == nil {
		t.Fatal("missing canonical env must fail before sending")
	}
	if len(read()) != 0 {
		t.Fatal("missing env caused an outbound operation")
	}
	env := filepath.Join(root, ".nself/compose.env")
	writeFile(t, env, "NSELF_SECRET_VERSION_POSTGRES=digest\n", 0600)
	shipped, err := tr.CopyFileSecrets(context.Background(), compose, "/opt/app")
	if err != nil || !shipped {
		t.Fatalf("file secret transfer failed: %v", err)
	}
	seen := false
	for _, call := range read() {
		if call.tool == "rsync" && strings.Contains(strings.Join(call.args, " "), env) {
			seen = strings.Contains(strings.Join(call.args, " "), "-az") && call.args[len(call.args)-1] == "deploy@203.0.113.7:/opt/app/.nself/compose.env"
		}
	}
	if !seen {
		t.Fatal("canonical env was not copied with file modes preserved")
	}
	if err := os.Chmod(env, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := sshSecretFiles(compose); err == nil {
		t.Fatal("loose canonical env mode must be refused")
	}
}

func TestFileSecretComposeKeepsUserEnvBeforeGeneratedEnv(t *testing.T) {
	root := t.TempDir()
	for _, withUserEnv := range []bool{false, true} {
		if withUserEnv {
			writeFile(t, filepath.Join(root, ".env"), "CUSTOM_VALUE=kept\n", 0600)
		}
		for _, snapshot := range []string{"", filepath.Join(root, ".env.prod")} {
			command := fileSecretComposeCommand("printf '%s\\n'", root, snapshot)
			out, err := exec.Command("sh", "-c", command).Output()
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Fields(string(out))
			want := []string{"--env-file", filepath.Join(root, ".nself/compose.env")}
			if snapshot != "" {
				want = append([]string{"--env-file", snapshot}, want...)
			} else if withUserEnv {
				want = append([]string{"--env-file", filepath.Join(root, ".env")}, want...)
			}
			if strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatalf("user env precedence changed: %v, want %v", got, want)
			}
		}
	}
}

func assertSecretModes(t *testing.T, root, rel string) {
	t.Helper()
	for p, mode := range map[string]os.FileMode{secretDir: 0700, rel: 0444} {
		info, err := os.Stat(filepath.Join(root, p))
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("incorrect secret mode for %s", p)
		}
	}
}

func TestFileSecretsRefuseSymlinkDirectories(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".nself"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, secretDir)); err != nil {
		t.Skip(err)
	}
	if _, err := stageSecretFiles(root, t.TempDir()); err == nil {
		t.Fatal("staging followed symlink")
	}
	if _, err := sshSecretFiles(filepath.Join(root, "docker-compose.yml")); err == nil {
		t.Fatal("SSH copy followed symlink")
	}
	if err := prepareSecretDir(context.Background(), newFakeRemote(t), root, []string{secretDir + "/POSTGRES_PASSWORD"}); err == nil {
		t.Fatal("remote protection followed symlink")
	}
}

func TestFileSecretsBackupRefusesNestedExternalSymlink(t *testing.T) {
	needTools(t)
	root, outside := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(outside, "private"), "not-for-backup", 0600)
	if err := os.MkdirAll(filepath.Join(root, secretDir), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "private"), filepath.Join(root, secretDir, "OTHER")); err != nil {
		t.Skip(err)
	}
	if _, err := ResolveDirs(context.Background(), newFakeRemote(t), root, layoutDirs([]string{"docker-compose.yml"})); err == nil {
		t.Fatal("backup layout guard followed external secret symlink")
	}
}
