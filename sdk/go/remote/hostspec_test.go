package remote

// Purpose: tests for ParseHostSpec and HostSpec. Adversarial first: every
// input that could become an ssh option, a second destination, an rsync
// daemon module or a shell fragment must be refused. ssh, scp and rsync are
// fake scripts or counted no-ops; no test connects to a host.

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestParseHostSpec_Valid(t *testing.T) {
	cases := []struct {
		in     string
		want   HostSpec
		canon  string
		dest   string
		sshOpt []string
	}{
		{"host", HostSpec{Host: "host"}, "host", "host", nil},
		{"u@host", HostSpec{User: "u", Host: "host"}, "u@host", "u@host", nil},
		{"u@host:2222", HostSpec{User: "u", Host: "host", Port: 2222}, "u@host:2222", "u@host", []string{"-p", "2222"}},
		{"host:22", HostSpec{Host: "host", Port: 22}, "host:22", "host", []string{"-p", "22"}},
		{"u@10.0.0.1", HostSpec{User: "u", Host: "10.0.0.1"}, "u@10.0.0.1", "u@10.0.0.1", nil},
		{"u@[2001:db8::1]:22", HostSpec{User: "u", Host: "2001:db8::1", Port: 22}, "u@[2001:db8::1]:22", "u@2001:db8::1", []string{"-p", "22"}},
		{"[::1]", HostSpec{Host: "::1"}, "[::1]", "::1", nil},
		{"[2001:DB8::A]", HostSpec{Host: "2001:DB8::A"}, "[2001:DB8::A]", "2001:DB8::A", nil},
		{"[::ffff:10.0.0.1]:2200", HostSpec{Host: "::ffff:10.0.0.1", Port: 2200}, "[::ffff:10.0.0.1]:2200", "::ffff:10.0.0.1", []string{"-p", "2200"}},
		{"u@host:/opt/nself", HostSpec{User: "u", Host: "host", LegacyPath: "/opt/nself"}, "u@host", "u@host", nil},
		{"host:/", HostSpec{Host: "host", LegacyPath: "/"}, "host", "host", nil},
		{"u@[::1]:/srv/app", HostSpec{User: "u", Host: "::1", LegacyPath: "/srv/app"}, "u@[::1]", "u@::1", nil},
		{"u@my_alias", HostSpec{User: "u", Host: "my_alias"}, "u@my_alias", "u@my_alias", nil},
		{"u@_x", HostSpec{User: "u", Host: "_x"}, "u@_x", "u@_x", nil},
		{"_u.1-x@h", HostSpec{User: "_u.1-x", Host: "h"}, "_u.1-x@h", "_u.1-x@h", nil},
		{"Deploy-Host.Example.ORG", HostSpec{Host: "Deploy-Host.Example.ORG"}, "Deploy-Host.Example.ORG", "Deploy-Host.Example.ORG", nil},
		{"u@a.-b", HostSpec{User: "u", Host: "a.-b"}, "u@a.-b", "u@a.-b", nil},
		{"h:1", HostSpec{Host: "h", Port: 1}, "h:1", "h", []string{"-p", "1"}},
		{"h:65535", HostSpec{Host: "h", Port: 65535}, "h:65535", "h", []string{"-p", "65535"}},
		{"h:10000", HostSpec{Host: "h", Port: 10000}, "h:10000", "h", []string{"-p", "10000"}},
		{"h:9", HostSpec{Host: "h", Port: 9}, "h:9", "h", []string{"-p", "9"}},
	}
	for _, c := range cases {
		got, err := ParseHostSpec(c.in)
		if err != nil {
			t.Errorf("ParseHostSpec(%q) = %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseHostSpec(%q) = %+v, want %+v", c.in, got, c.want)
		}
		if s := got.String(); s != c.canon {
			t.Errorf("String(%q) = %q, want %q", c.in, s, c.canon)
		}
		if d := got.Dest(); d != c.dest {
			t.Errorf("Dest(%q) = %q, want %q", c.in, d, c.dest)
		}
		if o := got.SSHOptions(); !reflect.DeepEqual(o, c.sshOpt) {
			t.Errorf("SSHOptions(%q) = %q, want %q", c.in, o, c.sshOpt)
		}
		wantArgs := append(append([]string{}, c.sshOpt...), "--", c.dest)
		if a := got.SSHArgs(); !reflect.DeepEqual(a, wantArgs) {
			t.Errorf("SSHArgs(%q) = %q, want %q", c.in, a, wantArgs)
		}
		if err := got.Validate(); err != nil {
			t.Errorf("Validate(%q) = %v", c.in, err)
		}
		if err := ValidateDest(got.Dest()); err != nil {
			t.Errorf("ValidateDest(%q) = %v: the parser accepted what the funnel refuses", got.Dest(), err)
		}
		again, err := ParseHostSpec(got.String())
		if err != nil || again.String() != got.String() {
			t.Errorf("round trip of %q: %+v, %v", c.in, again, err)
		}
	}
}

