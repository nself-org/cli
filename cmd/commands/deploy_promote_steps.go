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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/controlplane"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/deploy/archguard"
	"github.com/nself-org/cli/internal/errs"
	remotesdk "github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
}

// preparePromoteEngine resolves local inventory facts and wires concrete steps.
func preparePromoteEngine(cmd *cobra.Command, workdir, source, target string, dryRun, jsonOut bool) (cpromote.Engine, *cpromote.State, error) {
	inv, err := controlplane.Load(workdir)
	if err != nil {
		return cpromote.Engine{}, nil, err
	}
	source, ok := inventoryEnvName(inv, source)
	if !ok {
		return cpromote.Engine{}, nil, errs.Newf("E483", "unknown promotion source %q", source)
	}
	target, ok = inventoryEnvName(inv, target)
	if !ok {
		return cpromote.Engine{}, nil, errs.Newf("E483", "unknown promotion target %q", target)
	}
	srcTargets, err := controlplane.ResolveTargets(inv, controlplane.Selector{Env: source})
	if err != nil {
		return cpromote.Engine{}, nil, err
	}
	tgtTargets, err := controlplane.ResolveTargets(inv, controlplane.Selector{Env: target})
	if err != nil {
		return cpromote.Engine{}, nil, err
	}
	st := &cpromote.State{Source: source, Target: target, SourceTier: srcTargets[0].Tier, TargetTier: tgtTargets[0].Tier, DryRun: dryRun, Values: map[string]any{}}
	if err := cpromote.ValidateTiers(source, target, st.SourceTier, st.TargetTier); err != nil {
		return cpromote.Engine{}, nil, err
	}
	w := &promoteWiring{workdir: workdir, source: source, target: target, inv: inv, started: time.Now().UTC()}
	w.sourceServer, _ = primaryServer(inv, source)
	w.targetServer, _ = primaryServer(inv, target)
	eng := cpromote.Engine{Steps: promoteSteps(w, jsonOut)}
	if dryRun {
		return eng, st, nil
	}
	yes, _ := cmd.Flags().GetBool("yes")
	if prodClassFn(inv, target) && !yes && !deployConfirmPrompt(cmd.Context(), fmt.Sprintf("Promote %s to production target %s", source, target)) {
		return cpromote.Engine{}, nil, errs.New("E403", "promotion to a production-class target requires --yes or terminal confirmation")
	}
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
		step("resolve", func(context.Context, *cpromote.State) error { return nil }),
		step("confirm", func(context.Context, *cpromote.State) error { return nil }),
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
	return c.CombinedOutput()
}
func (localPromoteRemote) Rsync(context.Context, []string, string, string, string) error {
	return fmt.Errorf("local promotion does not use rsync")
}

func promoteRemote(ctx context.Context, srv controlplane.Server, env, workdir string) (deploy.Remote, error) {
	if srv.Host == "" {
		return localPromoteRemote{root: workdir}, nil
	}
	return openDeployRemote(ctx, srv, os.Getenv(srv.SSHKeyRef), env)
}

func runPromoteApply(w *promoteWiring) func(context.Context, *cpromote.State) error {
	return func(ctx context.Context, st *cpromote.State) error {
		if w.apply == nil {
			return fmt.Errorf("promotion build did not produce an artifact set")
		}
		remotes := map[string]deploy.Remote{}
		paths := map[string]string{}
		w.apply.ProdClass, w.apply.BackupID, w.apply.BackupHost = prodClassFn(w.inv, w.target), st.BackupID, st.BackupHost
		w.apply.Open = func(c context.Context, srv controlplane.Server, key, env string) (deploy.Remote, error) {
			r, err := openDeployRemote(c, srv, key, env)
			if err == nil {
				remotes[r.Label()], paths[r.Label()] = r, srv.RemotePath
			}
			return r, err
		}
		w.apply.Record = func(_ controlplane.Server, res deploy.ApplyResult, err error) {
			if !res.Synced {
				return
			}
			if st.PreDeployTar == nil {
				st.PreDeployTar, st.PreDeployChanged, st.PreDeployAbsent = map[string]string{}, map[string][]string{}, map[string][]string{}
			}
			st.PreDeployTar[res.BackupHost] = res.PreDeployTar
			st.PreDeployChanged[res.BackupHost], st.PreDeployAbsent[res.BackupHost] = res.Changed, res.Absent
		}
		st.Values["apply_remotes"], st.Values["apply_paths"] = remotes, paths
		result, err := controlplane.RunWithApply(ctx, w.inv, w.target, newDeployProber(w.workdir), w.compose, w.apply)
		if err == nil {
			err = failedServersError(result)
		}
		return err
	}
}

