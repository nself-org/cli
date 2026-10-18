//go:build darwin || linux

package maintenance

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
)

// Purpose: install, check and remove the entitlement refresh timer on POSIX hosts (systemd as root,
// crontab as non-root Linux, LaunchAgent on macOS). Constraints: path-safety rule R7 before any write.

var safePathRe = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)

// host carries every outside-world dependency so tests can fake it.
type host struct {
	goos, home, unitDir, launchDir, exe string
	euid                                int
	systemd, crontab                    bool
	run                                 func(stdin, name string, args ...string) (string, error)
	trustAbove                          string // tests: ancestors above this path are not checked
}

func systemHost() host {
	_, cronErr := exec.LookPath("crontab")
	_, sdErr := os.Stat("/run/systemd/system")
	home, _ := os.UserHomeDir()
	exe, _ := os.Executable()
	return host{goos: runtime.GOOS, home: home, exe: exe, unitDir: entSystemdDir, euid: os.Geteuid(),
		launchDir: filepath.Join(home, "Library", "LaunchAgents"), systemd: sdErr == nil, crontab: cronErr == nil, run: runCmd}
}

var currentHost = systemHost

// Ensure installs or updates the timer for projectDir; it changes the host only on a difference.
func Ensure(projectDir, binPath string) (Result, error) {
	return currentHost().ensure(projectDir, binPath)
}

// Remove removes the timer for projectDir from every backend this process may touch.
func Remove(projectDir string) (RemoveResult, error) {
	return currentHost().remove(Hash(canonical(projectDir)))
}

// RemoveAll removes every entitlement timer of this user (root: the systemd ones too).
func RemoveAll() (RemoveResult, error) { return currentHost().remove("") }

// Status reports whether an active timer exists for projectDir and, if not, why.
func Status(projectDir string) TimerStatus { return currentHost().status(canonical(projectDir)) }

func (h host) backend() string {
	switch {
	case h.goos == "darwin":
		return BackendLaunchAgent
	case h.euid == 0 && h.systemd:
		return BackendSystemd
	case h.crontab:
		return BackendCron
	}
	return ""
}

