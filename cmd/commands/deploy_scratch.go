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
//              wrote it). The stub PEM files are never shipped: the artifact set
//              has no ssl entries. A symlink is copied only when it points inside
//              the project, so a build can never write through one into the
//              operator's tree.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/plugin"
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
	scratch := scratchDirFor(workdir, env)
	if err := os.RemoveAll(scratch); err != nil {
		return "", fmt.Errorf("deploy: clearing %s: %w", scratch, err)
	}
	if err := scratchCopyProject(workdir, scratch); err != nil {
		return "", fmt.Errorf("deploy: copying the project to %s: %w", scratch, err)
	}
	if err := writeStubSSL(scratch, snap.SSL); err != nil {
		return "", fmt.Errorf("deploy: writing the stub ssl tree: %w", err)
	}
	if _, err := deployScratchBuildFn(ctx, scratch, true, nil); err != nil {
		return "", fmt.Errorf("deploy: scratch build failed, nothing was sent: %w", err)
	}
	return scratch, nil
}

// copyProject copies src to dst without the entries scratchSkipTop and
// scratchSkipAny name and without deploy env snapshots.
func scratchCopyProject(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		top := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		skip := scratchSkipTop[top] || scratchSkipAny[info.Name()] || strings.HasPrefix(info.Name(), ".nself-deploy-env-")
		if skip {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		out := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(out, 0o755)
		case info.Mode()&os.ModeSymlink != 0:
			return scratchCopyLink(src, p, out)
		case info.Mode().IsRegular():
			return scratchCopyFile(p, out, info)
		}
		return nil
	})
}

// copyLink recreates a symlink only when it is relative and stays inside src.
func scratchCopyLink(src, link, out string) error {
	target, err := os.Readlink(link)
	if err != nil || filepath.IsAbs(target) {
		return nil
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(link), target))
	if resolved != src && !strings.HasPrefix(resolved, src+string(filepath.Separator)) {
		return nil
	}
	return os.Symlink(target, out)
}

func scratchCopyFile(from, to string, info os.FileInfo) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(to, info.ModTime(), info.ModTime())
}

// writeStubSSL lays ssl/certificates/<dir>/<file>.pem stubs mirroring the
// target's names and presence. The content is a marker, never key material.
func writeStubSSL(scratch string, certs []deploy.CertDir) error {
	for _, c := range certs {
		dir := filepath.Join(scratch, "ssl", "certificates", c.Name)
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
	return deploy.Stage(deploy.StageInput{
		Map:      deploy.PathMap{ScratchRoot: scratch, OperatorRoot: p.workdir, PluginDir: build.DefaultPluginDir(), Home: deploy.DefaultHome(), RemotePath: p.srv.RemotePath},
		StageDir: stage, Compose: files, EnvFiles: envs, EnvSnapshot: envSnap, EnvName: cascade,
	})
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

// ttyConfirm asks on a terminal; without one the answer is no.
func ttyConfirm(msg string) bool {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stderr.Fd())) {
		return false
	}
	_, _ = fmt.Fprintf(os.Stderr, "%s. Type yes to continue: ", msg)
	var ans string
	_, _ = fmt.Scanln(&ans)
	return strings.EqualFold(strings.TrimSpace(ans), "yes")
}
