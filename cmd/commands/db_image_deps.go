package commands

// db_image_deps.go — the seams of the image switch: config, compose stack and
// the reconcile applier (EPIC D6). Only cmd/commands calls reconcile (EPIC D6).
//
// Purpose: give internal/postgres/imageswitch its Stack (docker compose) and
// Applier (env lines + regenerate compose and lock through the build plan path).
// Inputs: the project directory and the loaded config. Outputs: the Config and Deps.
// Constraints: the applier writes only POSTGRES_IMAGE and POSTGRES_DATA_VOLUME
// to the project's .env, restores the file byte for byte if the regeneration
// fails, and checks that the cascade resolves to the values it wrote (an
// override in another env file would otherwise leave the old volume in use).

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/postgres/imageswitch"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
)

// imageSwitchConfig builds the engine's Config from the loaded project config.
func imageSwitchConfig(dir string, cfg *config.Config) (imageswitch.Config, error) {
	previous, err := config.ReadEnvFile(filepath.Join(dir, ".env"))
	if err != nil && !os.IsNotExist(err) {
		return imageswitch.Config{}, config.EnvReadError(filepath.Join(dir, ".env"), err)
	}
	prev := make(map[string]string)
	for _, key := range []string{imageswitch.EnvImage, imageswitch.EnvDataVolume} {
		if value, present := previous[key]; present {
			prev[key] = value
		}
	}
	return imageswitch.Config{
		ProjectDir: dir, ProjectName: cfg.ProjectName, Service: "postgres", Container: cfg.ProjectName + "_postgres",
		User: cfg.Postgres.User, Password: cfg.Postgres.Password, TargetFor: pgvectorImageForMajor,
		PrevEnv: prev,
	}, nil
}

// pgvectorImageForMajor is the pgvector image compose would run for a Postgres major.
func pgvectorImageForMajor(major int) (string, error) {
	if major < 12 {
		return "", fmt.Errorf("postgres %d is too old for the pgvector image", major)
	}
	img := compose.ResolvePostgresImage(config.PostgresConfig{Version: strconv.Itoa(major), Extensions: []string{"pgvector"}})
	if !strings.Contains(img, "pgvector") {
		return "", fmt.Errorf("compose resolves Postgres %d to %q, not a pgvector image", major, img)
	}
	return img, nil
}

// imageSwitchDepsFor builds the effects of a run; tests substitute fakes.
var imageSwitchDepsFor = imageSwitchDeps

// imageSwitchDeps wires the production effects. Progress goes to stderr.
func imageSwitchDeps(cmd *cobra.Command, dir string, asJSON bool) imageswitch.Deps {
	d := imageswitch.Deps{Runtime: imageswitch.DockerRuntime{}, Stack: &composeStack{dir: dir}, Out: cmd.ErrOrStderr()}
	ap := &envApplier{dir: dir, stderr: cmd.ErrOrStderr()}
	if !asJSON {
		ap.diff = cmd.ErrOrStderr()
	}
	d.Applier = ap
	return d
}

// composeStack is the project's compose lifecycle.
type composeStack struct{ dir string }

// compose reads the manifest each call: the switch regenerates the files mid-run.
func (s *composeStack) compose() *docker.Compose {
	files, _ := build.ReadComposeManifest(s.dir)
	c := docker.NewCompose(files...)
	c.EnvFiles = build.ComposeEnvFiles(s.dir)
	return c
}

