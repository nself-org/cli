package commands

// Purpose: bind the pure promotion engine to inventory, scratch-build,
//          remote-apply, snapshot, migration, health, and local record seams.
// Inputs: resolved project root, source/target names and command flags.
// Outputs: a canonical Engine and initialized State.
// Constraints: dry-run returns before remote open, preflight, or scratch build;
//              the operator project tree is never populated with target values.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	remotesdk "github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/spf13/cobra"
)

type promoteWiring struct {
	workdir, source, target string
	inv                     *controlplane.Inventory
	sourceServer            controlplane.Server
	targetServer            controlplane.Server
	sourceRemote            deploy.Remote
	targetRemote            deploy.Remote
	apply                   *controlplane.RemoteApply
	compose                 string
	started                 time.Time
	yes                     bool
}

// preparePromoteEngine resolves local inventory facts and wires concrete steps.
func preparePromoteEngine(cmd *cobra.Command, workdir, source, target string, dryRun, jsonOut bool) (cpromote.Engine, *cpromote.State, error) {
	inv, err := controlplane.Load(workdir)
	if err != nil {
		return cpromote.Engine{}, nil, err
	}
	sourceName, ok := inventoryEnvName(inv, source)
	if !ok {
		return cpromote.Engine{}, nil, errs.Newf("E483", "unknown promotion source %q", source)
	}
	targetName, ok := inventoryEnvName(inv, target)
	if !ok {
		return cpromote.Engine{}, nil, errs.Newf("E483", "unknown promotion target %q", target)
	}
	source, target = sourceName, targetName
	sourceEnv, targetEnv := inv.Environments[source], inv.Environments[target]
	st := &cpromote.State{Source: source, Target: target, SourceTier: sourceEnv.Tier, TargetTier: targetEnv.Tier, DryRun: dryRun, Values: map[string]any{}}
	if err := cpromote.ValidateTiers(source, target, st.SourceTier, st.TargetTier); err != nil {
		return cpromote.Engine{}, nil, err
	}
	w := &promoteWiring{workdir: workdir, source: source, target: target, inv: inv, started: time.Now().UTC()}
	st.Values["wiring"] = w
	w.sourceServer, ok = primaryServer(inv, source)
	if !ok && sourceEnv.Kind == "local" {
		w.sourceServer = controlplane.Server{Name: "local", RemotePath: workdir, Primary: true}
	} else if !ok {
		return cpromote.Engine{}, nil, errs.Newf("E486", "promotion source %q has no server", source)
	}
	w.targetServer, ok = primaryServer(inv, target)
	if !ok {
		return cpromote.Engine{}, nil, errs.Newf("E486", "promotion target %q has no server", target)
	}
	eng := cpromote.Engine{Steps: promoteSteps(w, jsonOut)}
	if dryRun {
		return eng, st, nil
	}
	w.yes, _ = cmd.Flags().GetBool("yes")
	w.sourceRemote, err = promoteRemote(cmd.Context(), w.sourceServer, source, workdir)
	if err != nil {
		return cpromote.Engine{}, nil, err
	}
	w.targetRemote, err = promoteRemote(cmd.Context(), w.targetServer, target, workdir)
	if err == nil {
		eng.Steps = promoteSteps(w, jsonOut)
	}
	return eng, st, err
}

