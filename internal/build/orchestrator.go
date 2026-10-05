package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/oplock"
	"github.com/nself-org/cli/internal/ssl"
)

// BuildOptions controls build behavior via CLI flags.
type BuildOptions struct {
	// Force rebuilds everything regardless of cache freshness.
	Force bool
	// Verbose enables detailed build progress output.
	Verbose bool
	// Check validates configuration and exits without generating files.
	Check bool
	// SecurityReport prints a detailed security audit after validation.
	SecurityReport bool
	// NoAutoRedis disables automatic Redis enablement when a BullMQ-backed
	// plugin (ai, claw, mux, cron, notify, push) is detected. Pass
	// --no-auto-redis from the CLI to opt out of this behaviour.
	NoAutoRedis bool
	// Profile selects a curated subset of services for the generated
	// docker-compose.yml.  Empty string or "app" preserves today's full
	// behaviour (no regression).  Use "ops" for an observability + CI server.
	// Valid values: "app" (default), "ops".  See internal/compose/profiles.go.
	Profile compose.ProfileName
	// Hosts explicitly opts a build into /etc/hosts management (--hosts)
	// for a BASE_DOMAIN that ssl.shouldManageHosts's local-dev heuristic
	// does not recognize. Never overrides the ENV=prod veto — see
	// internal/ssl/hosts_gate.go. Unset (false) preserves the normal local
	// dev experience: a recognized local-dev domain is still managed
	// automatically with no flag.
	Hosts bool
	// Mode selects write (default) or plan. In ModePlan the whole pipeline
	// renders in memory: nothing is written under the project, nothing is
	// created, and no host effect runs. BuildResult.Planned carries the
	// rendered artifacts and the recorded effects.
	Mode BuildMode
	// Rand, when non-nil, is the byte source for every secret the build
	// generates (JWT key, admin and plugin secrets, via config.UseRandSource).
	// Nil keeps crypto/rand. A seeded reader lets a plan and an apply of the
	// same fresh project agree byte for byte. The reader is never logged.
	Rand io.Reader
}

// BuildMode selects what Build does with the files it renders.
type BuildMode int

const (
	// ModeWrite writes every artifact and runs every host effect (default).
	ModeWrite BuildMode = iota
	// ModePlan renders in memory and records effects without acting.
	ModePlan
)

// BuildResult summarizes what the build produced.
type BuildResult struct {
	// ProjectName is the sanitized project name from config.
	ProjectName string
	// ComposeFile is the path to the generated docker-compose.yml.
	ComposeFile string
	// NginxConfig is the path to the generated nginx/nginx.conf.
	NginxConfig string
	// SSLCerts is the number of SSL certificate sets generated.
	SSLCerts int
	// Duration is the wall-clock time the build took.
	Duration time.Duration
	// FilesGenerated is the total number of files written.
	FilesGenerated int
	// PluginComposeFiles lists the absolute paths to plugin compose files
	// discovered during build. Empty when no plugins with compose files
	// are installed.
	PluginComposeFiles []string
	// MissingPlugins lists plugins declared in nself.yaml that could not be
	// wired into the generated stack (not installed, not auto-installable,
	// and not satisfied by a core service). Non-empty means the generated
	// stack does NOT match the declared manifest.
	MissingPlugins []string
	// CAInstalled is true when the mkcert CA is trusted by the OS.
	CAInstalled bool
	// CAManualCmd is non-empty when the user must manually trust the CA.
	CAManualCmd string
	// HostsAdded is the number of new /etc/hosts entries written.
	HostsAdded int
	// HostsManualNote is non-empty when /etc/hosts could not be updated automatically.
	HostsManualNote string
	// Env is the project's ENV as loaded (not normalised).
	Env string
	// FrontingDir is the fronting stack's served nginx/sites directory when
	// the plan has "@fronting/" keys; empty otherwise. Set in ModePlan only.
	FrontingDir string
	// Planned holds the rendered artifacts and recorded effects. Set only in
	// ModePlan; nil after a write-mode build.
	Planned *PlannedBuild
}