func rollbackPromoteApply(ctx context.Context, w *promoteWiring, st *cpromote.State) error {
	remotes, _ := st.Values["apply_remotes"].(map[string]deploy.Remote)
	paths, _ := st.Values["apply_paths"].(map[string]string)
	for host, tar := range st.PreDeployTar {
		r := remotes[host]
		if r == nil {
			return fmt.Errorf("promotion rollback has no remote for %s", host)
		}
		if err := deploy.RollbackRemote(ctx, deploy.RollbackOptions{Remote: r, RemotePath: paths[host], Tar: tar, Changed: st.PreDeployChanged[host], Absent: st.PreDeployAbsent[host], StrictHasura: true}); err != nil {
			return err
		}
	}
	return nil
}

func runPromoteRecord(w *promoteWiring) func(context.Context, *cpromote.State) error {
	return func(_ context.Context, st *cpromote.State) error {
		now := time.Now().UTC()
		saved, _ := st.Values["saved_manifest"].(cpromote.SavedManifest)
		return cpromote.SaveRecord(w.workdir, cpromote.Record{
			ID: deploy.NewBackupID(w.started), Source: st.Source, Target: st.Target,
			StartedAt: w.started, CompletedAt: now, Status: "ok", SnapshotTag: st.SnapshotTag,
			BackupID: st.BackupID, BackupHost: st.BackupHost, PreDeployTar: st.PreDeployTar,
			Changed: st.PreDeployChanged, Absent: st.PreDeployAbsent, Digests: st.Digests,
			SavedManifest: saved,
		})
	}
}

func runPromoteCapture(w *promoteWiring) func(context.Context, *cpromote.State) error {
	return func(ctx context.Context, st *cpromote.State) error {
		images, err := readPromoteImages(filepath.Join(w.workdir, "docker-compose.yml"))
		if err != nil {
			return err
		}
		rp := promotePath(w.sourceServer, w.workdir)
		run := func(c context.Context, command string) ([]byte, error) {
			return w.sourceRemote.Run(c, "cd "+remotesdk.ShellQuote(rp)+" && "+command)
		}
		captured, err := cpromote.CaptureSource(ctx, run, images, archguard.RegistryLookup)
		st.Values["source_images"] = captured
		return err
	}
}

func runPromoteCompare(w *promoteWiring) func(context.Context, *cpromote.State) error {
	return func(ctx context.Context, st *cpromote.State) error {
		images, err := readPromoteImages(w.compose)
		if err != nil {
			return err
		}
		arch := strings.TrimSpace(w.targetServer.Arch)
		if arch == "" {
			raw, probeErr := w.targetRemote.Run(ctx, "uname -m")
			if probeErr != nil {
				return probeErr
			}
			arch = strings.TrimSpace(string(raw))
		}
		if arch == "x86_64" {
			arch = "amd64"
		} else if arch == "aarch64" {
			arch = "arm64"
		}
		source, _ := st.Values["source_images"].([]cpromote.SourceImage)
		rows, err := cpromote.CompareDigests(ctx, source, images, arch, st.TargetTier, nil, func(msg string) { _, _ = fmt.Fprintln(os.Stderr, "warning:", msg) })
		st.Digests = rows
		return err
	}
}

func readPromoteImages(file string) ([]cpromote.Image, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("reading promotion compose: %w", err)
	}
	var doc struct {
		Services map[string]struct {
			Image string    `yaml:"image"`
			Build yaml.Node `yaml:"build"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing promotion compose: %w", err)
	}
	images := make([]cpromote.Image, 0, len(doc.Services))
	for name, service := range doc.Services {
		images = append(images, cpromote.Image{Service: name, Ref: service.Image, Build: service.Build.Kind != 0})
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Service < images[j].Service })
	return images, nil
}
