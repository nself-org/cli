package verify

// Purpose: ask the ci plugin whether the release evidence is acceptable and
//          map its answer to E497/E498/E499.
// Inputs:  an EvidenceRunner (the mounted `ci evidence verify`), the evidence
//          file and the manifest revision.
// Outputs: nil, or a coded error.
// Constraints: this package never judges the evidence itself. The plugin's
//              exit status decides current/bound/passing (Epic D11); only the
//              report's class, result and quarantined fields are read here,
//              because the manifest contract names them.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

// subprocessWaitDelay bounds how long Run waits for a killed child's pipes.
const subprocessWaitDelay = 5 * time.Second

const (
	maxEvidenceOutput = 4 << 20
	evidenceSeconds   = 300
)

// EvidenceRunner runs `ci evidence verify --file <file> --at-revision <rev>
// --json` and returns its stdout and exit status. err is non-nil only when the
// plugin could not be started at all.
type EvidenceRunner func(ctx context.Context, file, revision string) (stdout []byte, exit int, err error)

type evidenceReport struct {
	Data struct {
		Class       string `json:"class"`
		Result      string `json:"result"`
		Quarantined *bool  `json:"quarantined"` // nil: absent, which must not read as "not quarantined"
	} `json:"data"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func checkEvidence(ctx context.Context, run EvidenceRunner, file, revision string) error {
	if run == nil {
		return errs.New("E499", "the ci plugin is not installed, so release evidence cannot be verified")
	}
	out, exit, err := run(ctx, file, revision)
	if err != nil {
		return errs.Wrap("E499", "the ci plugin could not be run to verify release evidence", err)
	}
	var rep evidenceReport
	parsed := json.Unmarshal(out, &rep) == nil
	if parsed && rep.Data.Quarantined != nil && *rep.Data.Quarantined {
		return errs.New("E498", "the ci plugin reports this release as quarantined")
	}
	if exit != 0 {
		msg := fmt.Sprintf("ci evidence verify refused the release evidence (exit %d)", exit)
		if parsed && rep.Error.Message != "" {
			msg += ": " + clip(rep.Error.Message)
		}
		return errs.New("E497", msg)
	}
	if !parsed || rep.Data.Quarantined == nil {
		return errs.New("E497", "ci evidence verify printed no readable report with a quarantine verdict")
	}
	if rep.Data.Class != "release" || rep.Data.Result != "pass" {
		return errs.Newf("E497", "release evidence is class %q result %q; a passing release-class document is required", clip(rep.Data.Class), clip(rep.Data.Result))
	}
	return nil
}

// ExecEvidence returns the production EvidenceRunner: it runs the ci plugin
// binary (a mount.Spec.BinaryPath) with the evidence verify argv, in repoRoot.
// Like the mount proxy it re-checks, right before the exec, that the binary
// still resolves to a regular executable file inside pluginDir.
func ExecEvidence(binary, pluginDir, repoRoot string) EvidenceRunner {
	return func(ctx context.Context, file, revision string) ([]byte, int, error) {
		if err := insidePluginDir(binary, pluginDir); err != nil {
			return nil, 0, err
		}
		ctx, cancel := context.WithTimeout(ctx, evidenceSeconds*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "evidence", "verify", "--file", file, "--at-revision", revision, "--json") //nolint:gosec // binary comes from plugin mount discovery; arguments are fixed
		cmd.Dir = repoRoot
		cmd.Env = os.Environ()
		var stdout bytes.Buffer
		cmd.Stdout = &limitWriter{w: &stdout, left: maxEvidenceOutput}
		cmd.WaitDelay = subprocessWaitDelay
		runErr := cmd.Run()
		if runErr == nil {
			return stdout.Bytes(), 0, nil
		}
		if ee, ok := runErr.(*exec.ExitError); ok {
			return stdout.Bytes(), ee.ExitCode(), nil
		}
		return nil, 0, runErr
	}
}

// limitWriter drops output beyond its budget so a hostile plugin cannot exhaust memory.
type limitWriter struct {
	w    *bytes.Buffer
	left int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	n := len(p)
	p = p[:min(n, l.left)]
	l.left -= len(p)
	_, _ = l.w.Write(p)
	return n, nil
}

// insidePluginDir proves binary is a regular executable file whose resolved
// path lies under pluginDir.
func insidePluginDir(binary, pluginDir string) error {
	root, err := filepath.EvalSymlinks(pluginDir)
	if err != nil {
		return fmt.Errorf("plugin directory is unavailable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(binary)
	if err != nil {
		return fmt.Errorf("ci plugin binary is unavailable: %w", err)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("ci plugin binary resolves outside the plugins directory")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0) {
		return fmt.Errorf("ci plugin binary is not an executable file")
	}
	return nil
}