// requiredDirs lists the directories that must exist before generation.
// Created relative to the project workdir.
var requiredDirs = []string{
	"nginx",
	"nginx/conf.d",
	"nginx/includes",
	"nginx/sites",
	"ssl",
	"ssl/certificates",
	"postgres",
	"monitoring",
	"services",
	".nself",
}

// Seams for the v1.4 wait.
var (
	buildLockClock             = oplock.RealClock()
	buildLockV14Wait           = 30 * time.Second
	buildLockStderr  io.Writer = os.Stderr
)

// buildLockCommand is the command name recorded in the lock holder file.
const buildLockCommand = "nself build"

// AcquireBuildLock takes the project operation lock (internal/oplock), a flock
// on .nself/op.lock the kernel drops when the holder dies, so a SIGKILLed build
// leaves nothing stale. A descendant of the holder (NSELF_OPLOCK_TOKEN)
// re-enters it. A build that owns the lock removes the file while still holding
// it, as the old build.lock was removed, so a direct Build leaves the same tree.
// Held: v1.5 fails naming the holder (errors.Is oplock.ErrHeld); v1.4 waits up to
// 30 s, warns, and builds unlocked. The caller must call release.
func AcquireBuildLock(ctx context.Context, workdir string) (release func(), err error) {
	opts := oplock.Opts{Command: buildLockCommand, Clock: buildLockClock}
	// compat.V15(P7-LIVE-03): a held lock is waited for up to 30 s, then a warning and the build runs unlocked -> a held lock fails the build at once, naming the holder
	v15 := compat.V15()
	if !v15 {
		opts.Wait = buildLockV14Wait
		opts.OnWait = func(h oplock.Holder, known bool) {
			_, _ = fmt.Fprintf(buildLockStderr, "nself: waiting up to %s for %s\n", buildLockV14Wait, (&oplock.HeldError{Holder: h, Known: known}).Error())
		}
	}
	l, err := oplock.Acquire(ctx, workdir, opts)
	var held *oplock.HeldError
	switch {
	case errors.Is(err, oplock.ErrUnsupported):
		return func() {}, nil // no flock on this platform: run unlocked, as the guard does
	case errors.As(err, &held) && !v15:
		_, _ = fmt.Fprintf(buildLockStderr, "nself: warning: %s after %s; building without the lock\n", held.Error(), held.Waited.Round(time.Second))
		return func() {}, nil
	case err != nil:
		return nil, fmt.Errorf("another nself operation is changing this project: %w", err)
	}
	return func() {
		if !l.Reentrant() {
			_ = os.Remove(oplock.LockPath(workdir))
		}
		l.Release()
	}, nil
}

