package commands

// Purpose: the scratch build of a v1.5 remote deploy (D24 step 2, review F26).
//          The project is copied to <workdir>/.nself/deploy-build/<env>/ (rebuilt
//          each deploy), a stub ssl tree mirroring the target's certificate dirs
//          is laid in it, and `nself build` runs there with the target cascade, so
//          generated cert paths are the target's and nothing is written into the
//          operator's own tree.
// Inputs:  workdir, env name, the target snapshot (cert dir names and presence).
// Outputs: the scratch dir with a finished build in it.
// Constraints: the copy leaves out .nself/, .git, node_modules, ssl/, the backup
//              and data dirs, logs and the previously generated
//              docker-compose.yml (so the build freshness check proves this build
//              wrote it) and the nginx/ files that carry the build's generated
//              marker (stale output; only files this build writes ship, see
//              deploy_scratch_nginx.go). A project with NGINX_FRONTED_BY builds
//              in <env>/<FrontedBy>/backend (deploy_scratch_fronted.go). The stub PEM files are never shipped: the artifact set
//              has no ssl entries. A symlink is copied only when it points inside
//              the project, so a build can never write through one into the
//              operator's tree.

import (
	"context"
	"fmt"
	"github.com/nself-org/cli/internal/deploy/pathmap"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"
	"golang.org/x/term"
)

// deployScratchBuildFn runs the build inside the scratch dir. A variable so
// tests can write the build output without running docker.
var deployScratchBuildFn = runDeployBuildStep

// scratchSkipTop are the top-level project entries the scratch copy leaves out:
// ssl/ (stubbed instead), the backup dir (config default BACKUP_DIR=./backups),
// the data dir (.volumes, see internal/setup), logs, and the generated compose.
var scratchSkipTop = map[string]bool{
	".nself": true, "ssl": true, "backups": true, ".volumes": true, "logs": true, "docker-compose.yml": true,
}

// scratchSkipAny are skipped at any depth.
var scratchSkipAny = map[string]bool{".git": true, "node_modules": true}

// deployBuildStepForMode is the default deploy build step: in v1.5 a remote
// target builds later, in the scratch copy after the target snapshot, so this
// step does nothing for it; everything else builds in place as before.
func deployBuildStepForMode(ctx context.Context, workdir string, remote bool, steps []deployStep) ([]deployStep, error) {
	// compat.V15(P7-DEPL-24): remote build in the operator's project dir -> scratch build after the target snapshot
	if compat.V15() && remote {
		return steps, nil
	}
	return runDeployBuildStep(ctx, workdir, remote, steps)
}

// scratchDirFor is the scratch dir of env.
func scratchDirFor(workdir, env string) string {
	return filepath.Join(workdir, ".nself", "deploy-build", env)
}

// runScratchBuild copies the project, stubs ssl from snap and builds. It
// returns the scratch dir.
func runScratchBuild(ctx context.Context, workdir, env string, snap deploy.TargetSnapshot) (string, error) {
	if env == "" || env != filepath.Base(env) || strings.HasPrefix(env, ".") {
		return "", fmt.Errorf("deploy: %q is not a valid environment name for a scratch directory", env)
	}
	policy, err := nginxPolicyFor(workdir, env) // a bad allowlist fails before the build
	if err != nil {
		return "", err
	}
	frontedBy, err := scratchFrontedBy(workdir, env)
	if err != nil {
		return "", err
	}
	scratch := scratchProjectDir(workdir, env, frontedBy)
	if err := clearScratch(workdir, env); err != nil {
		return "", err
	}
	if err := scratchCopyProject(workdir, scratch, policy); err != nil {
		return "", fmt.Errorf("deploy: copying the project to %s: %w", scratch, err)
	}
	if err := writeStubSSL(scratch, snap.SSL); err != nil {
		return "", fmt.Errorf("deploy: writing the stub ssl tree: %w", err)
	}
	if err := writeFrontStubSSL(scratch, frontedBy, snap.FrontSSL); err != nil {
		return "", fmt.Errorf("deploy: writing the stub ssl tree: %w", err)
	}
	restoreSites, err := preserveScratchNginxSites(scratch, policy)
	if err != nil {
		return "", fmt.Errorf("deploy: preserving allowlisted nginx sites: %w", err)
	}
	if _, err := deployScratchBuildFn(ctx, scratch, true, nil); err != nil {
		return "", fmt.Errorf("deploy: scratch build failed, nothing was sent: %w", err)
	}
	if err := restoreSites(); err != nil {
		return "", fmt.Errorf("deploy: restoring allowlisted nginx sites: %w", err)
	}
	return scratch, nil
}

// writeStubSSL lays ssl/certificates/<dir>/<file>.pem stubs mirroring the
// target's names and presence. The content is a marker, never key material.
func writeStubSSL(scratch string, certs []deploy.CertDir) error {
	return writeStubSSLDir(filepath.Join(scratch, "ssl"), certs)
}

