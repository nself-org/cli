package repoqa

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestReleaseCreateDenylist exercises tag-time and post-build checks with
// isolated artifacts and mocked git/gh commands; it never publishes a release.
func TestReleaseCreateDenylist(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release wrapper fixtures require a POSIX shell environment")
	}
	requireTool(t, "bash")
	for _, tc := range []struct {
		name, artifacts, license, blocked string
		dist                              bool
	}{
		{name: "tag time", license: "success"},
		{name: "empty dist", dist: true, license: "success", blocked: "DENY-001"},
		{name: "signature only", dist: true, artifacts: "app.cosign.bundle", license: "success", blocked: "DENY-002"},
		{name: "sboms only", dist: true, artifacts: "sbom.spdx.json sbom.cdx.json", license: "success", blocked: "DENY-001"},
		{name: "complete dist", dist: true, artifacts: "app.cosign.bundle sbom.spdx.json sbom.cdx.json", license: "success"},
		{name: "failed main audit", license: "failure", blocked: "DENY-004"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := releaseCreateFixture(t)
			if tc.dist {
				if err := os.Mkdir(filepath.Join(root, "dist"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range strings.Fields(tc.artifacts) {
				writeReleaseFixture(t, filepath.Join(root, "dist", name), "", 0600)
			}
			cmd := exec.Command("bash", filepath.Join(root, "scripts/release-create.sh"), "v9.9.9", "--generate-notes")
			cmd.Env = append(os.Environ(), "PATH="+filepath.Join(root, "bin")+":"+os.Getenv("PATH"), "AUDIT_RESULT="+tc.license)
			out, err := cmd.CombinedOutput()
			if tc.blocked != "" {
				if err == nil || !strings.Contains(string(out), tc.blocked) || strings.Contains(string(out), "MOCK_RELEASE") {
					t.Fatalf("expected block %s: err=%v\n%s", tc.blocked, err, out)
				}
			} else if err != nil || !strings.Contains(string(out), "MOCK_RELEASE v9.9.9 --generate-notes") {
				t.Fatalf("expected release: err=%v\n%s", err, out)
			}
			if !tc.dist && (!strings.Contains(string(out), "DENY-001") || !strings.Contains(string(out), "DENY-002") || !strings.Contains(string(out), "pre-release-gate.yml")) {
				t.Fatalf("missing post-build deferral note:\n%s", out)
			}
		})
	}
}

// releaseCreateFixture copies only the wrapper and supplies deterministic tools.
func releaseCreateFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	script, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts/release-create.sh"))
	if err != nil {
		t.Fatal(err)
	}
	writeReleaseFixture(t, filepath.Join(root, "scripts/release-create.sh"), string(script), 0600)
	writeReleaseFixture(t, filepath.Join(root, ".github/release-denylist.json"), "{}", 0600)
	writeReleaseFixture(t, filepath.Join(root, "bin/git"), "#!/bin/sh\nexit 0\n", 0700)
	writeReleaseFixture(t, filepath.Join(root, "bin/gh"), `#!/bin/sh
case "$1 $2" in
  'run list')
    case " $* " in
      *' --branch main '*) printf '%s\n' "$AUDIT_RESULT" ;;
      *) printf 'failure\n' ;;
    esac ;;
  'api '*) printf 'closed\n' ;;
  'release create') shift 2; printf 'MOCK_RELEASE %s\n' "$*" ;;
  *) exit 2 ;;
esac
`, 0700)
	return root
}

func writeReleaseFixture(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
