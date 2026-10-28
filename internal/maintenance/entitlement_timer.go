package maintenance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Purpose: keep a paid online stack entitled with no operator command (P7-PLUG-64): one host timer
// per project runs `nself license refresh --project DIR --quiet`. Constraints: names derive only from
// Hash(dir); paths are single-quoted in shell text; no key, JWT or `sub` in any unit, line or plist.

// ExitGone is the exit status of a refresh run that found the timer's reason gone; the
// root systemd ExecStopPost removes the units when it sees it (SuccessExitStatus=ExitGone).
const ExitGone = 78

const (
	entUnitPrefix  = "nself-entitlement-"
	entLabelPrefix = "org.nself.entitlement."
	entCronTag     = "# nself-entitlement:"
	entSystemdDir  = "/etc/systemd/system"
)

// Backend names reported by Result and TimerStatus.
const (
	BackendSystemd     = "systemd"
	BackendCron        = "cron"
	BackendLaunchAgent = "launchagent"
)

// Reasons Status gives when no timer is active (doctor E132 text).
const (
	ReasonNotInstalled = "not-installed"
	ReasonUnsafePath   = "unsafe-path"
	ReasonNoBackend    = "no-backend"
)

var (
	// ErrUnsafePath: the project dir or binary path failed the path-safety rule (R7).
	ErrUnsafePath = errors.New("unsafe path for an unattended timer")
	// ErrNoBackend: neither systemd (root), crontab (non-root Linux) nor launchd is usable.
	ErrNoBackend = errors.New("no timer backend is available on this host")
)

// Result describes one Ensure call.
type Result struct {
	Backend  string
	Changed  bool // true when a unit, crontab line or plist was written
	Disabled bool // true when NSELF_ENTITLEMENT_AUTOREFRESH=false: nothing installed
}

// RemoveResult describes one Remove call.
type RemoveResult struct {
	Removed      bool // at least one timer artifact was removed
	RootRequired bool // systemd units exist but this process is not root
}

// TimerStatus describes the timer for one project directory.
type TimerStatus struct {
	Active  bool
	Backend string
	Reason  string // empty when Active
}

// AutorefreshDisabled reports NSELF_ENTITLEMENT_AUTOREFRESH=false (also 0, no, off).
func AutorefreshDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("NSELF_ENTITLEMENT_AUTOREFRESH"))) {
	case "false", "0", "no", "off":
		return true
	}
	return false
}

// Hash is the first 16 hex characters of sha256(project dir): the only source of names.
func Hash(projectDir string) string {
	sum := sha256.Sum256([]byte(projectDir))
	return hex.EncodeToString(sum[:])[:16]
}

// shQuote single-quotes s for sh.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// systemdEscape escapes the characters systemd expands inside a quoted word.
func systemdEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`, "%", "%%", "$", "$$").Replace(s)
}

// cronEscape escapes `%`, which cron turns into a newline.
func cronEscape(s string) string { return strings.ReplaceAll(s, "%", `\%`) }

// refreshScript guards presence, not executability: an upgrade can temporarily
// clear executable bits. Only a missing binary or project retires the timer.
func refreshScript(bin, dir string, gone string) string {
	return fmt.Sprintf("test -e %s && test -d %s || %s", shQuote(bin), shQuote(dir), gone)
}

// refreshCmd pins v1.5 mode: the timer is only installed in it, and its environment is empty.
func refreshCmd(bin, dir string) string {
	return fmt.Sprintf("env NSELF_V15=1 %s license refresh --project %s --quiet", shQuote(bin), shQuote(dir))
}

func checkRenderable(bin, dir string) error {
	if strings.ContainsAny(bin+dir, "\x00\n\r") || len(bin)+len(dir) > 300 { // cron truncates a line near 1000 bytes
		return fmt.Errorf("%w: control character or over-long path", ErrUnsafePath)
	}
	return nil
}

func entUnitName(hash string) string { return entUnitPrefix + hash }

// renderSystemd returns the service and timer units for the system backend.
func renderSystemd(bin, dir, hash string, uid uint32) (service, timer string, err error) {
	if err = checkRenderable(bin, dir); err != nil {
		return "", "", err
	}
	script := refreshScript(bin, dir, fmt.Sprintf("exit %d", ExitGone)) + "; exec " + refreshCmd(bin, dir)
	u := entUnitName(hash)
	service = fmt.Sprintf(`[Unit]
Description=nSelf entitlement refresh %s

[Service]
Type=oneshot
User=%d
SuccessExitStatus=%d
ExecStart=/bin/sh -c '%s'
ExecStopPost=+/bin/sh -c '%s'
`, hash, uid, ExitGone, systemdEscape(script), systemdEscape(rootStep(hash)))
	timer = fmt.Sprintf(`[Unit]
Description=nSelf entitlement refresh timer %s

[Timer]
OnBootSec=15min
OnUnitActiveSec=6h
RandomizedDelaySec=30m
Persistent=true
Unit=%s.service