func TestParseHostSpec_Invalid(t *testing.T) {
	long := strings.Repeat("a", 63)
	bad := []string{
		// the acceptance table
		"", "-oProxyCommand=x", "u@-h", "u@h;rm", "u@h p", "u@h:0", "u@h:65536", "u@h:022", "u@[::1", "u@h:rel/path", "u@h:/a/../b",
		// option injection through user or host
		"-oX@h", "u@-oProxyCommand=x", "-h", "--", "-", "u@-", "-p", "u@-x", "u@-x:22", "-u@h", "-oProxyCommand=touch$IFS/tmp/x",
		"-o ProxyCommand=x", "u@-o", "-G", "[-oX]", "u@[-oX]:22",
		// second @ or :, empty parts
		"user@host@evil", "u@h@", "@h", "u@", "@", "@@", "u@@h", "u:x@h", ":22", "h:", "h::", "h:22:22", "u@h:22:/x", "u@h::module",
		"host::/path", "h:/a:/b", "::1", "u@2001:db8::1", "2001:db8::1", "u@2001:db8::1:22", ":", ":/x", "u@:22", "u@[]", "[]", "u@[]:22",
		"u@.h", "u@h.", "u@a..b", ".",
		// IPv6 forms
		"[::1]x", "[::1]22", "[::1]/x", "[::1]:", "[::1]:0", "[::1]:rel", "[::1]::22", "[[::1]]", "[::1]]", "[1.2.3.4]", "[host]", "[::g]",
		"[fe80::1%eth0]", "[fe80::1%25eth0]", "[::1", "[", "u@[", "u@[::1]@evil", "[::1]@h", "x[::1]", "u@h[::1]", "[:::1]", "[1:2:3:4:5:6:7:8:9]",
		// ports
		"h:+22", "h:-22", "h: 22", "h:22 ", "h:2.2", "h:0x16", "h:1e2", "h:٢٢", "h:00", "h:01", "h:65536", "h:99999", "h:100000", "h:123456789012345678901234567890",
		"h:2_2", "h:22\n", "h:\x0022",
		// legacy path
		"h:/a b", "h:/a;b", "h:/a$b", "h:/a`b`", "h:/a\nb", "h:/a/..", "h:/../a", "h:/a/../../b", "h:/a|b", "h:/a*b", "h:/a'b", "h:/a\"b", "h:/a%20b", "h:/\x00",
		"h:/é", "h:/a@b", "h:/a:b", "h:/~", "h://a b",
		// whitespace, control bytes, unicode, NUL
		" h", "h ", "h\t", "h\n", "h\r", "u@h\n", "\nu@h", "h\x00", "h\x00.evil", "u\x00@h", "u @h", "u@ h", "u@h\x7f", "h\x1b[0m",
		"é", "hé", "h.é", "u@hé", "éu@h", "hｅ", "h\u00a0", "h\u2028", "h\u200b", "ｈ", "h\xff", "h\xc0\xaf",
		// shell metacharacters and tokens
		"u@h;", "u@h|x", "u@h&x", "u@h$(id)", "u@h`id`", "u@h>x", "u@h<x", "u@h(x)", "u@h{x}", "u@h*", "u@h?", "u@h!", "u@h\"", "u@h'", "u@h\\",
		"u@h#x", "u@h~", "u@h=x", "u@h,x", "u@h/x", "u@h+x", "u+x@h", "u@h%h", "u@%h", "%u@h", "u@h%p", "%h", "u@%%", "u@h%%",
		"$USER@h", "u@$HOST", "~/x", "~u@h",
		// user shape
		"U@h", "Root@h", "1u@h", ".u@h", "-u@h", "u u@h", strings.Repeat("a", 33) + "@h", "u@h:22@x",
		// host shape
		strings.Repeat("a", 254), long + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 63) + "." + strings.Repeat("e", 3),
		"a.b..c", "u@h h", "u@h:",
	}
	for _, in := range bad {
		h, err := ParseHostSpec(in)
		if err == nil {
			t.Errorf("ParseHostSpec(%q) = %+v, want an error", in, h)
			continue
		}
		var he *HostSpecError
		if !errors.As(err, &he) {
			t.Errorf("ParseHostSpec(%q) error %T is not *HostSpecError", in, err)
			continue
		}
		if h != (HostSpec{}) {
			t.Errorf("ParseHostSpec(%q) returned %+v with an error", in, h)
		}
		if he.Reason == "" || (len(in) <= maxEcho && he.Input != in) {
			t.Errorf("ParseHostSpec(%q): Input=%q Reason=%q", in, he.Input, he.Reason)
		}
	}
}

