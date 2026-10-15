package access

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

func TestSSHTransportDefaultPortPin(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	store := remote.PinnedHostKeys{Path: filepath.Join(home, ".config", "nself", "deploy_known_hosts")}
	spec := remote.HostSpec{Host: "example.test", Port: 22}
	if err := store.Add(remote.PinAlias(spec), "ssh-ed25519 AQID"); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"u@example.test", "u@example.test:22"} {
		args, err := (&SSHTransport{Host: host}).sshArgs()
		if err != nil || !strings.Contains(strings.Join(args, " "), "HostKeyAlias="+remote.PinAlias(spec)) {
			t.Fatalf("%s: %q, %v", host, args, err)
		}
	}
	if err := store.Add("nself-a.test-2222", "ssh-ed25519 AQID"); err != nil {
		t.Fatal(err)
	}
	args, err := (&SSHTransport{Host: "nself-a.test-2222"}).sshArgs()
	if err != nil || !strings.Contains(strings.Join(args, " "), `GlobalKnownHostsFile="/dev/null"`) {
		t.Fatalf("ambiguous bare host can still match legacy pins: %q, %v", args, err)
	}
}
