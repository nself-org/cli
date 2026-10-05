package remote

import (
	"context"
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
}

// dotDotPaths hold a ".." segment. cli deploy has always accepted them
// (filepath.Join cleans them), so ValidateRemotePath does too; CopyTo and
// Rsync refuse them through ValidateCopyPath.
var dotDotPaths = []string{"..", "../etc", "/opt/../etc", "/opt/x/..", "a/../../b", "/srv/../app"}

func TestValidateRemotePath_RefusesHostile(t *testing.T) {
	for _, p := range hostilePaths {
		if err := ValidateRemotePath(p); err == nil {
			t.Errorf("ValidateRemotePath(%q) = nil, want error", p)
		}
	}
}

func TestValidateRemotePath_AcceptsDotDotSegments(t *testing.T) {
	for _, p := range dotDotPaths {
		if err := ValidateRemotePath(p); err != nil {
			t.Errorf("ValidateRemotePath(%q) = %v, want nil (deploy accepts it)", p, err)
		}
	}
}

func TestValidateCopyPath_RefusesHostileAndDotDot(t *testing.T) {
	for _, p := range append(append([]string(nil), hostilePaths...), dotDotPaths...) {
		if p == "" {
			continue
		}
		if err := ValidateCopyPath(p); err == nil {
			t.Errorf("ValidateCopyPath(%q) = nil, want error", p)
		}
	}
	for _, p := range []string{"", "/opt/nself", "/opt/a..b", "x-y", "/a/.hidden/b"} {
		if err := ValidateCopyPath(p); err != nil {
			t.Errorf("ValidateCopyPath(%q) = %v, want nil", p, err)
		}
	}
}

func TestCopyToAndRsyncRefuseDotDot(t *testing.T) {
	n := countExecs(t)
	for _, p := range dotDotPaths {
		if CopyTo(context.Background(), ciTarget("h1"), "./a", p) == nil || Rsync(context.Background(), ciTarget("h1"), nil, "./a", p) == nil {
			t.Errorf("remote path %q accepted by CopyTo or Rsync", p)
		}
	}
	if *n != 0 {
		t.Fatalf("%d execs", *n)
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
// metacharacter or a leading '-'. (".." segments are accepted; see
// dotDotPaths.)
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
	})
}

// FuzzValidateCopyPath: as FuzzValidateRemotePath, and no ".." segment.
func FuzzValidateCopyPath(f *testing.F) {
	for _, s := range append([]string{"", "/opt/nself", "/a/b.c-d_e"}, dotDotPaths...) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if ValidateCopyPath(s) != nil {
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

// TestAdvRsyncColonDestDaemonMode: "host:" made the operand "host::/dst",
// which rsync reads as daemon mode (host::module) over the rsync protocol,
// bypassing -e and the pinned host keys. A ':' is refused unless it sits in a
// complete IPv6 literal.
func TestAdvRsyncColonDestDaemonMode(t *testing.T) {
	for _, d := range []string{"node.invalid:", "a:b", "node.invalid:873", "u:x@host", "u@host:", ":::", "1:2", "fe80::1%eth0", "host::module", "a:"} {
		if ValidateDest(d) == nil {
			t.Errorf("ValidateDest(%q) = nil, want error", d)
		}
	}
	for _, d := range []string{"web1", "deploy@203.0.113.7", "2001:db8::1", "deploy@2001:db8::1", "::1", "user+tag@h.example"} {
		if err := ValidateDest(d); err != nil {
			t.Errorf("ValidateDest(%q) = %v, want nil", d, err)
		}
	}
	n := countExecs(t)
	err := Rsync(context.Background(), ciTarget("node.invalid:"), []string{"-az"}, "/tmp/a", "/dst")
	if err == nil || *n != 0 {
		t.Fatalf("Rsync to colon dest: err=%v execs=%d", err, *n)
	}
}
