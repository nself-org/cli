package deploy

// A recording Remote for tests. Shell commands that touch only the file system
// run for real with sh against a temp dir that stands in for the host; commands
// that need docker or nself are answered by the handler. Every command and every
// rsync is recorded in order.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

type fakeRemote struct {
	t       *testing.T
	home    string // $HOME of the fake host
	mu      sync.Mutex
	log     []string
	flags   [][]string // flags of each rsync call
	fail    map[string]error
	handler func(script string) (out string, ok bool)
}

func newFakeRemote(t *testing.T) *fakeRemote {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake host runs remote scripts with sh against a local temp dir; remote paths are POSIX")
	}
	return &fakeRemote{t: t, home: t.TempDir(), fail: map[string]error{}}
}

func (f *fakeRemote) Label() string { return "fake-host" }

// unshell returns the script inside "sh -c '<script>'".
func unshell(cmd string) string {
	if s, ok := strings.CutPrefix(cmd, "sh -c "); ok {
		s = strings.TrimSuffix(strings.TrimPrefix(s, "'"), "'")
		return strings.ReplaceAll(s, `'\''`, `'`)
	}
	return cmd
}

func (f *fakeRemote) Run(ctx context.Context, cmd string) ([]byte, error) {
	script := unshell(cmd)
	f.mu.Lock()
	f.log = append(f.log, script)
	f.mu.Unlock()
	for sub, err := range f.fail {
		if sub == "<rsync>" {
			continue
		}
		if strings.Contains(script, sub) {
			return nil, err
		}
	}
	if f.handler != nil {
		if out, ok := f.handler(script); ok {
			return []byte(out), nil
		}
	}
	if realTool.MatchString(script) {
		f.t.Fatalf("unhandled docker/nself command would run the real tool: %q", script)
	}
	c := exec.CommandContext(ctx, "sh", "-c", script)
	c.Env = append(os.Environ(), "HOME="+f.home)
	var se bytes.Buffer
	c.Stderr = &se
	out, err := c.Output()
	if err != nil {
		return out, fmt.Errorf("%w: %s", err, strings.TrimSpace(se.String()))
	}
	return out, nil
}

func (f *fakeRemote) Rsync(ctx context.Context, flags []string, filesFrom, srcDir, destDir string) error {
	list, err := os.ReadFile(filesFrom)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.log = append(f.log, "rsync "+strings.Join(flags, " ")+" -> "+destDir)
	f.flags = append(f.flags, flags)
	f.mu.Unlock()
	if ferr, ok := f.fail["<rsync>"]; ok {
		return ferr
	}
	if _, err := rsyncArgv(sshSpecForTest(), nil, flags, filesFrom, srcDir, destDir); err != nil {
		return err
	}
	for _, rel := range strings.Fields(string(list)) {
		data, err := os.ReadFile(filepath.Join(srcDir, rel))
		if err != nil {
			return err
		}
		dst := filepath.Join(destDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		info, err := os.Stat(filepath.Join(srcDir, rel))
		if err != nil {
			return err
		}
		mode := info.Mode().Perm()
		if len(flags) > 0 && flags[0] == "--chmod=F600" {
			mode = 0o600
		}
		if err := os.WriteFile(dst, data, mode); err != nil {
			return err
		}
		if err := os.Chmod(dst, mode); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeRemote) commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.log...)
}

// fakeClock never sleeps: Sleep advances Now.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	c.now = c.now.Add(d)
	return ctx.Err()
}

// realTool matches an invocation of docker or nself in a script.
var realTool = regexp.MustCompile(`(^|[;&|(\s])(nself|docker)\s`)

var errBoom = errors.New("boom")

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool { _, err := os.Lstat(path); return err == nil }

func sshSpecForTest() remote.HostSpec { return remote.HostSpec{Host: "fake.example"} }