// writeStubSSLDir is writeStubSSL for an ssl dir that is not <scratch>/ssl (the
// fronting stack's, see deploy_scratch_fronted.go).
func writeStubSSLDir(sslDir string, certs []deploy.CertDir) error {
	for _, c := range certs {
		dir := filepath.Join(sslDir, "certificates", c.Name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for name, present := range map[string]bool{"fullchain.pem": c.Fullchain, "privkey.pem": c.Privkey, "chain.pem": c.Chain} {
			if !present {
				continue
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte("STUB: written by nself deploy for the scratch build, never shipped\n"), 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

// stageScratch reads the build output of scratch and stages the artifact set.
func stageScratch(p v15Params, scratch string) (deploy.ArtifactSet, error) {
	files, err := build.ReadComposeManifest(scratch)
	if err != nil {
		return deploy.ArtifactSet{}, err
	}
	envs, err := build.ReadComposeEnvManifest(scratch)
	if err != nil {
		return deploy.ArtifactSet{}, err
	}
	if len(envs) == 0 {
		envs = build.ComposeEnvFiles(scratch)
	}
	cascade := deployCascadeEnv(p.workdir, p.target)
	envSnap, cleanup, err := writeResolvedDeployEnv(scratch, cascade)
	if err != nil {
		return deploy.ArtifactSet{}, fmt.Errorf("deploy: env snapshot failed, nothing was sent: %w", err)
	}
	defer cleanup()
	stage := filepath.Join(scratch, ".nself", "deploy-stage")
	if err := os.RemoveAll(stage); err != nil {
		return deploy.ArtifactSet{}, err
	}
	if err := os.MkdirAll(stage, 0o700); err != nil {
		return deploy.ArtifactSet{}, err
	}
	policy, err := nginxPolicyFor(p.workdir, p.target)
	if err != nil {
		return deploy.ArtifactSet{}, err
	}
	frontedBy, err := scratchFrontedBy(p.workdir, p.target)
	if err != nil {
		return deploy.ArtifactSet{}, err
	}
	in := deploy.StageInput{
		Map:      pathmap.PathMap{ScratchRoot: scratch, OperatorRoot: p.workdir, PluginDir: build.DefaultPluginDir(), Home: pathmap.DefaultHome(), RemotePath: p.srv.RemotePath},
		StageDir: stage, Compose: files, EnvFiles: envs, EnvSnapshot: envSnap, EnvName: cascade, Nginx: policy,
	}
	if frontedBy != "" {
		in.FrontedBy, in.FrontScratch = frontedBy, filepath.Dir(scratch)
	}
	set, err := deploy.Stage(in)
	if err != nil {
		return set, err
	}
	return finishScratchStage(deployOut(p.jsonOut), set, staleGeneratedNginx(p.workdir, scratch))
}

// localPluginRefs lists the plugins the build will read.
func localPluginRefs() ([]deploy.PluginRef, error) {
	dir := build.DefaultPluginDir()
	ms, err := plugin.LoadManifestsFromDir(dir)
	if err != nil {
		return nil, fmt.Errorf("deploy: reading the local plugins: %w", err)
	}
	var refs []deploy.PluginRef
	for _, m := range ms {
		refs = append(refs, deploy.PluginRef{Name: m.Name, Version: m.Version, Disabled: plugin.IsDisabled(m.Name, dir)})
	}
	return refs, nil
}

// hasuraStrictFor is the legacy strict rule: the target's default, overridden by
// NSELF_HASURA_METADATA_STRICT.
func hasuraStrictFor(workdir, target string) bool {
	if raw := os.Getenv("NSELF_HASURA_METADATA_STRICT"); raw != "" {
		return raw == "true"
	}
	return remoteHasuraStrict(workdir, target)
}

// deployConfirmPrompt is the prompt the apply uses; a variable so a test can observe the
// context it is given.
var deployConfirmPrompt = ttyConfirm

// ttyConfirm asks on a terminal; without one the answer is no. A cancelled ctx
// (Ctrl-C while the prompt waits) answers no at once, so the command aborts.
func ttyConfirm(ctx context.Context, msg string) bool {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stderr.Fd())) {
		return false
	}
	return confirmFrom(ctx, os.Stdin, os.Stderr, msg)
}

// confirmFrom prompts on out and reads one line from in in a goroutine, so a
// blocking read cannot swallow the interrupt.
func confirmFrom(ctx context.Context, in io.Reader, out io.Writer, msg string) bool {
	_, _ = fmt.Fprintf(out, "%s. Type yes to continue: ", msg)
	line := make(chan string, 1)
	go func() {
		var ans string
		_, _ = fmt.Fscanln(in, &ans)
		line <- ans
	}()
	select {
	case ans := <-line:
		return strings.EqualFold(strings.TrimSpace(ans), "yes")
	case <-ctx.Done():
		_, _ = fmt.Fprintln(out)
		return false
	}
}

// previousShipped is the union of the paths the target's last recorded deploy
// shipped or pruned, on every host: what a prune may remove. An unreadable
// record is reported and yields nothing, so the prune removes less, never more.
func previousShipped(workdir, target string) []string {
	st, err := deploy.LoadDeployState(workdir)
	if err != nil {
		ui.Warn(fmt.Sprintf("deploy: %v; pruning only what this deploy knows", err))
		return nil
	}
	var out []string
	if st.Target == target {
		for _, paths := range st.PreDeployChanged {
			out = append(out, paths...)
		}
	}
	return out
}

type deployTargetKey struct{}

// withDeployTarget carries the target's own name to the legacy push, whose argument
// is the cascade env (a prod-class env called "live" pushes as "prod").
func withDeployTarget(ctx context.Context, target string) context.Context {
	return context.WithValue(ctx, deployTargetKey{}, target)
}

func deployTargetFrom(ctx context.Context, fallback string) string {
	if t, _ := ctx.Value(deployTargetKey{}).(string); t != "" {
		return t
	}
	return fallback
}