// Build orchestrates the full nself build pipeline.
//
// The sequence follows BUILD_SPEC.md:
//  1. Load config via env cascade
//  2. Validate config (security, passwords, ports, CORS)
//  3. If --check: return after validation
//  4. Check cache (skip rebuild if not --force and cache fresh)
//  5. Create required directories
//  6. Generate SSL certificates
//  7. Generate nginx configuration files
//  8. Generate docker-compose.yml
//  9. Write docker-compose.yml with 0600 permissions
//  10. Write .env.computed (DATABASE_URL + DOCKER_NETWORK)
//  11. Save build version to .nself/build-version
//  12. Return BuildResult with summary
func Build(workdir string, opts BuildOptions) (*BuildResult, error) {
	// Scope the secret source for this build only; restored on return.
	defer config.UseRandSource(opts.Rand)()
	st := newBuildState(workdir, opts)

	// Acquire exclusive build lock to prevent concurrent builds from
	// producing inconsistent compose artifacts. Plan mode records the lock as
	// an effect instead of creating .nself/op.lock.
	var releaseLock func()
	if err := st.fx.Do(EffectBuildLock, filepath.Join(workdir, ".nself", "op.lock"), "exclusive project operation lock", func() (err error) {
		releaseLock, err = AcquireBuildLock(context.Background(), workdir)
		return err
	}); err != nil {
		return nil, err
	}
	if releaseLock != nil {
		defer releaseLock()
	}

	// Steps 1-4 (load config, persist secrets, permissions, validate,
	// nginx conflict check, --check/cache early exits) — extracted to
	// orchestrator_build_config.go (T-P6-E2-W1-S1-T3). A non-nil
	// *BuildResult here means one of the early-exit paths (--check or a
	// cache-fresh skip) was taken.
	if early, err := st.loadValidateConfig(); err != nil {
		return nil, err
	} else if early != nil {
		return early, nil
	}

	// Steps 5-7.6 (dirs, SSL, nginx gen+write, plugin resolve, plugin nginx
	// routes, postgres init script, np_plugins seed, redis auto-enable) —
	// extracted to orchestrator_build_ssl.go (T-P6-E2-W1-S1-T3).
	if err := st.generateSSLAndNginx(); err != nil {
		return nil, err
	}

	// Steps 8-9.8 (compose gen, ollama merge, secret templating, write
	// compose + manifest, plugin Dockerfile healthcheck warnings, ɳSentry/
	// Loki wiring, hasura config refresh) — extracted to
	// orchestrator_build_compose.go (T-P6-E2-W1-S1-T3).
	if err := st.generateCompose(); err != nil {
		return nil, err
	}

	// Steps 10-12 (.env.computed, compose.env, build-version, OpenAPI spec,
	// post-build validation, final BuildResult assembly) — extracted to
	// orchestrator_build_finish.go (T-P6-E2-W1-S1-T3).
	res, err := st.writeFinalArtifacts()
	if err != nil {
		return nil, err
	}
	if st.cfg != nil {
		res.Env = st.cfg.Env
	}
	if m, ok := st.sink.(*memSink); ok {
		res.Planned = m.snapshot()
		res.Planned.Effects = st.fx.Recorded()
		res.FrontingDir = m.fronting
	}
	return res, nil
}

// newBuildState wires the Sink and Effects for the requested mode.
func newBuildState(workdir string, opts BuildOptions) *buildState {
	st := &buildState{workdir: workdir, opts: opts, start: time.Now()}
	st.ensureSeam()
	return st
}

// ensureSeam picks the Sink and Effects from opts.Mode when unset: plan mode
// gets the in-memory sink and the recording effects, write mode the disk sink
// and the acting effects (today's behaviour). Every phase calls it first, so a
// buildState built field by field (tests) works too.
func (st *buildState) ensureSeam() {
	if st.sink != nil && st.fx != nil {
		return
	}
	if st.opts.Mode == ModePlan {
		st.sink, st.fx = newMemSink(st.workdir, ""), &planEffects{}
	} else {
		st.sink, st.fx = newDiskSink(st.workdir), writeEffects{}
	}
}

// planning reports whether this build renders in memory.
func (st *buildState) planning() bool { return st.opts.Mode == ModePlan }

// buildState carries the values threaded through Build()'s phases —
// resolved config, accumulated file count, and the intermediate artifacts
// (plugin dir, SSL result, compose path/YAML, secret map, plugin compose
// files) each later phase and the final BuildResult need. Introduced when
// Build() was split across orchestrator_build_{config,ssl,compose,finish}.go
// for 300-line compliance (T-P6-E2-W1-S1-T3); every field here was
// previously a local variable inside the single Build() function.
type buildState struct {
	workdir string
	opts    BuildOptions
	start   time.Time
	cfg     *config.Config

	// sink receives every file the pipeline writes; fx performs or records
	// every host effect. Both are chosen by newBuildState from opts.Mode.
	sink Sink
	fx   Effects

	filesGenerated int

	pluginDir      string
	missingPlugins []string

	sslResult *ssl.GenerateResult

	composePath        string
	secretMap          map[string]string
	pluginComposeFiles []string
}