// Two entries in the table above are valid on purpose-built boundaries; keep
// the invalid list honest by checking the boundary neighbours explicitly.
func TestParseHostSpec_Boundaries(t *testing.T) {
	ok := func(in string) {
		t.Helper()
		if _, err := ParseHostSpec(in); err != nil {
			t.Errorf("ParseHostSpec(%.40q len %d) = %v", in, len(in), err)
		}
	}
	no := func(in string) {
		t.Helper()
		if _, err := ParseHostSpec(in); err == nil {
			t.Errorf("ParseHostSpec(%.40q len %d) accepted", in, len(in))
		}
	}
	name := func(n int) string { // dotted name of exactly n bytes, labels of 63
		var b strings.Builder
		for b.Len() < n {
			if b.Len() > 0 {
				b.WriteByte('.')
			}
			b.WriteString(strings.Repeat("a", min(63, n-b.Len())))
		}
		return b.String()
	}
	ok(name(253))
	no(name(254))
	ok(strings.Repeat("a", 32) + "@h")
	no(strings.Repeat("a", 33) + "@h")
	ok("u@h:1")
	no("u@h:0")
	ok("u@h:65535")
	no("u@h:65536")
	no("u@h:65537")
	no("u@h:99999")
	ok("u@h:9999")
	ok("a@h")
	ok("_@h")
	no("0@h")
	ok("a0@h")
	ok("a-@h")
	ok("a.@h")
	ok("a_@h")
	no("a/@h")
	no("a`@h")
	no("a{@h")
	no("a@@h")
	no("a[@h")
	ok("h-")
	ok("h_")
	ok("0h")
	ok("Z")
	ok("z.A")
	ok("0.9")
	no("h/")
	no("h:")
	no("h@")
	no("h`")
	no("h{")
	no("h[")
	no("h@")
	no("h/")
	no("h0:")
	no("h:/..")
	ok("h:/.")
	ok("h:/a..b")
	ok("h:/a/b.c-d_e")
}