[Install]
WantedBy=timers.target
`, hash, u)
	return service, timer, nil
}

// rootStep is the only root-run text: fixed paths from HASH, never a project path.
func rootStep(hash string) string {
	u := entSystemdDir + "/" + entUnitName(hash)
	return fmt.Sprintf(`if test "$EXIT_STATUS" = %d; then systemctl disable --now %s.timer; rm -f %s.service %s.timer; systemctl daemon-reload; fi`,
		ExitGone, entUnitName(hash), u, u)
}

// renderCron returns one crontab line (minute jitter from HASH, every 6 h).
func renderCron(bin, dir, hash string) (string, error) {
	if err := checkRenderable(bin, dir); err != nil {
		return "", err
	}
minute, _ := strconv.ParseUint(hash[:2], 16, 8)
tag := entCronTag + hash
	// Guard flock: if unavailable, fall back to unlocked removal so the job does not run forever.
	drop := fmt.Sprintf(`{ mkdir -p "$HOME/.nself" && t=$(crontab -l 2>/dev/null) && printf '%%s\n' "$t" | grep -vF %s | { if command -v flock >/dev/null 2>&1; then flock -x 9 crontab - 9>"$HOME/.nself/entitlement-cron.lock"; else crontab -; fi; }; }`, shQuote(tag))
line := fmt.Sprintf("%d */6 * * * if test -e %s && test -d %s; then %s >/dev/null; else %s; fi %s",
minute%60, shQuote(bin), shQuote(dir), refreshCmd(bin, dir), drop, tag)
return cronEscape(line), nil
}

// launchAgentLabel is the launchd label for a project.
func launchAgentLabel(hash string) string { return entLabelPrefix + hash }

// renderPlist builds the LaunchAgent plist with encoding/xml (ProgramArguments array).
func renderPlist(bin, dir, hash string) ([]byte, error) {
	if err := checkRenderable(bin, dir); err != nil {
		return nil, err
	}
	label := launchAgentLabel(hash)
	script := fmt.Sprintf(`if test -e %s && test -d %s; then exec %s; else rm -f "$HOME/Library/LaunchAgents/%s.plist"; launchctl bootout "gui/$(id -u)/%s"; fi`,
		shQuote(bin), shQuote(dir), refreshCmd(bin, dir), label, label)
	var sb strings.Builder
	sb.WriteString(xml.Header)
	sb.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	enc := xml.NewEncoder(&sb)
	enc.Indent("", "  ")
	el := func(name, text string) {
		_ = enc.EncodeElement(text, xml.StartElement{Name: xml.Name{Local: name}})
	}
	start := func(name string, attrs ...xml.Attr) {
		_ = enc.EncodeToken(xml.StartElement{Name: xml.Name{Local: name}, Attr: attrs})
	}
	end := func(name string) { _ = enc.EncodeToken(xml.EndElement{Name: xml.Name{Local: name}}) }
	start("plist", xml.Attr{Name: xml.Name{Local: "version"}, Value: "1.0"})
	start("dict")
	el("key", "Label")
	el("string", label)
	el("key", "ProgramArguments")
	start("array")
	for _, a := range []string{"/bin/sh", "-c", script} {
		el("string", a)
	}
	end("array")
	el("key", "StartInterval")
	el("integer", "21600")
	end("dict")
	end("plist")
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	return []byte(sb.String() + "\n"), nil
}

func runCmd(stdin, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func canonical(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		return r
	}
	return filepath.Clean(dir)
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// writeIfDiff writes data atomically unless the file already holds it.
func writeIfDiff(path string, data []byte) (bool, error) {
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".nself-entitlement-*")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Chmod(0o644)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return false, err
	}
	return true, os.Rename(tmp.Name(), path)
}

// taggedLines splits crontab text into the lines that carry tag and the rest.
func taggedLines(crontab, tag string) (rest, tagged []string) {
	for _, l := range strings.Split(strings.TrimRight(crontab, "\n"), "\n") {
		switch {
		case l == "":
		case strings.Contains(l, tag):
			tagged = append(tagged, l)
		default:
			rest = append(rest, l)
		}
	}
	return
}

// renderAll returns the rendered systemd units, crontab line and plist, for the dry run.
func renderAll(bin, dir string, uid uint32) (string, error) {
	hash := Hash(dir)
	svc, tmr, err := renderSystemd(bin, dir, hash, uid)
	if err != nil {
		return "", err
	}
	cron, err := renderCron(bin, dir, hash)
	if err != nil {
		return "", err
	}
	plist, err := renderPlist(bin, dir, hash)
	return fmt.Sprintf("# %s.service\n%s\n# %s.timer\n%s\n# crontab\n%s\n\n# LaunchAgent plist\n%s", entUnitName(hash), svc, entUnitName(hash), tmr, cron, plist), err
}

// Preview renders every backend's text for the hidden dry run; it validates and writes nothing.
func Preview(projectDir, binPath string) (string, error) {
	return renderAll(binPath, canonical(projectDir), uint32(os.Getuid()))
}

func launchTarget(uid int, hash string) string {
	return fmt.Sprintf("gui/%d/%s", uid, launchAgentLabel(hash))
}
