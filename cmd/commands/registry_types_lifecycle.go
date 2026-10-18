// JSON data-type registration and result assembly for the lifecycle domain
// (init, start, stop, restart).
//
// Purpose: `nself init|start|stop|restart --json` answer with the v1 envelope
// whose data is one of the four result types below (P7-SURF-10). Each command
// reports once, at the end of its run; progress and the human text stay on
// stderr (the invocation decorator isolates stdout in JSON mode). One data type
// per command (REG D1). `--json` was refused with E402 before, so the envelope
// is additive in both compat modes, not v1.5-only.
//
// `start --watch` is a stream by the fragment's flag override: it ends with the
// same result as the last line of a json-stream (output.StreamEnd), as `logs
// --follow` does. `start dev` is a stream row with json none, so dev.go has no
// envelope and `--json` is refused there with E402, like every json-none stream.
//
// Result assembly sits with its command: init here, start in start_cmd.go, the
// stop and restart service lists in restart.go.
// Inputs: command results (setup result, health report, compose services).
// Outputs: values placed in the envelope `data`; tools/schemagen reads the
// registration and emits schemas/commands/<cmd>.v1.schema.json.
// Constraints: registers from init(); see registry_types.go for the rules.

package commands

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	clonetemplate "github.com/nself-org/cli/internal/templates/clone"
	"github.com/spf13/cobra"
)

// InitResult is the data of `nself init --json`.
type InitResult struct {
	// ProjectDir is the absolute directory init scaffolded into.
	ProjectDir string `json:"project_dir"`
	// Env is the ENV value written to .env; empty when init wrote no .env
	// (--list-presets, --template).
	Env string `json:"env"`
	// FilesWritten lists the files init created or replaced, relative to
	// ProjectDir with forward slashes; empty on --dry-run and --list-presets.
	FilesWritten []string `json:"files_written"`
	// Next lists the commands init suggests running next, in order.
	Next []string `json:"next"`
}

// StartService is one service of a started stack.
type StartService struct {
	Name string `json:"name"`
	// Status is the container state: running, missing or unknown.
	Status string `json:"status"`
	// Health is the Docker healthcheck result: healthy, unhealthy, starting
	// or none (the service declares no healthcheck).
	Health string `json:"health"`
}

// StartResult is the data of `nself start --json` (the last line of a stream
// under --watch).
type StartResult struct {
	Services []StartService `json:"services"`
	// URLs lists the endpoints the stack answers on, without the display labels.
	URLs       []string `json:"urls"`
	DurationMS int64    `json:"duration_ms"`
}

// StopResult is the data of `nself stop --json`.
type StopResult struct {
	// Stopped names the services that were running and are now stopped; empty
	// when none was running or the running set could not be read.
	Stopped []string `json:"stopped"`
	// VolumesRemoved is true when --volumes deleted the data volumes.
	VolumesRemoved bool `json:"volumes_removed"`
}

// RestartResult is the data of `nself restart --json`.
type RestartResult struct {
	// Restarted names the services that were restarted.
	Restarted []string `json:"restarted"`
	// Mode is the strategy that ran: all, services, smart or quick.
	Mode string `json:"mode"`
}

func init() {
	registerJSONType("init", InitResult{})
	registerJSONType("start", StartResult{})
	registerJSONType("stop", StopResult{})
	registerJSONType("restart", RestartResult{})
}

// emitLifecycle writes the result of command when JSON mode is on. Under
// `start --watch` (a stream by the fragment's flag override) the result is the
// last line of the stream instead of a document.
func emitLifecycle(cmd *cobra.Command, command string, data any) error {
	if !lifecycleJSON(cmd) {
		return nil
	}
	if lifecycleStream(cmd) {
		return output.StreamEnd(pilotWriter().Out, command, data, nil)
	}
	return output.EmitData(pilotWriter(), command, data)
}

// daemonUnreachable reports whether err is the Docker daemon not answering: the
// canonical sentinel, the daemon client's own message in compose's stderr tail,
// or a failed compose run while `docker info` itself fails (the stderr tail of a
// compose run is not always kept). Any other failure is not Docker being
// unavailable: a compose error with a live daemon, or an error that never ran
// compose, keeps its own code.
func daemonUnreachable(cmd *cobra.Command, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errs.ErrDockerNotRunning) || strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
		return true
	}
	return strings.Contains(err.Error(), "docker compose") && errors.Is(checkDockerAvailable(commandContext(cmd)), errs.ErrDockerNotRunning)
}

