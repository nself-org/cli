package remote

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func ciTarget(dest string) Target {
	opts := append(CISSHFlags(), CIOptions("n1", "/pin/known_hosts", Version{9, 6})...)
	return Target{Dest: dest, Options: opts}
}

func TestRun_StrictArgvShape(t *testing.T) {
	log := stubTools(t, map[string]string{"ssh": "echo out"})
	out, err := Run(context.Background(), ciTarget("deploy@203.0.113.7"), "agent --stdio")
	if err != nil || out != "out" {
		t.Fatalf("Run = %q, %v", out, err)
	}
	c := readCalls(t, log)[0]
	n := len(c.args)
	if c.args[n-3] != "--" || c.args[n-2] != "deploy@203.0.113.7" || c.args[n-1] != "agent --stdio" {
		t.Fatalf("tail of argv = %q, want -- dest command", c.args[n-3:])
	}
	for _, f := range []string{"-T", "-a", "-x"} {
		if !hasArg(c.args, f) {
			t.Errorf("ssh argv lacks %s", f)
		}
	}
	if !hasArg(c.args, "StrictHostKeyChecking=yes") || !hasArg(c.args, "UserKnownHostsFile=/pin/known_hosts") {
		t.Error("ssh argv lacks the pinned host key options")
	}
}

