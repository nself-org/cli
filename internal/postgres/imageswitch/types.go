// Package imageswitch moves an existing non-pgvector Postgres cluster to the
// pgvector image of the same major version without touching the old volume
// (EPIC P7-ADOPT D6, P7-ADOPT-22).
//
// Order (nothing is switched before every check has passed):
//
//	preflight -> quiesce (services stopped, databases read-only) -> dump (one
//	exported snapshot per database) -> restore into a NEW volume inside a
//	one-shot server (no initdb.d, no network) -> verify the restore -> re-check
//	the old cluster has not changed since the dump -> switch env, compose and
//	lock -> start.
//
// Purpose: `nself db image switch --to pgvector`. The command layer
// (cmd/commands/db_image*.go) supplies the three seams below; this package never
// imports reconcile or cobra.
// Inputs: Config and Deps. Outputs: a Report; coded errors E509-E512.
// Constraints: the old data volume is never removed, renamed or mounted
// read-write by this package; the database password reaches a container only as
// PGPASSWORD through docker.RunSpec.EnvPass; state and reports hold no secret.
package imageswitch

import (
	"context"
	"io"
	"time"

	"github.com/nself-org/cli/internal/docker"
)

// Phase is how far a switch has got. It is written to state.json after every step.
type Phase string

const (
	// PhaseStarted: preflight passed and the state exists; nothing was stopped.
	PhaseStarted Phase = "started"
	// PhaseQuiesced: services stopped, databases read-only, other sessions gone.
	PhaseQuiesced Phase = "quiesced"
	// PhaseDumped: the dump and the old-cluster capture are in the stage.
	PhaseDumped Phase = "dumped"
	// PhaseRestored: the new volume holds the restore and was captured.
	PhaseRestored Phase = "restored"
	// PhaseVerified: the restore equals the dump-time capture.
	PhaseVerified Phase = "verified"
	// PhaseRechecked: the old cluster still equals the dump-time capture.
	PhaseRechecked Phase = "rechecked"
	// PhaseSwitching: postgres is being stopped and env, compose and lock written.
	PhaseSwitching Phase = "switching"
	// PhaseSwitched: env, compose and lock name the new volume; the stack is not yet started.
	PhaseSwitched Phase = "switched"
	// PhaseDone: the stack runs on the new volume.
	PhaseDone Phase = "done"
	// PhaseRolledBack: the project is back on the old volume.
	PhaseRolledBack Phase = "rolled-back"
)

// Complete reports whether the switch needs no further action.
func (p Phase) Complete() bool { return p == PhaseDone || p == PhaseRolledBack }

// Env keys the switch writes; the first two are also what rollback restores.
const (
	EnvImage      = "POSTGRES_IMAGE"
	EnvDataVolume = "POSTGRES_DATA_VOLUME"
)

// Config describes the project the switch runs in.
type Config struct {
	ProjectDir  string
	ProjectName string
	// Service is the compose service name of postgres ("postgres").
	Service string
	// Container is the postgres container name (<project>_postgres).
	Container string
	// User is POSTGRES_USER; Password is POSTGRES_PASSWORD (EnvPass only).
	User     string
	Password string
	// TargetFor returns the pgvector image of the given Postgres major, or an
	// error when there is none.
	TargetFor func(major int) (string, error)
	// PrevEnv holds the current raw values of EnvImage and EnvDataVolume in the
	// project (empty when unset); rollback writes them back.
	PrevEnv map[string]string
}

// Deps are the effects the switch makes. Tests replace every one.
type Deps struct {
	Runtime Runtime
	Stack   Stack
	Applier Applier
	// Out receives one progress line per step; nil discards.
	Out io.Writer
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
	// FreeBytes reports the free bytes under a host path; nil uses statfs.
	FreeBytes func(path string) (uint64, error)
}

