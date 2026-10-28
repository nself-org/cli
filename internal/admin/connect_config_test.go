package admin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// adminScanStub emulates OpenSSH configuration and its unauthenticated key
// capture. All commands stay inside the test sandbox.
func adminScanStub(e argvEnv, key *hostKey) string {
	script := `#!/bin/sh
case "$1" in
 -V) echo 'OpenSSH_9.6p1' >&2; exit 0;;
 -G)
  shift; port=22
  if [ "$1" = -p ]; then port="$2"; shift 2; fi
  shift; host="${1#*@}"
  case "$host" in
   alias.test) host=203.0.113.7;;
   jump.test) host=203.0.113.7; port=2200; printf 'proxyjump bastion.test\nhostkeyalias operator-alias\n';;
  esac
  printf 'hostname %s\nport %s\nuser u\n' "$host" "$port"
  exit 0;;
esac
for a in "$@"; do
 case "$a" in UserKnownHostsFile=*) file="${a#UserKnownHostsFile=}";; esac
done
if [ -n "$file" ]; then
`
	script += " : > '" + e.scanMark() + "'\n"
	script += " printf '%s\\n' \"$@\" > '" + filepath.Join(e.bin, "scan-argv") + "'\n"
	if key != nil {
		script += " read -r typ blob comment < '" + filepath.Join(e.bin, "hostkey.pub") + "'\n"
		script += " printf 'scan %s %s\\n' \"$typ\" \"$blob\" > \"$file\"\n"
	}
	return script + " exit 255\nfi\n"
}

func TestAdminSSHConfigAlias(t *testing.T) {
	compattest.Set(t, true)
	e := newArgvEnv(t, "", &hostKey{alias: "[203.0.113.7]:2222"})
	if err := VerifySSHKey(context.Background(), "u", "alias.test", 2222); err != nil {
		t.Fatal(err)
	}
	scan, err := os.ReadFile(filepath.Join(e.bin, "scan-argv"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(scan), "-p\n2222\n--\nu@alias.test\ntrue") {
		t.Fatalf("scan lost original SSH endpoint: %s", scan)
	}
	if strings.Contains(strings.Join(e.argv(t), " "), "203.0.113.7") {
		t.Fatal("connection must keep SSH alias/ProxyJump configuration")
	}
}

func TestAdminUnknownIPTrustRemedy(t *testing.T) {
	compattest.Set(t, true)
	e := newArgvEnv(t, "", &hostKey{alias: "203.0.113.7"})
	if err := os.Remove(filepath.Join(e.home, ".ssh", "known_hosts")); err != nil {
		t.Fatal(err)
	}
	err := VerifySSHKey(context.Background(), "u", "203.0.113.7", 22)
	if err == nil || !strings.Contains(err.Error(), "E487") || !strings.Contains(err.Error(), "203.0.113.7") || !strings.Contains(err.Error(), "known_hosts") || strings.Contains(err.Error(), "env target add") {
		t.Fatalf("unusable IP trust remedy: %v", err)
	}
}

func TestAdminSSHConfigPortAndHostKeyAlias(t *testing.T) {
	compattest.Set(t, true)
	e := newArgvEnv(t, "", &hostKey{alias: "operator-alias"})
	if err := VerifySSHKey(context.Background(), "u", "jump.test", 0); err != nil {
		t.Fatal(err)
	}
	args := strings.Join(e.argv(t), " ")
	if !strings.HasSuffix(args, "-- u@jump.test true") || strings.Contains(args, "-p ") {
		t.Fatalf("lost configured endpoint: %s", args)
	}
	scan, err := os.ReadFile(filepath.Join(e.bin, "scan-argv"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(scan), "ProxyJump=") || !strings.Contains(string(scan), "u@jump.test") {
		t.Fatalf("scan overrode jump configuration: %s", scan)
	}
}
