package build

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
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
}

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

// buildLockFile is the name of the file used to prevent concurrent builds.
const buildLockFile = ".nself/build.lock"

// acquireBuildLock creates an exclusive build lock file using O_EXCL so that
// two concurrent builds never write conflicting compose artifacts. The caller
// must defer releaseBuildLock.
func acquireBuildLock(workdir string) (*os.File, error) {
	lockPath := filepath.Join(workdir, buildLockFile)
	_ = os.MkdirAll(filepath.Dir(lockPath), 0755)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("another build is already running (lock file exists: %s). If no other build is running, remove the lock file and retry", lockPath)
		}
		return nil, fmt.Errorf("acquiring build lock: %w", err)
	}
	return f, nil
}

// releaseBuildLock closes and removes the lock file returned by acquireBuildLock.
func releaseBuildLock(f *os.File, workdir string) {
	_ = f.Close()
	_ = os.Remove(filepath.Join(workdir, buildLockFile))
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
	st := &buildState{workdir: workdir, opts: opts, start: time.Now()}

	// Acquire exclusive build lock to prevent concurrent builds from
	// producing inconsistent compose artifacts.
	buildLock, err := acquireBuildLock(workdir)
	if err != nil {
		return nil, err
	}
	defer releaseBuildLock(buildLock, workdir)

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
	return st.writeFinalArtifacts()
}

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

	filesGenerated int

	pluginDir      string
	missingPlugins []string

	sslResult *ssl.GenerateResult

	composePath        string
	secretMap          map[string]string
	pluginComposeFiles []string
}