// safePath applies the R7 rule and returns the resolved path and its owner uid.
func (h host) safePath(p string, owner func(uid uint32) bool) (string, uint32, error) {
	bad := func(why string) (string, uint32, error) { return "", 0, fmt.Errorf("%w: %s %q", ErrUnsafePath, why, p) }
	if !safePathRe.MatchString(p) || filepath.Clean(p) != p {
		return bad("not an absolute plain path")
	}
	res, err := filepath.EvalSymlinks(p)
	if err != nil || !safePathRe.MatchString(res) || filepath.Clean(res) != res {
		return bad("does not resolve to a plain path")
	}
	var uid uint32
	for cur := res; ; cur = filepath.Dir(cur) {
		info, err := os.Stat(cur)
		if err != nil || info.Mode().Perm()&0o022 != 0 {
			return bad("group- or world-writable (or unreadable) path component " + cur)
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || (cur == res && !owner(st.Uid)) || (cur != res && st.Uid != 0 && !owner(st.Uid)) {
			return bad("unexpected owner of " + cur) // an owner could swap what is below it
		}
		if cur == res {
			uid = st.Uid
		}
		if cur == "/" || cur == h.trustAbove {
			return res, uid, nil
		}
	}
}

func (h host) paths(projectDir, binPath string) (dir, bin string, uid uint32, err error) {
	dir, uid, err = h.safePath(projectDir, func(u uint32) bool { return h.euid == 0 || int(u) == h.euid })
	if err != nil {
		return
	}
	bin = binPath // the unit keeps the stable (possibly symlinked) path; its target is what is validated
	res, _, err := h.safePath(binPath, func(u uint32) bool { return u == 0 || u == uid })
	if info, serr := os.Stat(res); err == nil && (serr != nil || !info.Mode().IsRegular()) {
		err = fmt.Errorf("%w: binary %q is not a regular file", ErrUnsafePath, binPath)
	}
	return dir, bin, uid, err
}

func (h host) ensure(projectDir, binPath string) (Result, error) {
	if AutorefreshDisabled() {
		return Result{Disabled: true}, nil
	}
	dir, bin, uid, err := h.paths(projectDir, binPath)
	if err != nil {
		return Result{}, err
	}
	hash, backend := Hash(dir), h.backend()
	res := Result{Backend: backend}
	switch backend {
	case BackendSystemd:
		res.Changed, err = h.installSystemd(bin, dir, hash, uid)
	case BackendCron:
		res.Changed, err = h.installCron(bin, dir, hash)
	case BackendLaunchAgent:
		res.Changed, err = h.installLaunchAgent(bin, dir, hash)
	default:
		err = ErrNoBackend
	}
	return res, err
}

func (h host) status(dir string) TimerStatus {
	hash, backend := Hash(dir), h.backend()
	if h.goos == "linux" && fileExists(filepath.Join(h.unitDir, entUnitName(hash)+".timer")) {
		backend = BackendSystemd // a root-installed unit is visible to (and checked for) non-root users
	}
	if backend == "" {
		return TimerStatus{Reason: ReasonNoBackend}
	}
	active := false
	switch backend {
	case BackendSystemd:
		_, err := h.run("", "systemctl", "is-active", "--quiet", entUnitName(hash)+".timer")
		active = err == nil && fileExists(filepath.Join(h.unitDir, entUnitName(hash)+".timer"))
	case BackendCron:
		out, _ := h.cronList()
		active = strings.Contains(out, entCronTag+hash)
	case BackendLaunchAgent:
		_, err := h.run("", "launchctl", "print", launchTarget(h.euid, hash))
		active = err == nil && fileExists(filepath.Join(h.launchDir, launchAgentLabel(hash)+".plist"))
	}
	if active {
		return TimerStatus{Active: true, Backend: backend}
	}
	if _, _, _, err := h.paths(dir, h.exe); err != nil {
		return TimerStatus{Backend: backend, Reason: ReasonUnsafePath}
	}
	return TimerStatus{Backend: backend, Reason: ReasonNotInstalled}
}

func (h host) installSystemd(bin, dir, hash string, uid uint32) (bool, error) {
	svc, tmr, err := renderSystemd(bin, dir, hash, uid)
	if err != nil {
		return false, err
	}
	u := entUnitName(hash)
	c1, err := writeIfDiff(filepath.Join(h.unitDir, u+".service"), []byte(svc))
	if err != nil {
		return false, err
	}
	c2, err := writeIfDiff(filepath.Join(h.unitDir, u+".timer"), []byte(tmr))
	if err != nil {
		return false, err
	}
	if c1 || c2 {
		if out, err := h.run("", "systemctl", "daemon-reload"); err != nil {
			return true, cmdErr(out, err, "systemctl", "daemon-reload")
		}
	}
	if _, err := h.run("", "systemctl", "is-active", "--quiet", u+".timer"); err == nil && !c1 && !c2 {
		return false, nil
	}
	if out, err := h.run("", "systemctl", "enable", "--now", u+".timer"); err != nil {
		return true, cmdErr(out, err, "systemctl", "enable", "--now", u+".timer")
	}
	return true, nil
}

func (h host) installCron(bin, dir, hash string) (bool, error) {
	line, err := renderCron(bin, dir, hash)
	if err != nil {
		return false, err
	}
	cur, err := h.cronList()
	if err != nil {
		return false, err // never overwrite a crontab we could not read
	}
	rest, tagged := taggedLines(cur, entCronTag+hash)
	if len(tagged) == 1 && tagged[0] == line {
		return false, nil
	}
	out, err := h.run(strings.Join(append(rest, line), "\n")+"\n", "crontab", "-")
	if err != nil {
		return false, cmdErr(out, err, "crontab", "-")
	}
	return true, nil
}

// cronList reads the user's crontab. "No crontab" is an empty crontab; any other failure is an
// error, so a crontab we could not read is never overwritten. macOS never uses cron here.
func (h host) cronList() (string, error) {
	if h.goos != "linux" {
		return "", ErrNoBackend
	}
	out, err := h.run("", "crontab", "-l")
	if err != nil && !strings.Contains(out, "no crontab for") {
		return "", cmdErr(out, err, "crontab", "-l")
	}
	if err != nil {
		return "", nil
	}
	return out, nil
}

func (h host) installLaunchAgent(bin, dir, hash string) (bool, error) {
	data, err := renderPlist(bin, dir, hash)
	if err != nil {
		return false, err
	}
	path := filepath.Join(h.launchDir, launchAgentLabel(hash)+".plist")
	changed, err := writeIfDiff(path, data)
	if err != nil {
		return false, err
	}
	if _, perr := h.run("", "launchctl", "print", launchTarget(h.euid, hash)); perr == nil && !changed {
		return false, nil
	}
	_, _ = h.run("", "launchctl", "bootout", launchTarget(h.euid, hash))
	if out, err := h.run("", "launchctl", "bootstrap", fmt.Sprintf("gui/%d", h.euid), path); err != nil {
		return true, cmdErr(out, err, "launchctl", "bootstrap")
	}
	return true, nil
}

// remove drops the timer for hash ("" = all) from every backend this process can reach.
func (h host) remove(hash string) (r RemoveResult, firstErr error) {
	fail := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	id := func(path, prefix, suffix string) string {
		return strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), prefix), suffix)
	}
	var units, plists []string
	if h.goos == "linux" {
		units, _ = filepath.Glob(filepath.Join(h.unitDir, entUnitPrefix+"*.timer"))
	} else {
		plists, _ = filepath.Glob(filepath.Join(h.launchDir, entLabelPrefix+"*.plist"))
	}
	for _, t := range units {
		if i := id(t, entUnitPrefix, ".timer"); hash != "" && i != hash {
			continue
		} else if h.euid != 0 {
			r.RootRequired = true
		} else {
			_, _ = h.run("", "systemctl", "disable", "--now", entUnitName(i)+".timer")
			_ = os.Remove(t)
			fail(os.Remove(filepath.Join(h.unitDir, entUnitName(i)+".service")))
			_, _ = h.run("", "systemctl", "daemon-reload")
			r.Removed = true
		}
	}
	for _, p := range plists {
		if i := id(p, entLabelPrefix, ".plist"); hash == "" || i == hash {
			_, _ = h.run("", "launchctl", "bootout", launchTarget(h.euid, i))
			fail(os.Remove(p))
			r.Removed = true
		}
	}
	if cur, err := h.cronList(); err == nil {
		if rest, tagged := taggedLines(cur, entCronTag+hash); len(tagged) > 0 {
			out, err := h.run(strings.Join(rest, "\n")+"\n", "crontab", "-")
			if err != nil {
				fail(cmdErr(out, err, "crontab", "-"))
			} else {
				r.Removed = true
			}
		}
	}
	return r, firstErr
}

func cmdErr(out string, err error, name string, args ...string) error {
	return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(out))
}