func TestParseHostSpec_RejectsEveryByteOutsideTheAllowlist(t *testing.T) {
	allowedHost := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-."
	for c := 0; c < 256; c++ {
		b := string([]byte{byte(c)})
		in := "h" + b + "x"
		_, err := ParseHostSpec(in)
		switch {
		case strings.Contains(allowedHost, b):
			if err != nil {
				t.Errorf("host byte %#x refused: %v", c, err)
			}
		case b == ":" || b == "@":
			// delimiters: "h:x" is a bad port, "h@x" is user h with host x
			if b == ":" && err == nil {
				t.Errorf("%q accepted", in)
			}
		default:
			if err == nil {
				t.Errorf("host byte %#x accepted in %q", c, in)
			}
		}
		if _, err := ParseHostSpec("u" + b + "@h"); (err == nil) != strings.Contains(userRest, b) {
			t.Errorf("user byte %#x after the first: accepted=%v", c, err == nil)
		}
		if _, err := ParseHostSpec(b + "u@h"); (err == nil) != strings.Contains(userFirst, b) {
			t.Errorf("user byte %#x first: accepted=%v", c, err == nil)
		}
		if _, err := ParseHostSpec("h:/a" + b); err == nil && !strings.Contains(RemotePathChars, b) {
			t.Errorf("path byte %#x accepted", c)
		}
		if _, err := ParseHostSpec("h:1" + b); err == nil && !(b >= "0" && b <= "9") {
			t.Errorf("port byte %#x accepted", c)
		}
	}
}

// The user grammar [a-z_][a-z0-9_.-], written out here for the same reason.
const (
	userFirst = "abcdefghijklmnopqrstuvwxyz_"
	userRest  = "abcdefghijklmnopqrstuvwxyz_0123456789.-"
)

// RemotePathChars is the legacy-path charset, written out here so the test
// does not read it from the code under test.
const RemotePathChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/_.-"

func TestParseHostSpec_Reasons(t *testing.T) {
	for in, want := range map[string]string{
		"[::1":       "unterminated",
		"[":          "unterminated",
		"[]":         "brackets hold",
		"[::1]x":     "text after ']'",
		"[::1]:":     "port must be",
		":22":        "empty",
		"h:":         "port must be",
		"h:/a/..":    "legacy path",
		"h:70000":    "at most 65535",
		"h:12a":      "port must be",
		"U@h":        "user must match",
		"u@h@evil":   "host holds a byte",
		"[1.2.3.4]":  "brackets hold",
		"u@h:22:/x":  "port must be",
		"u@2001::1":  "needs brackets",
		"a..b":       "empty",
		"-x":         "start with '-'",
		"u@h:/a b":   "legacy path",
		"u@h:0":      "must not start with '0'",
		"u@h:":       "port must be",
		"u@[::1]:22": "",
	} {
		_, err := ParseHostSpec(in)
		if want == "" {
			if err != nil {
				t.Errorf("%q: %v", in, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseHostSpec(%q) error = %v, want it to mention %q", in, err, want)
		}
	}
}

func TestHostSpecError(t *testing.T) {
	_, err := ParseHostSpec("-oX")
	if err == nil || err.Error() != `invalid host "-oX": host must not start with '-'` {
		t.Fatalf("Error() = %v", err)
	}
	_, err = ParseHostSpec("u@h:0")
	if err == nil || !strings.Contains(err.Error(), "port must not start with '0'") {
		t.Fatalf("port error = %v", err)
	}
	// Input is cut at 128 bytes so a huge value never floods a log line.
	in := "-" + strings.Repeat("a", 127)
	var he *HostSpecError
	_, err = ParseHostSpec(in)
	if !errors.As(err, &he) || he.Input != in {
		t.Fatalf("128-byte input changed: %v", err)
	}
	_, err = ParseHostSpec(in + "a")
	if !errors.As(err, &he) || he.Input != in+"..." {
		t.Fatalf("129-byte input = %q", he.Input)
	}
}

func TestHostSpec_Validate(t *testing.T) {
	good := []HostSpec{
		{Host: "h"}, {User: "u", Host: "h", Port: 22}, {Host: "::1"}, {User: "u", Host: "h", LegacyPath: "/p"},
	}
	for _, h := range good {
		if err := h.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v", h, err)
		}
	}
	bad := []HostSpec{
		{}, {Host: "-oProxyCommand=x"}, {Host: "h", User: "-o"}, {Host: "h", Port: -1}, {Host: "h", Port: 65536},
		{Host: "h", Port: 22, LegacyPath: "/p"}, {Host: "h", LegacyPath: "p"}, {Host: "h", LegacyPath: "/a b"},
		{Host: "a b"}, {Host: "h:22"}, {Host: "[::1]"}, {User: "U", Host: "h"}, {Host: "h", Port: 0, LegacyPath: "/.."},
	}
	for _, h := range bad {
		if err := h.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil", h)
		}
	}
	// A hand-built spec never reaches ssh as an option: the destination follows "--".
	a := HostSpec{Host: "-oProxyCommand=x"}.SSHArgs()
	if !reflect.DeepEqual(a, []string{"--", "-oProxyCommand=x"}) {
		t.Errorf("SSHArgs of a hostile hand-built spec = %q", a)
	}
	if err := ValidateDest(HostSpec{Host: "-oProxyCommand=x"}.Dest()); err == nil {
		t.Error("the funnel accepts a hostile hand-built Dest")
	}
	if (HostSpec{Host: "h", Port: -3}).SSHOptions() != nil || (HostSpec{Host: "h"}).SSHOptions() != nil {
		t.Error("SSHOptions emitted a port for Port <= 0")
	}
	if got := (HostSpec{Host: "h", Port: 1}).SSHOptions(); !reflect.DeepEqual(got, []string{"-p", "1"}) {
		t.Errorf("Port 1 SSHOptions = %q", got)
	}
}

