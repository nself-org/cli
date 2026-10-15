package deploy

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWindowsDeployKeyPaths(t *testing.T) {
	for _, key := range []string{`C:/Users/Jane/.ssh/id_ed25519`, `C:\Users\Jane\.ssh\id_ed25519`} {
		tr, err := newSSHTransport(mustSpec(t, "host.test"), key, nil)
		if err != nil {
			t.Errorf("key %q: %v", key, err)
			continue
		}
		want := strings.ReplaceAll(key, `\`, "/")
		if tr.opts[1] != want {
			t.Errorf("key = %q, want %q", tr.opts[1], want)
		}
		if got := sshOpts(key, nil)[1]; got != want {
			t.Errorf("inventory key = %q, want %q", got, want)
		}
	}
	for _, key := range []string{`C:relative`, `C:/a b/key`, `C:/key -oProxyCommand=x`, `C:/key;id`, `C:\key"`, `host:key`, `-F/tmp/key`} {
		if err := checkDeployKeyPath(key); err == nil {
			t.Errorf("accepted unsafe key %q", key)
		}
	}
}

func TestWindowsDeployCopyPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX cygpath stub")
	}
	dir := t.TempDir()
	// Echo a mount path deliberately different from /c and /cygdrive/c: use
	// the installed runtime's mapping rather than assuming its mount prefix.
	script := "#!/bin/sh\n[ \"$1\" = -u ] || exit 2\n[ \"$2\" = -- ] || exit 2\nprintf '/mounted/c/%s\\n' \"${3#C:?}\" | tr '\\\\' '/'\n"
	if err := os.WriteFile(filepath.Join(dir, "cygpath"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	tr, err := newSSHTransport(mustSpec(t, "host.test"), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{`C:/Jane Doe/env`, `C:\Jane Doe\env`} {
		args, err := tr.rsyncArgv(src, "/remote")
		if err != nil {
			t.Fatal(err)
		}
		if got := args[len(args)-2]; got != "/mounted/c/Jane Doe/env" {
			t.Errorf("rsync source = %q", got)
		}
		args, err = tr.scpArgv(src, "/remote")
		if err != nil {
			t.Fatal(err)
		}
		if got := args[len(args)-2]; got != src {
			t.Errorf("native scp source = %q, want %q", got, src)
		}
	}
	args, err := rsyncArgv(tr.spec, tr.opts, nil, `C:\Jane Doe\list`, `C:\Jane Doe\project\`, "/remote")
	if err != nil {
		t.Fatal(err)
	}
	if got := args[len(args)-2]; got != "/mounted/c/Jane Doe/project/" {
		t.Errorf("inventory source = %q", got)
	}
	if !strings.Contains(strings.Join(args, "\n"), "--files-from=/mounted/c/Jane Doe/list\n") {
		t.Errorf("inventory list = %q", args)
	}
}
