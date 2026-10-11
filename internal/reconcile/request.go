package reconcile

// request.go — the input types of reconcile.Plan and reconcile.Apply
// (EPIC P7-LIVE D6).
//
// Purpose: one Request describes what a command is about to change; one
// ApplyOptions carries the operator's answers (--yes, --force, --plan-id and
// the interactive prompt). Both are plain data so cmd/ can fill them from flags
// and tests can fill them without a terminal.
// Constraints: nothing here reads the environment or a terminal.

import (
	"context"
	"io"

	nbuild "github.com/nself-org/cli/internal/build"
)

// Request describes one change.
type Request struct {
	// ProjectDir is the project root (the directory holding .env).
	ProjectDir string
	// Command and Trigger label the plan. Zero values mean `build` and a build
	// trigger with no subject.
	Command Command
	Trigger Trigger
	// Build is passed to the build pipeline. Plan forces Mode to ModePlan and
	// Apply to ModeWrite; every other field is used as given.
	Build nbuild.BuildOptions
	// Containers asks for container impact (the running stack compared with
	// what the next start would create).
	Containers bool
	// RemoveOrphans records an orphan-remove effect for every running container
	// of the project that the planned compose no longer defines.
	RemoveOrphans bool
	// Extra lists effects the caller knows about that the build pipeline does
	// not record (for example the plugin lifecycle step's expired-plugin
	// removals). They join the plan's effects as given.
	Extra []Effect
	// HandEdited reports whether a human changed a generated file since nself
	// last wrote it (P7-LIVE-04 supplies it). Nil means never.
	HandEdited func(path string) bool
	// Runtime answers the docker questions of container impact. Nil uses the
	// docker CLI through internal/docker.
	Runtime ContainerRuntime
	// Seed, when set, drives every secret the build generates (plan and write
	// use the same stream). Apply draws a random one when it is empty, so the
	// bytes it writes are exactly the bytes it planned.
	Seed []byte
	// DiffOut, when set, receives the unified diff of every changed artifact
	// (env-kind artifacts show key names only). Compute writes it; nothing else does.
	DiffOut io.Writer
	// Stderr receives notices (for example "container state unknown"). Nil
	// discards them.
	Stderr io.Writer
	// EnvOverlay holds env lines a command is about to write: project-relative
	// env file name (".env", ".env.secrets") -> key -> value (P7-LIVE-05). The
	// plan renders the project as if they were written (overlay.go
	// pendingEnv); nothing is written by Plan or Compute. Apply's write build
	// reads the disk, so the caller writes the lines in BeforeWrite.
	EnvOverlay map[string]map[string]string
	// EnvBefore is the project's ENV before the pending lines (empty: resolved
	// from the disk as config.Load would, when EnvOverlay is set). A prod-class EnvBefore makes a
	// non-empty plan need confirmation even when the lines change ENV to a
	// dev-class one (`config set ENV dev` on a prod box). ApplyBuild resolves it
	// once, before BeforeWrite, so its rechecks after the write agree.
	EnvBefore string
}

// ApplyOptions are the operator's answers to a plan.
type ApplyOptions struct {
	// Yes confirms a prod-class plan without a prompt.
	Yes bool
	// Force allows overwriting or removing a hand-edited generated file.
	Force bool
	// PlanID, when set, must equal the recomputed plan's id (E450 otherwise).
	PlanID string
	// BeforeWrite runs after the plan id and the confirmation have passed and
	// before the write-mode build; an error stops the apply. Nil does nothing.
	BeforeWrite func() error
	// Interactive asks the operator a yes/no question and reports the answer.
	// Nil means the session is not interactive: nothing can be asked.
	Interactive func(prompt string) bool
	// Rollback runs when the apply fails after BeforeWrite was called, still
	// under the project lock, so no other command sees the half-applied state
	// first; its error joins the apply error. Nil does nothing.
	Rollback func() error
}

// ContainerRuntime is the docker half of container impact. The default
// implementation shells out through internal/docker; tests inject a fake.
type ContainerRuntime interface {
	// Running lists the project's containers with their config-hash labels. An
	// error means the daemon could not be asked.
	Running(ctx context.Context, project string) ([]RunningContainer, error)
	// ConfigHashes returns service -> config hash for the given compose files,
	// env files and project directory.
	ConfigHashes(ctx context.Context, files, envFiles []string, projectDir string) (map[string]string, error)
}

// RunningContainer is one container of the project.
type RunningContainer struct {
	Name       string
	Service    string
	State      string
	ConfigHash string
}