func TestHostSpec_Target(t *testing.T) {
	h, _ := ParseHostSpec("deploy@[2001:db8::1]:2222")
	base := append(CISSHFlags(), CIOptions("n1", "/pin/known_hosts", Version{9, 6})...)
	before := append([]string{}, base...)
	tg := mustTarget(t, h, base)
	if tg.Dest != "deploy@2001:db8::1" {
		t.Errorf("Dest = %q", tg.Dest)
	}
	want := append(append([]string{}, base...), "-p", "2222")
	if !reflect.DeepEqual(tg.Options, want) {
		t.Errorf("Options = %q, want %q", tg.Options, want)
	}
	if !reflect.DeepEqual(base, before) {
		t.Error("Target modified the caller's options slice")
	}
	if err := tg.check(); err != nil {
		t.Errorf("the D4 block plus the port fails the funnel's own check: %v", err)
	}
	h2, _ := ParseHostSpec("h")
	if o := mustTarget(t, h2, nil).Options; o == nil || len(o) != 0 {
		t.Errorf("Target(nil).Options = %#v, want non-nil empty (nil would mean BaseOptions)", o)
	}
}

// The package's ssh, scp and rsync paths, fed a HostSpec target, put "--"
// before the destination and carry the port only as -p (ssh, rsync -e) or -P
// (scp); an IPv6 destination is [addr]:path for scp and rsync.
func TestHostSpecThroughTheFunnel(t *testing.T) {
	ctx := context.Background()
	log := stubTools(t, map[string]string{"ssh": "echo ok", "scp": "exit 0", "rsync": "exit 0"})
	d4 := append(CISSHFlags(), CIOptions("n1", "/pin/known_hosts", Version{9, 6})...)

	h, _ := ParseHostSpec("deploy@[2001:db8::1]:2222")
	tg := mustTarget(t, h, d4)
	if _, err := Run(ctx, tg, "uptime"); err != nil {
		t.Fatal(err)
	}
	if err := CopyTo(ctx, tg, "./env.file", "/opt/nself/.env"); err != nil {
		t.Fatal(err)
	}
	if err := Rsync(ctx, tg, []string{"-a"}, "./src/", "/opt/nself/"); err != nil {
		t.Fatal(err)
	}
	calls := readCalls(t, log)
	if len(calls) != 3 {
		t.Fatalf("%d calls", len(calls))
	}
	ssh, scp, rsync := calls[0].args, calls[1].args, calls[2].args

	// ssh: the port, then "--", the bare IPv6 destination, the command.
	n := len(ssh)
	if !reflect.DeepEqual(ssh[n-5:], []string{"-p", "2222", "--", "deploy@2001:db8::1", "uptime"}) {
		t.Errorf("ssh tail = %q", ssh[n-5:])
	}
	// The same elements HostSpec.SSHArgs() reports (the one builder).
	if !reflect.DeepEqual(ssh[n-5:n-1], h.SSHArgs()) {
		t.Errorf("ssh argv %q does not end with SSHArgs() %q", ssh, h.SSHArgs())
	}
	// scp: -P (never -p), "--", then local and [addr]:path.
	n = len(scp)
	if !reflect.DeepEqual(scp[n-5:], []string{"-P", "2222", "--", "./env.file", "deploy@[2001:db8::1]:/opt/nself/.env"}) {
		t.Errorf("scp tail = %q", scp[n-5:])
	}
	if hasArg(scp, "-p") {
		t.Errorf("scp argv holds -p (preserve times): %q", scp)
	}
	// rsync: the port lives inside the -e string, "--" precedes the operands.
	if rsync[0] != "-e" || !strings.HasSuffix(rsync[1], " -p 2222") || !strings.HasPrefix(rsync[1], "ssh -T -a -x ") {
		t.Errorf("rsync -e = %q", rsync[:2])
	}
	n = len(rsync)
	if !reflect.DeepEqual(rsync[n-3:], []string{"--", "./src/", "deploy@[2001:db8::1]:/opt/nself/"}) {
		t.Errorf("rsync tail = %q", rsync[n-3:])
	}
	for _, a := range rsync[2 : n-3] {
		if a == "-p" || a == "2222" {
			t.Errorf("port leaked outside -e: %q", rsync)
		}
	}

	// No port: no -p/-P anywhere; a plain name destination.
	h, _ = ParseHostSpec("web1")
	tg = mustTarget(t, h, d4)
	if err := CopyTo(ctx, tg, "./f", "/x"); err != nil {
		t.Fatal(err)
	}
	c := readCalls(t, log)[3].args
	if hasArg(c, "-P") || hasArg(c, "-p") || c[len(c)-2] != "./f" || c[len(c)-1] != "web1:/x" {
		t.Errorf("scp without a port: %q", c)
	}
}

