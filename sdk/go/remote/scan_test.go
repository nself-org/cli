package remote

import (
	"context"
	"strings"
	"testing"
)

func TestScanHostKeys_ArgvAndParse(t *testing.T) {
	log := stubTools(t, map[string]string{"ssh-keyscan": "echo '# 203.0.113.7:22 SSH-2.0-OpenSSH'; echo '203.0.113.7 " + keyA + "'; echo '203.0.113.7 " + keyR + "'"})
	keys, err := ScanHostKeys(context.Background(), "203.0.113.7", 2222)
	if err != nil || len(keys) != 2 {
		t.Fatalf("keys = %v, %v", keys, err)
	}
	if keys[0].Type != "ssh-ed25519" || !strings.HasPrefix(keys[0].Fingerprint, "SHA256:") {
		t.Fatalf("key0 = %+v", keys[0])
	}
	want := []string{"-T", "5", "-p", "2222", "--", "203.0.113.7"}
	if got := readCalls(t, log)[0].args; strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("argv = %q", got)
	}
}

func TestScanHostKeys_RefusesBadHostBeforeExec(t *testing.T) {
	n := countExecs(t)
	for _, h := range []string{"-oProxyCommand=x", "a b", "a;b", ""} {
		if _, err := ScanHostKeys(context.Background(), h, 22); err == nil {
			t.Errorf("host %q accepted", h)
		}
	}
	for _, p := range []int{0, -1, 70000} {
		if _, err := ScanHostKeys(context.Background(), "h", p); err == nil {
			t.Errorf("port %d accepted", p)
		}
	}
	if *n != 0 {
		t.Fatalf("%d execs", *n)
	}
}

// The stub follows ssh's real behaviour: it writes a known_hosts line to the
// UserKnownHostsFile it was given, then exits 255 (authentication fails).
const sshScanStub = `
case "$1" in -V) echo "OpenSSH_9.6p1, stub" >&2; exit 0;; esac
for a in "$@"; do case "$a" in UserKnownHostsFile=*) f="${a#UserKnownHostsFile=}";; HostKeyAlias=*) al="${a#HostKeyAlias=}";; esac; done
echo "$al ` + keyA + `" > "$f"
exit 255`

func TestScanHostKeysSSH_StubWritesKnownHostsExits255(t *testing.T) {
	log := stubTools(t, map[string]string{"ssh": sshScanStub})
	keys, err := ScanHostKeysSSH(context.Background(), Target{Dest: "deploy@203.0.113.7"}, "n1")
	if err != nil || len(keys) != 1 || keys[0].Line != "nself-ci-n1 "+keyA {
		t.Fatalf("keys = %+v, %v", keys, err)
	}
	var scan call
	for _, c := range readCalls(t, log) {
		if len(c.args) > 0 && c.args[0] != "-V" {
			scan = c
		}
	}
	for _, want := range []string{
		"PreferredAuthentications=none", "StrictHostKeyChecking=accept-new", "PubkeyAuthentication=no",
		"PasswordAuthentication=no", "KbdInteractiveAuthentication=no", "HostKeyAlias=nself-ci-n1",
		"KnownHostsCommand=none", "-T", "-a", "-x",
	} {
		if !hasArg(scan.args, want) {
			t.Errorf("argv lacks %s: %q", want, scan.args)
		}
	}
	if hasArg(scan.args, "StrictHostKeyChecking=yes") {
		t.Error("argv still holds StrictHostKeyChecking=yes")
	}
	for _, a := range scan.args {
		if a == "-J" || strings.HasPrefix(a, "ProxyJump=") || strings.HasPrefix(a, "ProxyCommand=") {
			t.Errorf("scan overrides the operator's jump host: %s", a)
		}
	}
	n := len(scan.args)
	if strings.Join(scan.args[n-3:], " ") != "-- deploy@203.0.113.7 true" {
		t.Errorf("tail = %q, want only the command `true`", scan.args[n-3:])
	}
}

func TestScanHostKeysSSH_NoKeyIsAnError(t *testing.T) {
	stubTools(t, map[string]string{"ssh": "echo 'Connection refused' >&2; exit 255"})
	if _, err := ScanHostKeysSSH(context.Background(), Target{Dest: "h1"}, "n1"); err == nil {
		t.Fatal("no key captured, no error")
	}
}

func TestScanHostKeysSSH_RefusesBeforeExec(t *testing.T) {
	n := countExecs(t)
	if _, err := ScanHostKeysSSH(context.Background(), Target{Dest: "-oProxyCommand=x"}, "n1"); err == nil {
		t.Error("bad dest accepted")
	}
	if _, err := ScanHostKeysSSH(context.Background(), Target{Dest: "h1"}, "n 1"); err == nil {
		t.Error("bad node id accepted")
	}
	if *n != 0 {
		t.Fatalf("%d execs", *n)
	}
}

// An ssh whose -V output cannot be parsed still gets KnownHostsCommand=none:
// fail closed, never silently omit the option.
func TestScanHostKeysSSH_UnknownVersionKeepsKnownHostsCommandNone(t *testing.T) {
	body := strings.Replace(sshScanStub, `echo "OpenSSH_9.6p1, stub" >&2`, `echo "mystery ssh" >&2`, 1)
	log := stubTools(t, map[string]string{"ssh": body})
	if _, err := ScanHostKeysSSH(context.Background(), Target{Dest: "h1"}, "n1"); err != nil {
		t.Fatal(err)
	}
	var scan call
	for _, c := range readCalls(t, log) {
		if len(c.args) > 0 && c.args[0] != "-V" {
			scan = c
		}
	}
	if !hasArg(scan.args, "KnownHostsCommand=none") {
		t.Fatalf("argv lacks KnownHostsCommand=none: %q", scan.args)
	}
}