// Runtime is docker for the switch: inspect, one-shots, volumes.
type Runtime interface {
	// Inspect returns the container, or an error when it does not exist.
	Inspect(ctx context.Context, name string) (ContainerInfo, error)
	// ExecSQL runs sql in the running container with psql over the local socket.
	ExecSQL(ctx context.Context, container, user, db, sql string) (string, error)
	// RunOneShot runs a throwaway container to completion.
	RunOneShot(ctx context.Context, spec docker.RunSpec) (stdout, stderr string, err error)
	// RemoveContainer force-removes a container; absent is success.
	RemoveContainer(ctx context.Context, name string) error
	// VolumeLabels returns a volume's labels; exists is false when it is absent.
	VolumeLabels(ctx context.Context, name string) (labels map[string]string, exists bool, err error)
	// VolumeCreate, VolumeExists and VolumeRemove manage named volumes.
	VolumeCreate(ctx context.Context, name string, labels map[string]string) error
	VolumeExists(ctx context.Context, name string) (bool, error)
	VolumeRemove(ctx context.Context, name string) error
}

// ContainerInfo is what the switch reads from `docker inspect`.
type ContainerInfo struct {
	Image          string
	Running        bool
	DataVolume     string // name of the volume mounted at /var/lib/postgresql/data
	Networks       []string
	ComposeProject string // label com.docker.compose.project
}

// Stack is the compose lifecycle of the project.
type Stack interface {
	// Running lists the service names that are running now.
	Running(ctx context.Context) ([]string, error)
	// Stop stops the services; Up starts them (none: every service).
	Stop(ctx context.Context, services ...string) error
	Up(ctx context.Context, services ...string) error
	// WaitHealthy returns when the service reports healthy, or an error on timeout.
	WaitHealthy(ctx context.Context, service string) error
}

// Applier writes env lines and regenerates compose and lock through the build
// plan path (reconcile). A value of "" means the key is set empty (unset).
type Applier interface {
	// Plan returns what Apply would change, as JSON-ready data; nothing is written.
	Plan(ctx context.Context, env map[string]string) (any, error)
	// Apply writes the lines, regenerates, and undoes the lines if it fails.
	Apply(ctx context.Context, env map[string]string) error
}

// Check is one preflight result.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Report is the command's data (`--json` envelope data).
type Report struct {
	// Status: planned, switched, already-pgvector, rolled-back.
	Status    string `json:"status"`
	Phase     Phase  `json:"phase,omitempty"`
	OldImage  string `json:"old_image,omitempty"`
	NewImage  string `json:"new_image,omitempty"`
	OldVolume string `json:"old_volume,omitempty"`
	NewVolume string `json:"new_volume,omitempty"`
	// Preflight lists every check; a failed one stops the switch with E509.
	Preflight []Check `json:"preflight,omitempty"`
	// Steps names what a run does, in order.
	Steps []string `json:"steps,omitempty"`
	// Stage is the directory the dump is written to (mode 0700); Dumps names the dump
	// files in it, one per database. Both are for --plan; the stage is removed at the end.
	Stage     string   `json:"stage,omitempty"`
	Dumps     []string `json:"dumps,omitempty"`
	Databases int      `json:"databases,omitempty"`
	Tables    int      `json:"tables,omitempty"`
	Sequences int      `json:"sequences,omitempty"`
	DataBytes int64    `json:"data_bytes,omitempty"`
	// DowntimeSeconds estimates the window from stopping services to a healthy stack.
	DowntimeSeconds int `json:"downtime_seconds,omitempty"`
	// Tolerated lists the bootstrap statements the restore skipped, each logged.
	Tolerated []string `json:"tolerated,omitempty"`
	// Build is the compose and lock change the build plan path would make.
	Build     any      `json:"build,omitempty"`
	NextSteps []string `json:"next_steps,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

// RunOptions are the flags of a run.
type RunOptions struct {
	// Resume continues an incomplete switch from its state.
	Resume bool
}