func promoteSteps(w *promoteWiring, jsonOut bool) []cpromote.Step {
	step := func(name string, run func(context.Context, *cpromote.State) error) cpromote.Step {
		return cpromote.Step{Name: name, Run: run}
	}
	steps := []cpromote.Step{
		step("resolve", func(_ context.Context, st *cpromote.State) error {
			if err := cpromote.ValidateTiers(st.Source, st.Target, st.SourceTier, st.TargetTier); err != nil {
				return err
			}
			if w.inv.Environments[st.Source].Kind != "local" {
				if _, err := controlplane.ResolveTargets(w.inv, controlplane.Selector{Env: st.Source}); err != nil {
					return err
				}
			}
			_, err := controlplane.ResolveTargets(w.inv, controlplane.Selector{Env: st.Target})
			return err
		}),
		step("confirm", func(ctx context.Context, _ *cpromote.State) error {
			if prodClassFn(w.inv, w.target) && !w.yes && !deployConfirmPrompt(ctx, fmt.Sprintf("Promote %s to production target %s", w.source, w.target)) {
				return errs.New("E403", "promotion to a production-class target requires --yes or terminal confirmation")
			}
			return nil
		}),
		{Name: "verify"},
		step("preflight", func(ctx context.Context, _ *cpromote.State) error {
			return controlplane.Preflight(ctx, w.inv, w.target, []string{filepath.Join(w.workdir, "docker-compose.yml")})
		}),
		step("source-health", func(ctx context.Context, st *cpromote.State) error {
			return cpromote.SourceHealthStep(w.sourceRemote, promotePath(w.sourceServer, w.workdir)).Run(ctx, st)
		}),
		step("capture", runPromoteCapture(w)),
		step("build", func(ctx context.Context, _ *cpromote.State) error {
			ra, compose, err := prepareRemoteApply(ctx, v15Params{workdir: w.workdir, target: w.target, srv: w.targetServer, keyPath: os.Getenv(w.targetServer.SSHKeyRef), prodClass: false, yes: true, jsonOut: jsonOut})
			w.apply, w.compose = ra, compose
			return err
		}),
		step("compare", runPromoteCompare(w)),
	}
	var suffix [4]byte
	_, _ = rand.Read(suffix[:])
	tag := fmt.Sprintf("pre-promote-%d-%s", time.Now().Unix(), hex.EncodeToString(suffix[:]))
	steps = append(steps, cpromote.SnapshotStep(w.targetRemote, promotePath(w.targetServer, w.workdir), tag))
	steps = append(steps, cpromote.SaveStep(w.targetRemote, promotePath(w.targetServer, w.workdir)))
	apply := step("apply", runPromoteApply(w))
	apply.Undo = func(ctx context.Context, st *cpromote.State) error { return rollbackPromoteApply(ctx, w, st) }
	steps = append(steps, apply, cpromote.MigrateStep(w.targetRemote, promotePath(w.targetServer, w.workdir)), cpromote.HealthStep(w.targetRemote, promotePath(w.targetServer, w.workdir)))
	steps = append(steps, step("record", runPromoteRecord(w)))
	return steps
}
func promotePath(s controlplane.Server, local string) string {
	if s.Host == "" {
		return local
	}
	return s.RemotePath
}

type localPromoteRemote struct{ root string }

func (r localPromoteRemote) Label() string { return "local" }
func (r localPromoteRemote) Run(ctx context.Context, command string) ([]byte, error) {
	c := exec.CommandContext(ctx, "sh", "-c", command) //nolint:gosec // command is built from validated paths and fixed fragments
	c.Dir = r.root
	return c.Output()
}
func (localPromoteRemote) Rsync(context.Context, []string, string, string, string) error {
	return fmt.Errorf("local promotion does not use rsync")
}

func promoteRemote(_ context.Context, srv controlplane.Server, env, workdir string) (deploy.Remote, error) {
	if srv.Host == "" {
		return localPromoteRemote{root: workdir}, nil
	}
	return &lazyPromoteRemote{srv: srv, env: env, key: os.Getenv(srv.SSHKeyRef)}, nil
}

func runPromoteApply(w *promoteWiring) func(context.Context, *cpromote.State) error {
	return func(ctx context.Context, st *cpromote.State) error {
		if w.apply == nil {
			return fmt.Errorf("promotion build did not produce an artifact set")
		}
		remotes := map[string]deploy.Remote{}
		paths := map[string]string{}
		w.apply.Yes = w.yes
		w.apply.ProdClass, w.apply.BackupID, w.apply.BackupHost = prodClassFn(w.inv, w.target), st.BackupID, st.BackupHost
		w.apply.Open = func(c context.Context, srv controlplane.Server, key, env string) (deploy.Remote, error) {
			r, err := openDeployRemote(c, srv, key, env)
			if err == nil {
				remotes[r.Label()], paths[r.Label()] = r, srv.RemotePath
			}
			return r, err
		}
		originalRecord := w.apply.Record
		w.apply.Record = func(srv controlplane.Server, res deploy.ApplyResult, err error) {
			if originalRecord != nil {
				originalRecord(srv, res, err)
			}
			cpromote.RecordApplied(st, res)
		}
		st.Values["apply_remotes"], st.Values["apply_paths"] = remotes, paths
		result, err := controlplane.RunWithApply(ctx, w.inv, w.target, newDeployProber(w.workdir), w.compose, w.apply)
		if err == nil {
			err = failedServersError(result)
			for _, server := range result.Servers {
				if server.Err != nil {
					err = errors.Join(err, server.Err)
				}
			}
		}
		return err
	}
}

