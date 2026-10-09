package admin

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAdminSSHKnownHostAndEntryPoints(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX SSH recorder")
	}
	t.Setenv("NSELF_V15", "1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := t.TempDir()
	key := filepath.Join(bin, "hostkey")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("keygen: %v %s", err, out)
	}
	pub, err := os.ReadFile(key + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(pub))
	line := "[host.test]:2222 " + fields[0] + " " + fields[1] + "\n"
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "known_hosts"), []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
	scan := "#!/bin/sh\nprintf '%s' '" + line + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh-keyscan"), []byte(scan), 0700); err != nil {
		t.Fatal(err)
	}
	ssh := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + filepath.Join(bin, "argv") + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(ssh), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	if err := VerifySSHKey(context.Background(), "u", "host.test", 2222); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(filepath.Join(bin, "argv"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "StrictHostKeyChecking=yes") || strings.Contains(string(args), "HostKeyAlias=") {
		t.Fatalf("trusted port argv: %s", args)
	}
	if err := EnsureRemoteAdmin(context.Background(), "u", "host.test", 2222); err != nil {
		t.Fatal(err)
	}
	cmd, err := OpenTunnel(context.Background(), ConnectOpts{User: "u", Host: "host.test", SSHPort: 2222, LocalPort: 18080, RemotePort: 8080})
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(home, ".ssh", "known_hosts")); err != nil {
		t.Fatal(err)
	}
	if err := VerifySSHKey(context.Background(), "u", "host.test", 2222); err == nil || !strings.Contains(err.Error(), "E487") || !strings.Contains(err.Error(), "--trust-host-key") {
		t.Fatalf("unknown key hint: %v", err)
	}
}
