//go:build darwin || linux

package maintenance

import (
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the entitlement timer goldens")

// fakeHost is a host whose commands act on in-memory state; root is the trust anchor.
type fakeHost struct {
	host
	cron         string
	active       map[string]bool
	calls        []string
	cronBroken   bool
	launchLoaded bool
}

func newFake(t *testing.T, goos string, euid int) (*fakeHost, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if euid != 0 {
		if euid = os.Geteuid(); euid == 0 {
			t.Skip("needs a non-root user")
		}
	}
	f := &fakeHost{active: map[string]bool{}}
	f.host = host{goos: goos, euid: euid, systemd: true, crontab: true, home: root, trustAbove: root,
		unitDir: filepath.Join(root, "units"), launchDir: filepath.Join(root, "agents")}
	f.run = f.exec
	oldHost := currentHost
	currentHost = func() host { return f.host }
	t.Cleanup(func() { currentHost = oldHost })
	t.Setenv("NSELF_ENTITLEMENT_AUTOREFRESH", "")
	return f, root
}

func (f *fakeHost) exec(stdin, name string, args ...string) (string, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	switch {
	case name == "crontab" && args[0] == "-l":
		if f.cronBroken {
			return "crontab: cannot read the spool: permission denied", fmt.Errorf("exit 1")
		}
		if f.cron == "" {
			return "no crontab for app", fmt.Errorf("exit 1")
		}
		return f.cron, nil
	case name == "crontab":
		f.cron = stdin
	case name == "systemctl" && args[0] == "is-active":
		if !f.active[args[len(args)-1]] {
			return "", fmt.Errorf("inactive")
		}
	case name == "systemctl" && args[0] == "enable":
		f.active[args[len(args)-1]] = true
	case name == "systemctl" && args[0] == "disable":
		delete(f.active, args[len(args)-1])
	case name == "launchctl" && args[0] == "print":
		if !f.launchLoaded {
			return "", fmt.Errorf("not loaded")
		}
	case name == "launchctl" && args[0] == "bootstrap":
		f.launchLoaded = true
	case name == "launchctl" && args[0] == "bootout":
		f.launchLoaded = false
	}
	return "", nil
}

// project makes root/proj (0755) and root/bin/nself (0755) and returns their paths.
func project(t *testing.T, root string) (dir, bin string) {
	t.Helper()
	dir, bin = filepath.Join(root, "proj"), filepath.Join(root, "bin", "nself")
	for _, d := range []string{dir, filepath.Dir(bin)} {
		if err := os.MkdirAll(d, 0o755); err != nil || os.Chmod(d, 0o755) != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, bin
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", "entitlement_timer", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil || os.WriteFile(p, got, 0o644) != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil || string(want) != string(got) {
		t.Fatalf("%s differs from the golden (run with -update to refresh)\n--- got ---\n%s", name, got)
	}
}

const gBin, gDir = "/usr/local/bin/nself", "/srv/app"

func TestEntitlementTimerGolden(t *testing.T) {
	h := Hash(gDir)
	svc, tmr, err := renderSystemd(gBin, gDir, h, 1000)
	cron, err2 := renderCron(gBin, gDir, h)
	plist, err3 := renderPlist(gBin, gDir, h)
	if err != nil || err2 != nil || err3 != nil {
		t.Fatal(err, err2, err3)
	}
	golden(t, "systemd.service", []byte(svc))
	golden(t, "systemd.timer", []byte(tmr))
	golden(t, "crontab", []byte(cron+"\n"))
	golden(t, "launchagent.plist", plist)
	for _, out := range []string{svc, tmr, cron, string(plist)} {
		for _, secret := range []string{"eyJ", "NSELF_PLUGIN_LICENSE_KEY", "sub"} {
			if strings.Contains(out, secret) {
				t.Errorf("rendered text contains %q", secret)
			}
		}
	}

	for _, tc := range []struct {
		goos, backend string
		euid          int
	}{{"linux", BackendSystemd, 0}, {"linux", BackendCron, 1000}, {"darwin", BackendLaunchAgent, 501}} {
		t.Run(tc.backend, func(t *testing.T) {
			f, root := newFake(t, tc.goos, tc.euid)
			dir, bin := project(t, root)
			f.exe = bin
			first, err := f.ensure(dir, bin)
			if err != nil || !first.Changed || first.Backend != tc.backend {
				t.Fatalf("first ensure = %+v, %v", first, err)
			}
			second, err := f.ensure(dir, bin)
			if err != nil || second.Changed {
				t.Fatalf("second ensure = %+v, %v (want no change)", second, err)
			}
			if st := Status(dir); !st.Active || st.Backend != tc.backend {
				t.Fatalf("status = %+v", st)
			}
			if strings.Contains(f.cron, "no crontab") {
				t.Fatalf("the crontab-list error text was written back as a crontab line: %q", f.cron)
			}
			if _, tagged := taggedLines(f.cron, entCronTag); len(tagged) > 1 {
				t.Fatalf("%d cron lines", len(tagged))
			}
			t.Setenv("NSELF_ENTITLEMENT_AUTOREFRESH", "false")
			f2, root2 := newFake(t, tc.goos, tc.euid)
			t.Setenv("NSELF_ENTITLEMENT_AUTOREFRESH", "false")
			dir2, bin2 := project(t, root2)
			res, err := f2.ensure(dir2, bin2)
			if err != nil || !res.Disabled || res.Changed || len(f2.calls) != 0 || f2.cron != "" {
				t.Fatalf("disabled ensure = %+v, %v, calls %v", res, err, f2.calls)
			}
			if _, err := os.Stat(f2.unitDir); err == nil {
				t.Fatal("disabled ensure wrote unit files")
			}
		})
	}
}

func TestEnsureUpdatesOnChangeAndRemoveByBackend(t *testing.T) {
	f, root := newFake(t, "linux", 1000)
	dir, bin := project(t, root)
	f.exe = bin
	if _, err := f.ensure(dir, bin); err != nil {
		t.Fatal(err)
	}
	f.cron = "MAILTO=x\n" + f.cron
	if _, err := f.ensure(dir, bin); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.cron, "MAILTO=x") || strings.Count(f.cron, entCronTag+Hash(dir)) != 2 {
		t.Fatalf("ensure dropped a foreign crontab line or duplicated ours: %q", f.cron)
	}
	if res, err := Remove(dir); err != nil || !res.Removed || strings.Contains(f.cron, entCronTag) || !strings.Contains(f.cron, "MAILTO=x") {
		t.Fatalf("remove = %+v, %v, crontab %q", res, err, f.cron)
	}
	if st := Status(dir); st.Active || st.Reason != ReasonNotInstalled {
		t.Fatalf("status after remove = %+v", st)
	}
	// systemd units exist but this process is not root: report RootRequired, remove nothing.
	f.systemd = true
	root0 := f.host
	root0.euid = 0
	if _, err := root0.ensure(dir, bin); err != nil {
		t.Fatal(err)
	}
	if svc, _ := os.ReadFile(filepath.Join(f.unitDir, entUnitName(Hash(dir))+".service")); !strings.Contains(string(svc), fmt.Sprintf("User=%d\n", os.Getuid())) {
		t.Fatalf("unit does not run as the project owner:\n%s", svc)
	}
	if st := Status(dir); !st.Active || st.Backend != BackendSystemd {
		t.Fatalf("a non-root doctor must see the root-installed unit: %+v", st)
	}
	res, _ := Remove(dir)
	if !res.RootRequired || res.Removed || !fileExists(filepath.Join(f.unitDir, entUnitName(Hash(dir))+".timer")) {
		t.Fatalf("non-root remove of systemd units = %+v", res)
	}
	if res, _ := root0.remove(""); !res.Removed || fileExists(filepath.Join(f.unitDir, entUnitName(Hash(dir))+".timer")) {
		t.Fatalf("root remove-all = %+v", res)
	}
	noBackend := f.host
	noBackend.crontab, noBackend.euid = false, os.Geteuid()
	if _, err := noBackend.ensure(dir, bin); err != ErrNoBackend {
		t.Fatalf("no backend err = %v", err)
	}
}