// lifecycleFailure returns err unchanged, except for a verified daemon-unreachable
// failure of a v1.5 JSON run: that writes the E002 envelope once, renders the
// original error text on stderr as the human run does, and returns a silent
// infra exit so the shared reporter prints neither a second time. Human runs and
// v1.4 keep the original error.
func lifecycleFailure(cmd *cobra.Command, command string, err error) error {
	// compat.V15(P7-SURF-10): daemon-unreachable stop/restart failure, unclassified -> E002 envelope with infra exit 2
	if err == nil || !compat.V15() || !lifecycleJSON(cmd) || !daemonUnreachable(cmd, err) {
		return err
	}
	w := pilotWriter()
	if werr := output.EmitError(w, command, fmt.Errorf("%w: %s", errs.ErrDockerNotRunning, err)); werr != nil {
		return werr
	}
	output.RenderError(w.Err, err)
	return errs.Exit(errs.ExitInfraError)
}

// lifecycleJSON reports whether the invocation asked for JSON output. A caller
// whose result costs work to assemble checks it first.
func lifecycleJSON(cmd *cobra.Command) bool {
	on, _ := cmd.Flags().GetBool("json")
	return on
}

// lifecycleStream reports whether this invocation is a json-stream: JSON mode
// with `start --watch`.
func lifecycleStream(cmd *cobra.Command) bool {
	watch, _ := cmd.Flags().GetBool("watch")
	return lifecycleJSON(cmd) && watch && cmd.Name() == "start"
}

// initStep is one suggested next command of `nself init` and why.
type initStep struct{ cmd, why string }

// initNextSteps are the suggested commands after a project init, in order. The
// human "Next steps" list and InitResult.Next both read it, so they cannot drift.
func initNextSteps(demo bool) []initStep {
	steps := []initStep{
		{"nself doctor", "Verify environment (Docker, ports, config)"},
		{"nself start", "Launch your backend stack"},
		{"nself status", "Live health dashboard"},
		{"nself plugin list", "Browse available plugins"},
	}
	if demo {
		steps = append(steps, initStep{"nself admin start", "Open the admin panel (demo mode)"})
	}
	return steps
}

// initScaffoldResult assembles the InitResult of the normal init flow from the
// setup result. AGENTS.md is written by init's hook, not by setup, so the caller
// says whether it wrote one (agentsMDWrittenSince).
func initScaffoldResult(dir, env string, created []string, demo bool, wroteAgents bool) InitResult {
	files := append([]string{}, created...)
	if wroteAgents {
		files = append(files, "AGENTS.md")
	}
	var next []string
	for _, s := range initNextSteps(demo) {
		next = append(next, s.cmd)
	}
	return InitResult{ProjectDir: dir, Env: env, FilesWritten: files, Next: next}
}

// templateNext lists the commands that bring a scaffolded template up. A clone
// template reports its manifest's next_steps (manifestNextSteps), the same list
// printTemplateNextSteps prints.
func templateNext(template string) []string {
	if !clonetemplate.IsCloneTemplate(template) {
		return []string{"nself start"}
	}
	m, err := clonetemplate.GetManifest(template)
	if err != nil {
		return []string{"nself start"}
	}
	return manifestNextSteps(m)
}

// fileStamp identifies one file version in a tree snapshot.
type fileStamp struct {
	size    int64
	modNano int64
}

// snapshotTree records every regular file under dir (a missing dir is empty).
// VCS and dependency directories are skipped: no template writes into them.
func snapshotTree(dir string) map[string]fileStamp {
	snap := map[string]fileStamp{}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); n == ".git" || n == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if info, ierr := d.Info(); ierr == nil && info.Mode().IsRegular() {
			if rel, rerr := filepath.Rel(dir, path); rerr == nil {
				snap[filepath.ToSlash(rel)] = fileStamp{info.Size(), info.ModTime().UnixNano()}
			}
		}
		return nil
	})
	return snap
}

// changedFiles lists, sorted, the files of dir that are new or changed since
// before was taken by snapshotTree(dir).
func changedFiles(dir string, before map[string]fileStamp) []string {
	files := []string{}
	for rel, st := range snapshotTree(dir) {
		if old, ok := before[rel]; !ok || old != st {
			files = append(files, rel)
		}
	}
	sort.Strings(files)
	return files
}

// absOrSelf returns the absolute form of dir, or dir itself when it has none.
func absOrSelf(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// recordTemplateInit runs one `init --template` scaffold into its destination
// (the first argument, else the current directory) and records where it went,
// which files it created or replaced and the commands that bring it up.
func recordTemplateInit(res *InitResult, template string, args []string, run func(destDir string) error) error {
	destDir := "."
	if len(args) > 0 {
		destDir = args[0]
	}
	res.ProjectDir = absOrSelf(destDir)
	before := snapshotTree(res.ProjectDir)
	if err := run(destDir); err != nil {
		return err
	}
	res.FilesWritten = changedFiles(res.ProjectDir, before)
	res.Next = templateNext(template)
	return nil
}

// agentsMDWrittenSince reports whether init's hook wrote AGENTS.md in dir at or
// after began (it is absent from setup's file list).
func agentsMDWrittenSince(dir string, began int64) bool {
	info, err := os.Stat(filepath.Join(dir, "AGENTS.md"))
	return err == nil && info.ModTime().UnixNano() >= began
}
