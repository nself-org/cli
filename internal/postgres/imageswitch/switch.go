package imageswitch

// switch.go — the Switch value and its small helpers.
//
// Purpose: hold the config, the effects and the live state so the step files
// (preflight, quiesce, dump, restore, verify, recheck, switchover, rollback,
// abort, run) stay small.
// Inputs: Config and Deps. Outputs: a *Switch. Constraints: one Switch per command run.

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

// onWindows is true on native Windows, where the switch is not supported: its
// stage directory is a POSIX bind mount owned by the host uid:gid. A variable so
// tests on every platform exercise the same path.
var onWindows = runtime.GOOS == "windows"

// Switch runs one image switch.
type Switch struct {
	cfg   Config
	d     Deps
	st    *State
	stage string
}

// New returns a Switch with defaults filled in.
func New(cfg Config, d Deps) *Switch {
	if d.Out == nil {
		d.Out = io.Discard
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.FreeBytes == nil {
		d.FreeBytes = freeBytes
	}
	if cfg.Service == "" {
		cfg.Service = "postgres"
	}
	return &Switch{cfg: cfg, d: d, stage: stagePath(cfg.ProjectDir)}
}

// say writes one progress line.
func (s *Switch) say(format string, a ...any) {
	_, _ = fmt.Fprintf(s.d.Out, "nself: image switch: "+format+"\n", a...)
}

// persist records the phase and writes the state.
func (s *Switch) persist(p Phase) error {
	if p != "" {
		s.st.Phase = p
	}
	return s.st.save(s.cfg.ProjectDir, s.d.Now())
}

var nameUnsafe = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

// oneShot is the name of a one-shot container of this run; a killed run leaves
// it behind and the next run removes it by this name.
func (s *Switch) oneShot(role string) string {
	return "nself-imgswitch-" + nameUnsafe.ReplaceAllString(s.cfg.ProjectName, "-") + "-" + role
}

// clearOneShots removes the one-shot containers a killed run may have left.
func (s *Switch) clearOneShots(ctx context.Context) {
	for _, role := range []string{"dump", "restore", "recheck", "probe"} {
		_ = s.d.Runtime.RemoveContainer(ctx, s.oneShot(role))
	}
}

// coded builds a coded error with why and fix text.
func coded(code, msg, why, fix string) error {
	e := errs.New(code, msg)
	if why != "" {
		e = e.WithWhy(why)
	}
	if fix != "" {
		e = e.WithFix(fix)
	}
	return e
}

// hostIDs returns the uid:gid the dump one-shots run as, so stage files belong to the host user.
func hostIDs() (uid, gid int) { return os.Getuid(), os.Getgid() }

// pgvectorImage reports whether an image reference is a pgvector image.
func pgvectorImage(ref string) bool {
	return strings.Contains(strings.ToLower(ref), "pgvector")
}
