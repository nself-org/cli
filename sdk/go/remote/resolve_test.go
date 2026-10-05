package remote

import (
	"context"
	"strings"
	"testing"
)

const sshGStub = `
cat <<'EOF'
host web1
user deploy
hostname 203.0.113.7
port 2222
proxyjump bastion
hostkeyalias web1-alias
identityfile ~/.ssh/id_ed25519
sendenv LANG
unknownkey whatever
EOF`

func TestResolveSSHHost_ParsesFiveKeys(t *testing.T) {
	log := stubTools(t, map[string]string{"ssh": sshGStub})
	r, err := ResolveSSHHost(context.Background(), "web1")
	if err != nil {
		t.Fatal(err)
	}
	want := Resolved{Hostname: "203.0.113.7", Port: 2222, User: "deploy", ProxyJump: "bastion", HostKeyAlias: "web1-alias"}
	if r != want {
		t.Fatalf("Resolved = %+v, want %+v", r, want)
	}
	if got := strings.Join(readCalls(t, log)[0].args, " "); got != "-G -- web1" {
		t.Fatalf("argv = %q", got)
	}
}

func TestResolveSSHHost_RefusesBeforeRunningSSH(t *testing.T) {
	log := stubTools(t, map[string]string{"ssh": sshGStub})
	n := countExecs(t)
	for _, a := range []string{"-oProxyCommand=x", "a b", "a;b", "", "$(id)", "a\nb"} {
		if _, err := ResolveSSHHost(context.Background(), a); err == nil {
			t.Errorf("alias %q accepted", a)
		}
	}
	if *n != 0 || len(readCalls(t, log)) != 0 {
		t.Fatalf("ssh ran for a refused alias (hook=%d)", *n)
	}
}

func TestResolveSSHHost_Failures(t *testing.T) {
	stubTools(t, map[string]string{"ssh": "echo 'port 99999'; echo 'hostname h'"})
	if _, err := ResolveSSHHost(context.Background(), "h"); err == nil {
		t.Error("invalid port accepted")
	}
	stubTools(t, map[string]string{"ssh": "echo 'user u'"})
	if _, err := ResolveSSHHost(context.Background(), "h"); err == nil {
		t.Error("missing hostname accepted")
	}
	stubTools(t, map[string]string{"ssh": "exit 255"})
	if _, err := ResolveSSHHost(context.Background(), "h"); err == nil {
		t.Error("ssh failure ignored")
	}
}

// FuzzResolveAlias: no accepted alias may hold whitespace, a shell
// metacharacter or a leading '-'.
func FuzzResolveAlias(f *testing.F) {
	for _, s := range []string{"web1", "u@h", "-oProxyCommand=x", "a b", "a;b", "", "2001:db8::1", "a\nb", "$(id)", "`x`", "h%h", "a\x00b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if ValidateAlias(s) != nil {
			return
		}
		if why := unsafeAccepted(s); why != "" {
			t.Fatalf("accepted %q: %s", s, why)
		}
	})
}
