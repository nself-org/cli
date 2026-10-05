package remote

import (
	"strings"
	"testing"
)

var hostilePaths = []string{
	"-oProxyCommand=touch /tmp/pwned",
	"-oProxyCommand=x",
	"-rf",
	"-",
	"; rm -rf /",
	"/opt/x; id",
	"/opt/x$(id)",
	"/opt/x`id`",
	"/opt/x | id",
	"/opt/x && id",
	"/opt/x\nid",
	"/opt/x\r\nid",
	"/opt/x\x00id",
	"/opt/x id",
	"/opt/x\tid",
	"/opt/'x'",
	"/opt/\"x\"",
	"/opt/x>out",
	"/opt/x<in",
	"/opt/x*",
	"/opt/x?",
	"/opt/x\\y",
	"~/x",
	"..",
	"../etc",
	"/opt/../etc",
	"/opt/x/..",
	"a/../../b",
}

func TestValidateRemotePath_RefusesHostile(t *testing.T) {
	for _, p := range hostilePaths {
		if err := ValidateRemotePath(p); err == nil {
			t.Errorf("ValidateRemotePath(%q) = nil, want error", p)
		}
	}
}

func TestValidateRemotePath_AcceptsLegitimate(t *testing.T) {
	for _, p := range []string{"", "/opt/nself", "/opt/nself-staging", "/home/ubuntu/app_v2", "opt/nself", "/opt/nself.d", "/a/.hidden/b", "/opt/a..b", "x-y"} {
		if err := ValidateRemotePath(p); err != nil {
			t.Errorf("ValidateRemotePath(%q) = %v, want nil", p, err)
		}
	}
}

func TestValidateRemotePath_ErrorTextKept(t *testing.T) {
	err := ValidateRemotePath("/opt/x; id")
	want := `remote path contains unsafe characters (got "/opt/x; id"): only [a-zA-Z0-9/_.-] allowed`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestValidateAlias(t *testing.T) {
	for _, a := range []string{"-oProxyCommand=x", "a b", "a;b", "", "-x", "a\nb", "a$b", "a`b", "a'b", "a|b", "a/b", "a*b", "a\x00b", "a%h", "[::1]", strings.Repeat("a", maxHostToken+1)} {
		if err := ValidateAlias(a); err == nil {
			t.Errorf("ValidateAlias(%q) = nil, want error", a)
		}
	}
	for _, a := range []string{"web1", "deploy@203.0.113.7", "host.example.org", "2001:db8::1", "a_b-c", "user+tag@h"} {
		if err := ValidateAlias(a); err != nil {
			t.Errorf("ValidateAlias(%q) = %v", a, err)
		}
	}
}

func TestValidateNodeID(t *testing.T) {
	for _, id := range []string{"", "-a", "a b", "a;b", "a/b", "a\n"} {
		if ValidateNodeID(id) == nil {
			t.Errorf("ValidateNodeID(%q) = nil", id)
		}
	}
	if err := ValidateNodeID("01HZX.node_1-a"); err != nil {
		t.Error(err)
	}
}

// shellMeta is every character the fuzz tests forbid in an accepted string.
const shellMeta = ";&|$`<>(){}[]*?!\"'\\#~%^=,\x00"

func unsafeAccepted(s string) string {
	if s != "" && s[0] == '-' {
		return "leading '-'"
	}
	if strings.ContainsAny(s, shellMeta) {
		return "shell metacharacter"
	}
	for i := 0; i < len(s); i++ {
		if s[i] <= ' ' || s[i] == 0x7f {
			return "whitespace or control character"
		}
	}
	return ""
}

// FuzzValidateRemotePath: no accepted path may hold whitespace, a shell
// metacharacter, a leading '-' or a ".." segment.
func FuzzValidateRemotePath(f *testing.F) {
	for _, s := range append([]string{"", "/opt/nself", "opt/x", "/a/b.c-d_e"}, hostilePaths...) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if ValidateRemotePath(s) != nil {
			return
		}
		if why := unsafeAccepted(s); why != "" {
			t.Fatalf("accepted %q: %s", s, why)
		}
		for _, seg := range strings.Split(s, "/") {
			if seg == ".." {
				t.Fatalf("accepted %q with a '..' segment", s)
			}
		}
	})
}