// TestCronReadFailureKeepsForeignLines: a crontab we cannot read is never overwritten.
func TestCronReadFailureKeepsForeignLines(t *testing.T) {
	f, root := newFake(t, "linux", 1000)
	dir, bin := project(t, root)
	f.exe = bin
	f.cron, f.cronBroken = "MAILTO=ops\n0 3 * * * backup\n", true
	if _, err := f.ensure(dir, bin); err == nil {
		t.Fatal("ensure succeeded although crontab -l failed")
	}
	if f.cron != "MAILTO=ops\n0 3 * * * backup\n" {
		t.Fatalf("foreign crontab was rewritten: %q", f.cron)
	}
	if res, err := Remove(dir); err != nil || res.Removed || f.cron != "MAILTO=ops\n0 3 * * * backup\n" {
		t.Fatalf("remove with an unreadable crontab: %+v %v %q", res, err, f.cron)
	}
}

func TestHostilePathRefused(t *testing.T) {
	f, root := newFake(t, "linux", 1000)
	dir, bin := project(t, root)
	ww := filepath.Join(root, "ww")
	if err := os.Mkdir(ww, 0o755); err != nil || os.Chmod(ww, 0o777) != nil {
		t.Fatal(err)
	}
	wwProj := filepath.Join(ww, "proj")
	if err := os.Mkdir(wwProj, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(wwProj, link); err != nil {
		t.Fatal(err)
	}
	gw := filepath.Join(root, "gw")
	if err := os.Mkdir(gw, 0o755); err != nil || os.Chmod(gw, 0o775) != nil {
		t.Fatal(err)
	}
	bad := map[string]string{
		"space": dir + " x", "squote": dir + "'", "dquote": dir + `"`, "subshell": dir + "$(id)", "backtick": dir + "`id`",
		"percent": dir + "%", "semicolon": dir + ";", "newline": dir + "\nx", "dotdot": dir + "/../proj", "lt": dir + "<", "gt": dir + ">",
		"amp": dir + "&", "relative": "proj", "empty": "", "worldwritable-parent": wwProj, "symlink-into-writable": link, "dot-segment": dir + "/./",
	}
	for name, p := range bad {
		for _, as := range []string{"dir", "bin"} {
			t.Run(name+"/"+as, func(t *testing.T) {
				d, b := dir, bin
				if as == "dir" {
					d = p
				} else {
					b = p
				}
				if _, err := f.ensure(d, b); err == nil {
					t.Fatalf("ensure(%q, %q) succeeded", d, b)
				}
				if _, err := os.Stat(f.unitDir); err == nil || f.cron != "" || len(f.calls) != 0 {
					t.Fatalf("a refused path touched the host: cron %q calls %v", f.cron, f.calls)
				}
			})
		}
	}
	// A group-writable project directory itself is refused as well.
	if _, err := f.ensure(gw, bin); err == nil {
		t.Fatal("group-writable project dir accepted")
	}
	if st := f.status(wwProj); st.Active || st.Reason == "" {
		t.Fatalf("status of a refused dir = %+v", st)
	}
	if _, err := f.ensure(dir, bin); err != nil {
		t.Fatalf("clean paths refused: %v", err)
	}
}

func TestRootStepGolden(t *testing.T) {
	dirs := []string{gDir, "/home/ubuntu/My-App.v2", "/opt/a_b/c"}
	var all strings.Builder
	for _, d := range dirs {
		h := Hash(d)
		svc, _, err := renderSystemd(gBin, d, h, 0)
		if err != nil {
			t.Fatal(err)
		}
		step := rootStep(h)
		line := "ExecStopPost=+/bin/sh -c '" + systemdEscape(step) + "'"
		if !strings.Contains(svc, line) {
			t.Fatalf("service lacks the root step for %s", d)
		}
		for _, p := range []string{d, gBin} {
			if strings.Contains(step, p) {
				t.Errorf("root step contains %q", p)
			}
		}
		for _, w := range strings.FieldsFunc(step, func(r rune) bool { return r == ' ' || r == ';' }) {
			switch {
			case w == "if" || w == "then" || w == "fi" || w == "test" || w == "systemctl" || w == "rm" || w == "-f" || w == "=" || w == "78" ||
				w == "disable" || w == "--now" || w == "daemon-reload" || w == `"$EXIT_STATUS"`:
			case strings.HasPrefix(w, entSystemdDir+"/"+entUnitName(h)+".") || strings.HasPrefix(w, entUnitName(h)+"."):
			default:
				t.Errorf("unexpected word %q in root step", w)
			}
		}
		fmt.Fprintf(&all, "%s\n", step)
	}
	golden(t, "rootstep.txt", []byte(strings.ReplaceAll(all.String(), Hash(gDir), "HASH0")))
}

func TestLaunchAgentGuard(t *testing.T) {
	for _, gone := range []string{"binary", "dir"} {
		t.Run(gone, func(t *testing.T) {
			tmp, _ := filepath.EvalSymlinks(t.TempDir())
			stubs, home, proj := filepath.Join(tmp, "stubs"), filepath.Join(tmp, "home"), filepath.Join(tmp, "proj")
			bin, calls := filepath.Join(tmp, "nself"), filepath.Join(tmp, "calls")
			for _, d := range []string{stubs, home, proj, filepath.Join(home, "Library", "LaunchAgents")} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			stub := "#!/bin/sh\necho \"$0 $*\" >> " + calls + "\n"
			for _, p := range []string{filepath.Join(stubs, "launchctl"), bin} {
				if err := os.WriteFile(p, []byte(stub), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			hash := Hash(proj)
			data, err := renderPlist(bin, proj, hash)
			if err != nil {
				t.Fatal(err)
			}
			plist := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel(hash)+".plist")
			if err := os.WriteFile(plist, data, 0o644); err != nil {
				t.Fatal(err)
			}
			if gone == "binary" {
				_ = os.Remove(bin)
			} else {
				_ = os.Remove(proj)
			}
			args := plistArgs(t, data)
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Env = []string{"HOME=" + home, "PATH=" + stubs + ":/usr/bin:/bin"}
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("guard script failed: %v\n%s", err, out)
			}
			log, _ := os.ReadFile(calls)
			if fileExists(plist) || !strings.Contains(string(log), "bootout gui/") || !strings.Contains(string(log), launchAgentLabel(hash)) {
				t.Fatalf("plist exists=%v, calls:\n%s", fileExists(plist), log)
			}
			if strings.Contains(string(log), "license refresh") {
				t.Fatalf("refresh ran although the %s is gone", gone)
			}
		})
	}
}

// plistArgs decodes ProgramArguments from a rendered plist.
func plistArgs(t *testing.T, data []byte) []string {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var strs []string
	var in, inArray bool
	var key string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			in = e.Name.Local == "string" || e.Name.Local == "key"
			if e.Name.Local == "array" {
				inArray = true
			}
			key = e.Name.Local
		case xml.EndElement:
			if e.Name.Local == "array" {
				inArray = false
			}
			in = false
		case xml.CharData:
			if in && inArray && key == "string" {
				strs = append(strs, string(e))
			}
		}
	}
	if len(strs) != 3 || strs[0] != "/bin/sh" || strs[1] != "-c" {
		t.Fatalf("ProgramArguments = %q", strs)
	}
	return strs
}