// Hostile specs never reach exec: the parser refuses them, and a hand-built
// spec with the same bytes is refused by the funnel.
func TestHostSpecHostileNeverExecs(t *testing.T) {
	ctx := context.Background()
	n := countExecs(t)
	for _, in := range []string{"-oProxyCommand=x", "u@-oProxyCommand=x", "u@h@evil", "u@h:22:/x", "u@h::mod", "u@h p", "u@h;id", "u@h:rel", "u@2001:db8::1"} {
		if _, err := ParseHostSpec(in); err == nil {
			t.Errorf("%q parsed", in)
		}
	}
	for _, h := range []HostSpec{{Host: "-oProxyCommand=x"}, {User: "-oX", Host: "h"}, {Host: "h:mod"}, {Host: "a b"}, {Host: "h;id"}} {
		tg := mustTarget(t, h, []string{})
		if _, err := Run(ctx, tg, "id"); err == nil {
			t.Errorf("Run accepted %+v", h)
		}
		if err := CopyTo(ctx, tg, "./a", "/b"); err == nil {
			t.Errorf("CopyTo accepted %+v", h)
		}
		if err := Rsync(ctx, tg, nil, "./a", "/b"); err == nil {
			t.Errorf("Rsync accepted %+v", h)
		}
	}
	if *n != 0 {
		t.Fatalf("%d processes started for hostile input", *n)
	}
}