func TestRun_ErrorFormat(t *testing.T) {
	stubTools(t, map[string]string{"ssh": "echo boom; exit 3"})
	_, err := Run(context.Background(), Target{Dest: "u@h", KeyPath: "/k"}, "ls")
	if err == nil || !strings.HasPrefix(err.Error(), "remote command on u@h failed: exit status 3\nboom") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunArgv_QuotesEveryElement(t *testing.T) {
	log := stubTools(t, map[string]string{"ssh": "exit 0"})
	_, err := RunArgv(context.Background(), ciTarget("h1"), "docker", "exec", "c", "sh", "-c", "id; $(x) 'q'")
	if err != nil {
		t.Fatal(err)
	}
	c := readCalls(t, log)[0]
	want := `'docker' 'exec' 'c' 'sh' '-c' 'id; $(x) '\''q'\'''`
	if c.args[len(c.args)-1] != want {
		t.Fatalf("command = %q, want %q", c.args[len(c.args)-1], want)
	}
}

func TestDestinationInjectionRefusedBeforeExec(t *testing.T) {
	ctx := context.Background()
	dests := []string{"-oProxyCommand=touch /tmp/pwned", "-oProxyCommand=x", "a b", "a;b", "a\nb", "$(id)", "`id`", "a|b", "-p", "", "a\x00b", "Admin@host", "u@h@e", "2001:db8::1"}
	for _, d := range dests {
		n := countExecs(t)
		tg := Target{Dest: d, Options: []string{}}
		if _, err := Run(ctx, tg, "true"); err == nil {
			t.Errorf("Run dest %q: no error", d)
		}
		if _, err := Start(ctx, tg, "true"); err == nil {
			t.Errorf("Start dest %q: no error", d)
		}
		if err := CopyTo(ctx, tg, "/tmp/a", "/opt/a"); err == nil {
			t.Errorf("CopyTo dest %q: no error", d)
		}
		if err := Rsync(ctx, tg, nil, "/tmp/a", "/opt/a"); err == nil {
			t.Errorf("Rsync dest %q: no error", d)
		}
		if *n != 0 {
			t.Errorf("dest %q: %d execs before refusal", d, *n)
		}
	}
}

func TestRemotePathInjectionRefusedBeforeExec(t *testing.T) {
	ctx := context.Background()
	for _, p := range hostilePaths {
		if p == "" {
			continue
		}
		n := countExecs(t)
		tg := ciTarget("h1")
		if err := CopyTo(ctx, tg, "/tmp/a", p); err == nil {
			t.Errorf("CopyTo remote %q: no error", p)
		}
		if err := Rsync(ctx, tg, []string{"-az"}, "/tmp/a", p); err == nil {
			t.Errorf("Rsync remote %q: no error", p)
		}
		if *n != 0 {
			t.Errorf("remote path %q: %d execs happened before refusal", p, *n)
		}
	}
}

func TestLocalOperandAndOptionAbuseRefused(t *testing.T) {
	ctx := context.Background()
	n := countExecs(t)
	tg := ciTarget("h1")
	for _, l := range []string{"evil:file", "", "a\nb", "-rf", "-e"} {
		if CopyTo(ctx, tg, l, "/opt/a") == nil || Rsync(ctx, tg, nil, l, "/opt/a") == nil {
			t.Errorf("local operand %q not refused", l)
		}
	}
	for _, a := range []string{"-e", "-eX", "-avze", "-M", "--rsh=sh -c id", "--rsync-path=sh", "--remote-option=x", "--rs=sh", "--rsync-pat=sh", "--remote-o=x", "--rsh"} {
		if Rsync(ctx, tg, []string{a}, "/tmp/a", "/opt/a") == nil {
			t.Errorf("rsync flag %q not refused", a)
		}
	}
	bad := Target{Dest: "h1", Options: []string{"-o", "UserKnownHostsFile=/a b"}}
	if _, err := Run(ctx, bad, "x"); err == nil {
		t.Error("whitespace in UserKnownHostsFile accepted")
	}
	if _, err := Run(ctx, Target{Dest: "h1", Options: []string{"-o", "X=a\nProxyCommand=id"}}, "x"); err == nil {
		t.Error("newline in option accepted")
	}
	if Rsync(ctx, Target{Dest: "h1", Options: []string{"-o", "ProxyCommand=a b"}}, nil, "/a", "/b") == nil {
		t.Error("whitespace option accepted by rsync -e")
	}
	if *n != 0 {
		t.Fatalf("%d execs before refusal", *n)
	}
}

func TestCopyTo_NoSSHOnlyFlagsAndDashDash(t *testing.T) {
	log := stubTools(t, map[string]string{"scp": "exit 0", "ssh": "exit 0"})
	opts := CIOptions("n1", "/pin/known_hosts", Version{9, 6}) // options only, as scp needs
	tg := Target{Dest: "deploy@203.0.113.7", Options: opts}
	if err := CopyTo(context.Background(), tg, "/tmp/a.tar", "/opt/nself/a.tar"); err != nil {
		t.Fatal(err)
	}
	c := readCalls(t, log)[0]
	for _, f := range []string{"-T", "-a", "-x"} {
		if hasArg(c.args, f) {
			t.Errorf("scp argv holds ssh-only flag %s", f)
		}
	}
	n := len(c.args)
	want := []string{"--", "/tmp/a.tar", "deploy@203.0.113.7:/opt/nself/a.tar"}
	if !reflect.DeepEqual(c.args[n-3:], want) {
		t.Fatalf("tail = %q, want %q", c.args[n-3:], want)
	}
}

func TestRsync_ArgvShape(t *testing.T) {
	log := stubTools(t, map[string]string{"rsync": "exit 0"})
	opts := CIOptions("n1", "/pin/known_hosts", Version{9, 6})
	if err := Rsync(context.Background(), Target{Dest: "h1", Options: opts}, []string{"-az"}, "./src/", "/opt/x/"); err != nil {
		t.Fatal(err)
	}
	c := readCalls(t, log)[0]
	if c.args[0] != "-e" || c.args[2] != "-az" || !strings.HasPrefix(c.args[1], "ssh -o BatchMode=yes ") {
		t.Fatalf("argv head = %q", c.args[:3])
	}
	n := len(c.args)
	if !reflect.DeepEqual(c.args[n-3:], []string{"--", "./src/", "h1:/opt/x/"}) {
		t.Fatalf("tail = %q", c.args[n-3:])
	}
	if strings.Contains(c.args[1], " -T") || strings.Contains(c.args[1], " -a ") {
		t.Errorf("rsync -e holds ssh-only flags: %s", c.args[1])
	}
}

func TestStart_SessionPipes(t *testing.T) {
	stubTools(t, map[string]string{"ssh": "cat"})
	s, err := Start(context.Background(), ciTarget("h1"), "agent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stdin.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	_ = s.Stdin.Close()
	buf := make([]byte, 4)
	if _, err := s.Stdout.Read(buf); err != nil || string(buf) != "ping" {
		t.Fatalf("read %q, %v", buf, err)
	}
	if err := s.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestShellQuote(t *testing.T) {
	if got := ShellQuote("O'Brien"); got != `'O'\''Brien'` {
		t.Fatalf("ShellQuote = %q", got)
	}
}

func TestBaseOptionsAndKeyPath(t *testing.T) {
	want := []string{"-i", "/k", "-o", "StrictHostKeyChecking=accept-new", "-o", "ForwardAgent=no"}
	if got := BaseOptions("/k"); !reflect.DeepEqual(got, want) {
		t.Fatalf("BaseOptions = %q", got)
	}
	t.Setenv("NSELF_DEPLOY_KEY_PATH", "")
	t.Setenv("NSELF_DEPLOY_SSH_KEY", "/legacy")
	if DefaultKeyPath() != "/legacy" {
		t.Error("legacy key env ignored")
	}
	t.Setenv("NSELF_DEPLOY_KEY_PATH", "/new")
	if DefaultKeyPath() != "/new" {
		t.Error("NSELF_DEPLOY_KEY_PATH ignored")
	}
	t.Setenv("NSELF_DEPLOY_KEY_PATH", "")
	t.Setenv("NSELF_DEPLOY_SSH_KEY", "")
	t.Setenv("HOME", "/home/x")
	if DefaultKeyPath() != "/home/x/.ssh/id_ed25519" {
		t.Errorf("default = %q", DefaultKeyPath())
	}
}

// TestAdvRsyncDoubleDashDropsD4 is the reviewer's repro: a caller "--" ahead of
// the Target's -e made rsync read -e and its D4 options as operands, so the
// transfer ran over plain ssh. The "--" and every non-option element are now
// refused before exec.
func TestAdvRsyncDoubleDashDropsD4(t *testing.T) {
	n := countExecs(t)
	tg := ciTarget("node.invalid")
	for _, args := range [][]string{{"-az", "--"}, {"--"}, {"-az", "other.invalid:/etc/shadow"}, {"--exclude", "x"}, {"x"}, {""}} {
		if err := Rsync(context.Background(), tg, args, "/tmp/a", "/dst"); err == nil {
			t.Errorf("Rsync args %q accepted", args)
		}
	}
	if *n != 0 {
		t.Fatalf("%d execs before refusal", *n)
	}
}

func TestRsync_AcceptsSingleElementOptions(t *testing.T) {
	log := stubTools(t, map[string]string{"rsync": "exit 0"})
	args := []string{"-az", "--exclude=.git", "--delete"}
	tg := Target{Dest: "h1", Options: CIOptions("n1", "/pin/known_hosts", Version{9, 6})}
	if err := Rsync(context.Background(), tg, args, "./s", "/opt/x"); err != nil {
		t.Fatal(err)
	}
	got := readCalls(t, log)[0].args
	if got[0] != "-e" || !strings.HasPrefix(got[1], "ssh -o BatchMode=yes ") || !reflect.DeepEqual(got[2:5], args) || got[5] != "--" {
		t.Fatalf("argv = %q", got)
	}
}

func TestRunRefusesLeadingDashCommandAndArgvBackslash(t *testing.T) {
	n := countExecs(t)
	ctx := context.Background()
	if _, err := Run(ctx, ciTarget("h1"), "-oProxyCommand=x"); err == nil {
		t.Error("command starting with '-' accepted")
	}
	// S4: under fish a backslash inside single quotes still escapes, so
	// ShellQuote(`\';id #`) is not a safe single word there.
	for _, a := range []string{`\';id #`, `a\b`, `\`} {
		if _, err := RunArgv(ctx, ciTarget("h1"), "echo", a); err == nil {
			t.Errorf("RunArgv accepted element %q", a)
		}
	}
	if *n != 0 {
		t.Fatalf("%d execs before refusal", *n)
	}
}

func TestIPv6DestinationIsBracketedForScpAndRsync(t *testing.T) {
	log := stubTools(t, map[string]string{"scp": "exit 0", "rsync": "exit 0", "ssh": "exit 0"})
	ctx := context.Background()
	if _, err := Run(ctx, ciTarget("deploy@2001:db8::7"), "true"); err == nil {
		t.Fatal("hand-built bare IPv6 destination accepted")
	}
	spec, err := ParseHostSpec("deploy@[2001:db8::7]:2222")
	if err != nil {
		t.Fatal(err)
	}
	tg, err := spec.Target(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := CopyTo(ctx, tg, "./a", "/opt/a"); err != nil {
		t.Fatal(err)
	}
	if err := Rsync(ctx, tg, []string{"-az"}, "./a", "/opt/a"); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, tg, "true"); err != nil {
		t.Fatal(err)
	}
	cs := readCalls(t, log)
	for _, c := range cs[:2] {
		if last := c.args[len(c.args)-1]; last != "deploy@[2001:db8::7]:/opt/a" {
			t.Errorf("%s operand = %q", c.tool, last)
		}
	}
	if a := cs[2].args; a[len(a)-2] != "deploy@2001:db8::7" { // ssh takes the bare literal
		t.Errorf("ssh dest = %q", a[len(a)-2])
	}
}
