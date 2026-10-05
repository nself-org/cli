package destinations

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

const fakeSSH = `#!/bin/sh
# fake ssh: log argv, run the remote command locally
d=$(dirname "$0")
if [ "$1" = "-V" ]; then echo "OpenSSH_9.6p1" >&2; exit 0; fi
echo "ssh $*" >> "$d/log"
while [ "$1" != "--" ]; do shift; done
shift; shift
exec sh -c "$1"
`

const fakeSCP = `#!/bin/sh
# fake scp: log argv, copy the local file to the path after the first colon
d=$(dirname "$0")
echo "scp $*" >> "$d/log"
while [ "$1" != "--" ]; do shift; done
shift
cp "$1" "${2#*:}"
`

// stat -c is GNU; macOS needs -f.
const fakeStat = `#!/bin/sh
if [ "$1" = "-c" ]; then shift; shift; exec /usr/bin/stat -f '%z %m %N' "$@"; fi
exec /usr/bin/stat "$@"
`

const fakeSum = "#!/bin/sh\nshift\nexec shasum -a 256 \"$@\"\n"

// fakeBin puts fake ssh/scp (and macOS shims) first on PATH and returns the
// file the fakes log their argv to.
func fakeBin(t *testing.T) string {
	t.Helper()
	skipWindows(t)
	dir := t.TempDir()
	files := map[string]string{"ssh": fakeSSH, "scp": fakeSCP}
	if runtime.GOOS == "darwin" {
		files["stat"], files["sha256sum"] = fakeStat, fakeSum
	}
	for n, body := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return filepath.Join(dir, "log")
}

// hostFixture returns an inventory with server "bk1" and a known_hosts file
// pinning it.
func hostFixture(t *testing.T) *Inventory {
	t.Helper()
	kh := filepath.Join(t.TempDir(), "kh")
	pin := remote.PinnedHostKeys{Path: kh}
	if err := pin.Add("nself-ci-bk1", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAAA"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(KnownHostsEnv, kh)
	return &Inventory{SchemaVersion: 1, Project: "p", Environments: map[string]controlplane.Environment{
		"prod": {Name: "prod", Kind: "remote", Servers: []controlplane.Server{
			{Name: "bk1", Role: controlplane.RoleApp, Host: "bk@fake.invalid"},
			{Name: "lo", Role: controlplane.RoleApp},
		}},
	}}
}

func readLog(t *testing.T, p string) string {
	t.Helper()
	b, _ := os.ReadFile(p)
	return strings.TrimSpace(string(b))
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// skipWindows skips tests that need /bin/sh fakes, symlinks or unix modes.
func skipWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell and unix file modes")
	}
}