func TestResolveSSHHost_HostSpecGrammar(t *testing.T) {
	log := stubTools(t, map[string]string{"ssh": sshGStub})
	cases := []struct{ in, want string }{
		{"web1", "-G -- web1"},
		{"u@web1", "-G -- u@web1"},
		{"u@web1:2222", "-G -p 2222 -- u@web1"},
		{"u@[2001:db8::1]:22", "-G -p 22 -- u@2001:db8::1"},
		{"u@my_alias", "-G -- u@my_alias"},
		{"u@web1:/opt/nself", "-G -- u@web1"}, // legacy path never reaches ssh
	}
	for i, c := range cases {
		if _, err := ResolveSSHHost(context.Background(), c.in); err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got := strings.Join(readCalls(t, log)[i].args, " "); got != c.want {
			t.Errorf("%q: argv = %q, want %q", c.in, got, c.want)
		}
	}
	n := countExecs(t)
	for _, in := range []string{"u@2001:db8::1", "u@h:0", "u@-h", "u@h@evil", "h:rel/p", "u@h:/a/../b", "U@h"} {
		if _, err := ResolveSSHHost(context.Background(), in); err == nil {
			t.Errorf("ResolveSSHHost(%q) accepted", in)
		}
	}
	if *n != 0 {
		t.Fatalf("ssh -G started %d times for refused input", *n)
	}
}

// Every accepted value in the table is safe in every shape it takes.
func TestAcceptedValuesAreSafeEverywhere(t *testing.T) {
	for _, in := range []string{"h", "u@h:22", "u@[::1]:22", "u@h:/opt/x", "_x@_y.z", "u@a.-b"} {
		h, err := ParseHostSpec(in)
		if err != nil {
			t.Fatal(err)
		}
		checkSafeArgv(t, in, h)
	}
}

// checkSafeArgv fails when an element SSHArgs returns, other than the fixed
// "-p" and "--", holds whitespace, a shell metacharacter or a leading '-',
// or when the structure is not [-p N] -- dest.
func checkSafeArgv(t testing.TB, in string, h HostSpec) {
	t.Helper()
	args := h.SSHArgs()
	i := 0
	if h.Port > 0 {
		if len(args) < 2 || args[0] != "-p" || args[1] != strconv.Itoa(h.Port) {
			t.Fatalf("%q: SSHArgs = %q lacks -p %d first", in, args, h.Port)
		}
		i = 2
	}
	if len(args) != i+2 || args[i] != "--" {
		t.Fatalf("%q: SSHArgs = %q is not [-p N] -- dest", in, args)
	}
	for _, a := range append([]string{args[i+1]}, args[:i]...) {
		if a == "-p" {
			continue
		}
		if why := unsafeAccepted(a); why != "" {
			t.Fatalf("%q: SSHArgs element %q: %s", in, a, why)
		}
	}
	if o := h.SSHOptions(); len(o) != i || !reflect.DeepEqual(o, args[:i]) && i > 0 {
		t.Fatalf("%q: SSHOptions %q is not the part of SSHArgs before \"--\" (%q)", in, o, args)
	}
	if err := ValidateDest(h.Dest()); err != nil {
		t.Fatalf("%q: Dest %q fails ValidateDest: %v", in, h.Dest(), err)
	}
}

// parseSSHG keeps ssh -G's port inside 1..65535.
func TestParseSSHG_PortBounds(t *testing.T) {
	for port, ok := range map[string]bool{"0": false, "1": true, "65535": true, "65536": false, "-1": false, "x": false} {
		_, err := parseSSHG("hostname h\nport " + port + "\n")
		if (err == nil) != ok {
			t.Errorf("port %s: err = %v, want ok=%v", port, err, ok)
		}
	}
}

func mustTarget(t testing.TB, h HostSpec, opts []string) Target {
	t.Helper()
	tg, err := h.Target(opts)
	if err != nil {
		t.Fatalf("Target(%q) of %+v: %v", opts, h, err)
	}
	return tg
}

