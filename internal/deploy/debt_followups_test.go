package deploy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDebtDeployStateRequiresSchema(t *testing.T) {
	for _, raw := range []string{"", "null", "{}", `{"strategy":"rolling"}`, `{"schema_version":null}`} {
		t.Run(raw, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, DeployStateFile)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadDeployState(dir); err == nil {
				t.Fatal("existing state without schema accepted")
			}
		})
	}
}

// imageShellRemote executes the production capture script against a shell
// function, so Docker failure handling is exercised inside the remote shell.
type imageShellRemote struct {
	Remote
	docker string
}

func (r imageShellRemote) Run(ctx context.Context, command string) ([]byte, error) {
	return exec.CommandContext(ctx, "sh", "-c", "docker() { "+r.docker+"; }; "+unshell(command)).CombinedOutput()
}

func TestDebtImageSnapshotFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX remote shell")
	}
	for _, tc := range []struct {
		name, docker string
		wantErr      bool
	}{
		{"daemon unavailable", `printf '%s\n' 'Cannot connect to Docker daemon' >&2; return 1`, true},
		{"permission denied", `printf '%s\n' 'permission denied while connecting to Docker socket' >&2; return 1`, true},
		{"transport failure", `printf '%s\n' 'error during connect: connection reset by peer' >&2; return 1`, true},
		{"missing image", `printf '%s\n' "Error response from daemon: No such image: app:latest" >&2; return 1`, false},
		{"malformed id", `printf '%s\n' broken`, true},
		{"existing image", `printf '%s\n' sha256:` + strings.Repeat("a", 64), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := imageShellRemote{Remote: newFakeRemote(t), docker: tc.docker}
			saved, err := saveImageTags(context.Background(), r, []string{"app:latest"})
			if (err != nil) != tc.wantErr {
				t.Fatalf("saved=%v, err=%v, wantErr=%v", saved, err, tc.wantErr)
			}
			if tc.name == "existing image" && len(saved) != 1 {
				t.Fatalf("saved=%v", saved)
			}
		})
	}
}
