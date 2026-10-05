package destinations

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// TestHostDestinationSSHD runs host:// against a throwaway
// linuxserver/openssh-server container on 127.0.0.1: put, list, get and a
// sha256 check through the real ssh and scp. It skips without Docker.
func TestHostDestinationSSHD(t *testing.T) {
	skipWindows(t)
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not reachable")
	}
	for _, tool := range []string{"ssh", "scp", "ssh-keygen", "ssh-keyscan"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
	tmp := t.TempDir()
	key := filepath.Join(tmp, "id")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v %s", err, out)
	}
	pub, _ := os.ReadFile(key + ".pub")
	out, err := exec.Command("docker", "run", "-d", "--rm", "-p", "127.0.0.1::2222",
		"-e", "PUBLIC_KEY="+strings.TrimSpace(string(pub)), "-e", "USER_NAME=bk",
		"-e", "PASSWORD_ACCESS=false", "linuxserver/openssh-server").CombinedOutput()
	if err != nil {
		t.Skipf("cannot start the sshd container: %v %s", err, out)
	}
	cid := strings.TrimSpace(string(out))
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", cid).Run() })
	var port string
	for i := 0; i < 30 && port == ""; i++ {
		if pm, err := exec.Command("docker", "port", cid, "2222/tcp").Output(); err == nil && len(pm) > 0 {
			line := strings.TrimSpace(strings.Split(string(pm), "\n")[0])
			port = line[strings.LastIndex(line, ":")+1:]
		} else {
			time.Sleep(time.Second)
		}
	}
	if port == "" {
		t.Fatal("container published no port")
	}

	var scan []byte
	for i := 0; i < 60; i++ {
		scan, _ = exec.Command("ssh-keyscan", "-p", port, "-t", "ed25519", "127.0.0.1").Output()
		if len(scan) > 0 {
			break
		}
		time.Sleep(time.Second)
	}
	f := strings.Fields(string(scan))
	if len(f) < 3 {
		t.Fatalf("no host key from the container: %q", scan)
	}
	kh := filepath.Join(tmp, "kh")
	if err := (remote.PinnedHostKeys{Path: kh}).Add("nself-ci-sshd", f[len(f)-2]+" "+f[len(f)-1]); err != nil {
		t.Fatal(err)
	}
	t.Setenv(KnownHostsEnv, kh)

	d := &hostDest{server: "sshd", dest: "bk@127.0.0.1", dir: "/config/backups",
		extra: []string{"-i", key, "-p", port}}
	src := writeFile(t, tmp, "a.dump", "sshd round trip")
	if err := d.Put(t.Context(), src, "p/a.dump"); err != nil {
		t.Fatalf("put: %v", err)
	}
	objs, err := d.List(t.Context(), "p/")
	if err != nil || len(objs) != 1 || objs[0].Key != "p/a.dump" || objs[0].Size != 15 {
		t.Fatalf("list: %+v %v", objs, err)
	}
	back := filepath.Join(tmp, "back")
	if err := d.Get(t.Context(), "p/a.dump", back); err != nil {
		t.Fatalf("get: %v", err)
	}
	if b, _ := os.ReadFile(back); string(b) != "sshd round trip" {
		t.Fatalf("downloaded %q", b)
	}
	if err := d.Get(t.Context(), "p/missing", filepath.Join(tmp, "m")); err == nil {
		t.Fatal("get of a missing object succeeded")
	}
}