// A port in the options that differs from the spec's is refused; the same
// port, or any port when the spec has none, is not.
func TestHostSpec_TargetRefusesConflictingPort(t *testing.T) {
	d4 := append(CISSHFlags(), CIOptions("n1", "/pin/known_hosts", Version{9, 6})...)
	spec := HostSpec{User: "u", Host: "h", Port: 2222}
	conflicts := [][]string{
		{"-p", "22"}, {"-p22"}, {"-p", "2223"}, {"-oPort=22"}, {"-o", "Port=22"}, {"-o", "Port 22"}, {"-o", "port = 22"},
		{"-oPORT=1"}, {"-i", "/k", "-p", "22"}, {"-q", "-o", "Port=22", "-v"}, {"-p", "x"}, {"-pabc"},
		append(append([]string{}, d4...), "-p", "22"),
	}
	for _, o := range conflicts {
		tg, err := spec.Target(o)
		var he *HostSpecError
		if !errors.As(err, &he) || !strings.Contains(err.Error(), "port") {
			t.Errorf("Target(%q) = %+v, %v; want a port conflict error", o, tg, err)
		}
		if tg.Dest != "" || tg.Options != nil {
			t.Errorf("Target(%q) returned a non-zero Target with an error: %+v", o, tg)
		}
	}
	if _, err := spec.Target([]string{"-p", "22"}); err == nil || !strings.Contains(err.Error(), `"22"`) || !strings.Contains(err.Error(), "2222") {
		t.Errorf("conflict error does not name both ports: %v", err)
	}
	fine := [][]string{
		nil, {}, d4, {"-p", "2222"}, {"-p2222"}, {"-oPort=2222"}, {"-i", "/k"}, {"-i"}, {"-o", "ConnectTimeout=5"},
		{"-oConnectTimeout=5"}, {"-o", "Compression=yes"}, {"-o"}, {"-p"}, {"-o", "Port"}, {"-4", "-q"},
	}
	for _, o := range fine {
		if _, err := spec.Target(o); err != nil {
			t.Errorf("Target(%q) = %v", o, err)
		}
	}
	// Without a spec port, the caller's port is the only one and stands.
	noPort := HostSpec{Host: "h"}
	tg, err := noPort.Target([]string{"-p", "22"})
	if err != nil || !reflect.DeepEqual(tg.Options, []string{"-p", "22"}) {
		t.Errorf("no spec port: %+v, %v", tg, err)
	}
	// optionPort reads all three -o spellings and ignores other keys.
	for in, want := range map[string]string{
		"Port=1": "1", "Port 1": "1", "port = 1": "1", " PORT=1": "1", "Port": "", "Portal=1": "", "ConnectTimeout=5": "", "": "", "Port=": "",
	} {
		if got := optionPort(in); got != want {
			t.Errorf("optionPort(%q) = %q, want %q", in, got, want)
		}
	}
}

// A bare IPv6 address is refused with a reason that says to use brackets.
func TestParseHostSpec_BareIPv6Reason(t *testing.T) {
	for in, fix := range map[string]string{
		"::1":             "[::1]",
		"u@::1":           "[::1]",
		"2001:db8::1":     "[2001:db8::1]",
		"u@2001:db8::1":   "[2001:db8::1]",
		"2001:db8::1:22":  "[2001:db8::1:22]", // an address, not host:port
		"fe80::1":         "[fe80::1]",
		"::ffff:10.0.0.1": "[::ffff:10.0.0.1]",
		"1:2:3:4:5:6:7:8": "[1:2:3:4:5:6:7:8]",
		"u@2001:DB8::A":   "[2001:DB8::A]",
	} {
		_, err := ParseHostSpec(in)
		want := "an IPv6 address needs brackets: " + fix
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseHostSpec(%q) error = %v, want it to say %q", in, err, want)
		}
	}
	// Not IPv6: the generic reasons stay, and IPv4 stays a valid host.
	for in, want := range map[string]string{
		"fe80::1%eth0": "port must be", "h:22:22": "port must be", "u@h::mod": "port must be", "a:b": "port must be", ":": "empty",
	} {
		if _, err := ParseHostSpec(in); err == nil || strings.Contains(err.Error(), "brackets: ") || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseHostSpec(%q) error = %v, want %q and no bracket hint", in, err, want)
		}
	}
	for _, in := range []string{"10.0.0.1", "u@10.0.0.1:22", "1.2.3.4"} {
		if _, err := ParseHostSpec(in); err != nil {
			t.Errorf("ParseHostSpec(%q) = %v", in, err)
		}
	}
}