func rollbackPromoteApply(ctx context.Context, w *promoteWiring, st *cpromote.State) error {
	remotes, _ := st.Values["apply_remotes"].(map[string]deploy.Remote)
	paths, _ := st.Values["apply_paths"].(map[string]string)
	return cpromote.UndoApplied(ctx, st, remotes, paths)
}

func runPromoteRecord(w *promoteWiring) func(context.Context, *cpromote.State) error {
	return func(_ context.Context, st *cpromote.State) error {
		return savePromoteRecord(w, st, "ok")
	}
}

func savePromoteRecord(w *promoteWiring, st *cpromote.State, status string) error {
	saved, _ := st.Values["saved_manifest"].(cpromote.SavedManifest)
	return cpromote.SaveRecord(w.workdir, cpromote.Record{
		ID: deploy.NewBackupID(w.started), Source: st.Source, Target: st.Target,
		StartedAt: w.started, CompletedAt: time.Now().UTC(), Status: status, SnapshotTag: st.SnapshotTag,
		BackupID: st.BackupID, BackupHost: st.BackupHost, PreDeployTar: st.PreDeployTar,
		Changed: st.PreDeployChanged, Absent: st.PreDeployAbsent, Digests: st.Digests, SavedManifest: saved,
		DBRestoredAt: st.DBRestoredAt, MigrateStarted: st.MigrateStarted,
	})
}
func runPromoteCapture(w *promoteWiring) func(context.Context, *cpromote.State) error {
	return func(ctx context.Context, st *cpromote.State) error {
		rp := promotePath(w.sourceServer, w.workdir)
		run := func(c context.Context, command string) ([]byte, error) {
			return w.sourceRemote.Run(c, "cd "+remotesdk.ShellQuote(rp)+" && "+command)
		}
		capturedRun, raw, err := cpromote.CaptureSourceManifest(ctx, run, rp)
		st.Values["source_runner"], st.Values["source_raw"] = capturedRun, raw
		return err
	}
}

func runPromoteCompare(w *promoteWiring) func(context.Context, *cpromote.State) error {
	return func(ctx context.Context, st *cpromote.State) error {
		var images []cpromote.Image
		var err error
		if w.apply != nil {
			images, err = cpromote.ResolvedArtifactImages(ctx, localPromoteRemote{root: w.apply.Set.StageDir}.resolveModel, w.targetRemote.Run, w.apply.Set)
		} else {
			files, fileErr := controlplane.ComposeFilesForDeploy(w.compose)
			if fileErr != nil {
				return fileErr
			}
			root := filepath.Dir(w.compose)
			envs, envErr := build.ReadComposeEnvManifest(root)
			if envErr != nil {
				return envErr
			}
			if len(envs) == 0 {
				envs = build.ComposeEnvFiles(root)
			}
			for i, file := range envs {
				if !filepath.IsAbs(file) {
					envs[i] = filepath.Join(root, file)
				}
			}
			images, err = cpromote.ResolvedComposeImages(ctx, localPromoteRemote{root: root}.resolveModel, files, envs)
		}
		if err != nil {
			return err
		}
		arch, err := cpromote.TargetArchitecture(ctx, w.targetServer.Arch, func(c context.Context, command string) ([]byte, error) {
			return w.targetRemote.Run(c, command)
		})
		if err != nil {
			return err
		}
		source, _ := st.Values["source_images"].([]cpromote.SourceImage)
		if run, ok := st.Values["source_runner"].(cpromote.Runner); ok {
			raw, _ := st.Values["source_raw"].([]byte)
			source, err = cpromote.CaptureManifestSource(ctx, run, raw, nil)
			if err != nil {
				return err
			}
		}
		rows, err := cpromote.CompareDigests(ctx, source, images, arch, st.TargetTier, nil, func(msg string) { _, _ = fmt.Fprintln(os.Stderr, "warning:", msg) })
		st.Digests = rows
		return err
	}
}