// Running lists the running services.
func (s *composeStack) Running(ctx context.Context) ([]string, error) {
	infos, err := s.compose().ComposePs(ctx, s.dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, i := range infos {
		if strings.EqualFold(i.State, "running") && i.Service != "" {
			names = append(names, i.Service)
		}
	}
	sort.Strings(names)
	return names, nil
}

// Stop stops services (compose keeps their containers).
func (s *composeStack) Stop(ctx context.Context, services ...string) error {
	return s.compose().ComposeStop(ctx, s.dir, services...)
}

// Up starts services, or the whole stack when none are named.
func (s *composeStack) Up(ctx context.Context, services ...string) error {
	return s.compose().ComposeUp(ctx, s.dir, services...)
}

// WaitHealthy polls the service's container until it reports healthy.
func (s *composeStack) WaitHealthy(ctx context.Context, service string) error {
	infos, err := s.compose().ComposePs(ctx, s.dir)
	if err != nil {
		return err
	}
	var name string
	for _, i := range infos {
		if i.Service == service {
			name = i.Name
		}
	}
	if name == "" {
		return fmt.Errorf("service %s has no container", service)
	}
	for {
		st, err := docker.GetHealthStatus(ctx, name)
		if err == nil && (st == "healthy" || st == "none") {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s is %q: %w", name, st, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

// envApplier writes env lines and regenerates through reconcile.
type envApplier struct {
	dir    string
	stderr io.Writer
	diff   io.Writer
}

func (a *envApplier) request(env map[string]string) reconcile.Request {
	var remove []string
	for _, key := range []string{imageswitch.EnvImage, imageswitch.EnvDataVolume} {
		if _, present := env[key]; !present {
			remove = append(remove, key)
		}
	}
	return reconcile.Request{
		ProjectDir: a.dir, Command: reconcile.CmdConfigSet,
		Trigger:    reconcile.Trigger{Kind: reconcile.TriggerConfig, Subject: imageswitch.EnvDataVolume},
		EnvOverlay: map[string]map[string]string{".env": env}, Stderr: a.stderr, DiffOut: a.diff,
		EnvRemove: map[string][]string{".env": remove},
	}
}

// Plan returns the change plan of the lines; nothing is written.
func (a *envApplier) Plan(ctx context.Context, env map[string]string) (any, error) {
	req := a.request(env)
	req.Containers = true
	state, err := reconcile.LoadGeneratedState(a.dir)
	if err != nil {
		return nil, err
	}
	req.HandEdited = state.HandEditedFn(a.dir)
	req.DiffOut = nil
	return reconcile.Compute(ctx, req)
}

// Apply writes the lines and regenerates; on failure the env file is restored.
func (a *envApplier) Apply(ctx context.Context, env map[string]string) error {
	defer build.SnapshotEnv()()
	// config.Load exports the switched values. Removed lines must not inherit
	// those stale process values when reconcile renders the restored cascade.
	for _, key := range []string{imageswitch.EnvImage, imageswitch.EnvDataVolume} {
		if _, present := env[key]; !present {
			if err := os.Unsetenv(key); err != nil {
				return err
			}
		}
	}
	envFile := filepath.Join(a.dir, ".env")
	keys := []string{imageswitch.EnvImage, imageswitch.EnvDataVolume}
	var writes []*envLineWrite
	defer func() {
		for _, w := range writes {
			w.releaseSignals()
		}
	}()
	opt := reconcile.ApplyOptions{
		Yes: true, // the command's own confirmation (or --yes) already covered this
		BeforeWrite: func() error {
			for _, k := range keys {
				w := &envLineWrite{path: envFile}
				writes = append(writes, w)
				value, present := env[k]
				if err := w.writeEnvPresence(k, value, present); err != nil {
					return err
				}
			}
			return checkEnvResolves(a.dir, env)
		},
		Rollback: func() error {
			var first error
			for i := len(writes) - 1; i >= 0; i-- {
				if err := writes[i].restore(); err != nil && first == nil {
					first = err
				}
			}
			return first
		},
	}
	_, err := reconcile.Apply(ctx, a.request(env), opt)
	return err
}

// checkEnvResolves fails when the env cascade does not resolve the written keys to the written values.
func checkEnvResolves(dir string, env map[string]string) error {
	cfg, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf("the project config does not load after the env change: %w", err)
	}
	got := map[string]string{imageswitch.EnvImage: cfg.Postgres.Image, imageswitch.EnvDataVolume: cfg.Postgres.DataVolume}
	for k, want := range env {
		if strings.TrimSpace(got[k]) != strings.TrimSpace(want) {
			return fmt.Errorf("%s is set by another env file in the cascade, so writing .env does not change it; remove that line and run again", k)
		}
	}
	return nil
}
