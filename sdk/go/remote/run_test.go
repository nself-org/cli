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

func TestRun_CompatArgvIsLegacy(t *testing.T) {
	log := stubTools(t, map[string]string{"ssh": "exit 0"})
	tg := Target{Dest: "u@h", KeyPath: "/k", Compat: true, Env: []string{"PATH=" + pathEnv(), "X=1"}}
	if _, err := Run(context.Background(), tg, "ls"); err != nil {
		t.Fatal(err)
	}
	want := []string{"-i", "/k", "-o", "StrictHostKeyChecking=accept-new", "-o", "ForwardAgent=no", "u@h", "ls"}
	c := readCalls(t, log)[0]
	if !reflect.DeepEqual(c.args, want) {
		t.Fatalf("argv = %q, want %q", c.args, want)
	}
	if !hasArg(c.env, "X=1") {
		t.Error("Target.Env was not used")
	}
}

func TestRun_ErrorFormat(t *testing.T) {
	stubTools(t, map[string]string{"ssh": "echo boom; exit 3"})
	_, err := Run(context.Background(), Target{Dest: "u@h", KeyPath: "/k", Compat: true}, "ls")
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
	strictOnly := map[string]bool{"a;b": true, "$(id)": true, "`id`": true, "a|b": true}
	dests := []string{"-oProxyCommand=touch /tmp/pwned", "-oProxyCommand=x", "a b", "a;b", "a\nb", "$(id)", "`id`", "a|b", "-p", "", "a\x00b"}
	for _, compat := range []bool{false, true} {
		for _, d := range dests {
			if compat && strictOnly[d] {
				continue // Compat keeps cli deploy's historical leniency for these
			}
			n := countExecs(t)
			tg := Target{Dest: d, Options: []string{}, Compat: compat}
			if _, err := Run(ctx, tg, "true"); err == nil {
				t.Errorf("Run dest %q compat=%v: no error", d, compat)
			}
			if _, err := Start(ctx, tg, "true"); err == nil {
				t.Errorf("Start dest %q compat=%v: no error", d, compat)
			}
			if !compat {
				if err := CopyTo(ctx, tg, "/tmp/a", "/opt/a"); err == nil {
					t.Errorf("CopyTo dest %q: no error", d)
				}
				if err := Rsync(ctx, tg, nil, "/tmp/a", "/opt/a"); err == nil {
					t.Errorf("Rsync dest %q: no error", d)
				}
			}
			if *n != 0 {
				t.Errorf("dest %q compat=%v: %d execs before refusal", d, compat, *n)
			}
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
	for _, l := range []string{"evil:file", "", "a\nb"} {
		if CopyTo(ctx, tg, l, "/opt/a") == nil || Rsync(ctx, tg, nil, l, "/opt/a") == nil {
			t.Errorf("local operand %q not refused", l)
		}
	}
	for _, a := range []string{"-e", "-eX", "-avze", "-M", "--rsh=sh -c id", "--rsync-path=sh", "--remote-option=x"} {
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
	if c.args[0] != "-az" || c.args[1] != "-e" || !strings.HasPrefix(c.args[2], "ssh -o BatchMode=yes ") {
		t.Fatalf("argv head = %q", c.args[:3])
	}
	n := len(c.args)
	if !reflect.DeepEqual(c.args[n-3:], []string{"--", "./src/", "h1:/opt/x/"}) {
		t.Fatalf("tail = %q", c.args[n-3:])
	}
	if strings.Contains(c.args[2], " -T") || strings.Contains(c.args[2], " -a ") {
		t.Errorf("rsync -e holds ssh-only flags: %s", c.args[2])
	}
}

func TestRsync_CompatMatchesLegacyShape(t *testing.T) {
	log := stubTools(t, map[string]string{"rsync": "exit 0"})
	tg := Target{Dest: "u@h", KeyPath: "/k", Compat: true}
	if err := Rsync(context.Background(), tg, []string{"-az"}, "/w/c.yml", "/opt/app/nself-compose.yml"); err != nil {
		t.Fatal(err)
	}
	want := []string{"-az", "-e", "ssh -i /k -o StrictHostKeyChecking=accept-new -o ForwardAgent=no", "/w/c.yml", "u@h:/opt/app/nself-compose.yml"}
	if got := readCalls(t, log)[0].args; !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %q, want %q", got, want)
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