// systemdUnquote undoes the quoting of one ExecStart word: \\ \' %% $$.
func systemdUnquote(s string) string {
	return strings.NewReplacer(`\\`, `\`, `\'`, `'`, "%%", "%", "$$", "$").Replace(s)
}

func shSyntaxOK(script string) bool {
	return exec.Command("sh", "-n", "-c", script).Run() == nil
}

// FuzzRenderTimer: a path either fails the textual path rule or every renderer round-trips.
func FuzzRenderTimer(f *testing.F) {
	f.Add("/srv/app", "/usr/local/bin/nself")
	f.Add("/home/u/My-App.v2/x_y", "/opt/nself/bin/nself")
	f.Add("/a/b' c", "/bin/n$x")
	f.Add("/p%q", "/q\\r")
	f.Fuzz(func(t *testing.T, dir, bin string) {
		for _, p := range []string{dir, bin} {
			if !safePathRe.MatchString(p) || filepath.Clean(p) != p {
				return
			}
		}
		hash := Hash(dir)
		svc, tmr, err := renderSystemd(bin, dir, hash, 1000)
		cron, err2 := renderCron(bin, dir, hash)
		plist, err3 := renderPlist(bin, dir, hash)
		if err != nil || err2 != nil || err3 != nil {
			t.Fatal(err, err2, err3)
		}
		if strings.Count(tmr, "\n") < 5 || strings.Contains(cron, "\n") {
			t.Fatalf("malformed output: cron %q", cron)
		}
		var execStart string
		for _, l := range strings.Split(svc, "\n") {
			if v, ok := strings.CutPrefix(l, "ExecStart=/bin/sh -c '"); ok {
				execStart = systemdUnquote(strings.TrimSuffix(v, "'"))
			}
			if v, ok := strings.CutPrefix(l, "ExecStopPost=+/bin/sh -c '"); ok {
				step := systemdUnquote(strings.TrimSuffix(v, "'"))
				if step != rootStep(hash) || strings.Contains(strings.ReplaceAll(step, hash, ""), "nself license") {
					t.Fatalf("root step %q", step)
				}
			}
		}
		if execStart == "" || !shSyntaxOK(execStart) || !strings.Contains(execStart, shQuote(dir)) {
			t.Fatalf("systemd ExecStart does not round-trip: %q", execStart)
		}
		fields := strings.SplitN(strings.ReplaceAll(cron, `\%`, "%"), " ", 6)
		if len(fields) != 6 || !shSyntaxOK(fields[5]) || !strings.HasSuffix(cron, entCronTag+hash) {
			t.Fatalf("cron line does not round-trip: %q", cron)
		}
		if strings.Contains(cron, "%") && !strings.Contains(cron, `\%`) {
			t.Fatalf("unescaped %% in %q", cron)
		}
		var script string
		dec := xml.NewDecoder(strings.NewReader(string(plist)))
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			if cd, ok := tok.(xml.CharData); ok && strings.Contains(string(cd), "license refresh") {
				script = string(cd)
			}
		}
		if script == "" || !shSyntaxOK(script) || !strings.Contains(script, shQuote(bin)) {
			t.Fatalf("plist script does not round-trip: %q", script)
		}
	})
}

// TestCronSelfDeleteKeepsForeignLines runs the real cron line through sh (cron's \% unescape
// applied) with the project gone, against a stub crontab, and checks foreign lines before and
// after the timer line survive byte for byte while the timer line disappears.
func TestCronSelfDeleteKeepsForeignLines(t *testing.T) {
	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	store, stubs := filepath.Join(tmp, "store"), filepath.Join(tmp, "stubs")
	if err := os.MkdirAll(stubs, 0o755); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\nif [ \"$1\" = -l ]; then cat " + store + "; else cat > " + store + "; fi\n"
	if err := os.WriteFile(filepath.Join(stubs, "crontab"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(tmp, "nself")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(tmp, "gone-project")
	line, err := renderCron(bin, gone, Hash(gone))
	if err != nil {
		t.Fatal(err)
	}
	before := "MAILTO=ops\n0 3 * * * printf 'a\\n%s' b\n"
	after := "5 4 * * * echo 'x\\ny'\n"
	if err := os.WriteFile(store, []byte(before+line+"\n"+after), 0o644); err != nil {
		t.Fatal(err)
	}
	fields := strings.SplitN(strings.ReplaceAll(line, `\%`, "%"), " ", 6)
	run := exec.Command("sh", "-c", fields[5])
	run.Env = []string{"PATH=" + stubs + ":/usr/bin:/bin"}
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("cron line failed: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(store)
	if strings.TrimRight(string(got), "\n") != strings.TrimRight(before+after, "\n") {
		t.Fatalf("foreign lines changed:\n got %q\nwant %q", got, before+after)
	}
}
