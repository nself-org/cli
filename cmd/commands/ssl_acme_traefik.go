package commands

// ssl_acme_traefik.go: the TLS lifecycle under the traefik proxy provider
// (P7-DEPL-25, EPIC D25). With traefik there is no nginx container to
// `nginx -s reload`: the file provider reloads only when dynamic.yml parses
// differently, and the renderer points each certFile at the lineage's resolved
// generation directory. So after the install pair switch the reload step is
// "run the plugin's traefik-config one-shot again" (docker.RunOneShot, no
// socket, no restart); the served-fingerprint check that follows is the
// reload's proof. HTTP-01 reaches the webroot through traefik-acme (P7-DEPL-02),
// so the challenge probe is provider-neutral. Certificates stay core-issued
// (ADR 0026/0027 section 2): no traefik ACME resolver exists.

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/nginxtopo"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ssl/acme"
)

const (
	traefikConfigService = "traefik-config" // the one-shot renderer (plugins free/traefik)
	traefikService       = "traefik"        // the serving container
	traefikSSLMount      = "/nself/ssl"     // where the plugin mounts ./ssl, read-only
)

// acmeProxyProvider derives the serving proxy exactly as `nself build` does.
var acmeProxyProvider = func(cfg *config.Config, workdir string) (string, error) {
	return build.ProxyProvider(cfg, workdir, build.DefaultPluginDir())
}

// traefikComposeConfig returns the resolved compose model of the traefik-config
// service (`docker compose config --format json`), seam for tests.
var traefikComposeConfig = func(ctx context.Context, workdir string) ([]byte, error) {
	files, err := build.ReadComposeManifest(workdir)
	if err != nil {
		return nil, fmt.Errorf("reading the compose manifest (run `nself build`): %w", err)
	}
	c := docker.NewCompose(files...)
	c.EnvFiles = build.ComposeEnvFiles(workdir)
	return c.OutputProject(ctx, workdir, docker.ProjectRun{}, "config", "--format", "json", traefikConfigService)
}

// traefikReloader implements acme.Reloader for the traefik provider.
type traefikReloader struct {
	workdir string
	config  func(context.Context, string) ([]byte, error)
	run     func(context.Context, docker.RunSpec) (string, string, error)
}

// Reload re-runs the traefik-config one-shot. A failed render leaves the
// previous dynamic.yml in place (the renderer writes atomically), so a failure
// here means nothing was reloaded.
func (t traefikReloader) Reload(ctx context.Context) error {
	raw, err := t.config(ctx, t.workdir)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", traefikConfigService, err)
	}
	spec, err := traefikConfigSpec(raw)
	if err != nil {
		return err
	}
	if _, stderr, err := t.run(ctx, spec); err != nil {
		return fmt.Errorf("%s one-shot failed (build and start the stack once so its image exists): %w\n%s",
			traefikConfigService, err, strings.TrimSpace(stderr))
	}
	return nil
}

// traefikConfigSpec turns the compose model of traefik-config into a one-shot
// run that keeps the service's own hardening: read-only root, no capabilities,
// no-new-privileges, and no network (the renderer needs none).
func traefikConfigSpec(raw []byte) (docker.RunSpec, error) {
	var doc struct {
		Services map[string]struct {
			Image       string   `json:"image"`
			Command     []string `json:"command"`
			User        string   `json:"user"`
			ReadOnly    bool     `json:"read_only"`
			CapDrop     []string `json:"cap_drop"`
			SecurityOpt []string `json:"security_opt"`
			Volumes     []struct {
				Type     string `json:"type"`
				Source   string `json:"source"`
				Target   string `json:"target"`
				ReadOnly bool   `json:"read_only"`
			} `json:"volumes"`
		} `json:"services"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return docker.RunSpec{}, fmt.Errorf("parsing the compose model of %s: %w", traefikConfigService, err)
	}
	s, ok := doc.Services[traefikConfigService]
	if !ok || s.Image == "" || len(s.Command) == 0 {
		return docker.RunSpec{}, fmt.Errorf("the compose model has no usable %s service (is the traefik plugin installed and built?)", traefikConfigService)
	}
	spec := docker.RunSpec{Image: s.Image, Args: s.Command, User: s.User, Network: "none",
		ReadOnly: s.ReadOnly, CapDrop: s.CapDrop, SecurityOpt: s.SecurityOpt}
	for _, v := range s.Volumes {
		if v.Type != "bind" {
			return docker.RunSpec{}, fmt.Errorf("%s mounts %s as a %s volume; only bind mounts are supported", traefikConfigService, v.Target, v.Type)
		}
		spec.Mounts = append(spec.Mounts, docker.Mount{Source: v.Source, Destination: v.Target, Type: "bind", ReadOnly: v.ReadOnly})
	}
	return spec, nil
}

// useProvider records the proxy provider and, for traefik, points the preflight
// at the traefik container and its /nself/ssl mount instead of nginx's.
func (r *acmeRun) useProvider(in *acme.Input) error {
	var err error
	if r.provider, err = acmeProxyProvider(r.cfg, r.workdir); err != nil {
		return e151(acmeRefuse("fix the proxy configuration (`nself build` reports the same refusal)", "choosing the proxy provider: %v", err))
	}
	if r.provider != build.ProviderTraefik {
		return nil
	}
	find := in.FindNginx
	if find == nil {
		find = docker.FindServiceContainer
	}
	in.FindNginx = func(ctx context.Context, m docker.ServiceMatch) (string, error) {
		m.Service = traefikService
		return find(ctx, m)
	}
	mounts := in.Mounts
	if mounts == nil {
		mounts = docker.ContainerMounts
	}
	in.Mounts = func(ctx context.Context, c string) ([]docker.Mount, error) {
		ms, err := mounts(ctx, c)
		for i := range ms { // the whole-ssl-dir check is written for nginx's path
			if ms[i].Destination == traefikSSLMount {
				ms[i].Destination = nginxtopo.NginxSSLContainerPath
			}
		}
		return ms, err
	}
	return nil
}

// reloader is the install step that makes the serving proxy pick up new files.
func (r *acmeRun) reloader() acme.Reloader {
	if r.provider == build.ProviderTraefik {
		return traefikReloader{workdir: r.workdir, config: traefikComposeConfig, run: acmeD.run}
	}
	return acme.NginxReloader{Container: r.res.Container, Exec: acmeD.exec}
}

// proxyName names the serving proxy in messages.
func (r *acmeRun) proxyName() string { return cmp.Or(r.provider, build.ProviderNginx) }

// printResolution prints the preflight; the nginx-only lines are named for the
// proxy actually serving.
func (r *acmeRun) printResolution() {
	if r.provider != build.ProviderTraefik {
		r.res.Print(r.out)
		return
	}
	r.say("served root:       %s\nserved ssl dir:    %s\nproxy provider:    traefik (reload: re-run %s)\ntraefik container: %s\nacme contact:      %s",
		r.res.Root, r.res.SSLDir, traefikConfigService, r.res.Container, r.res.Contact)
	if r.res.AgeBin != "" {
		r.say("age:               %s (key %s)", r.res.AgeBin, r.res.AgeKey)
	}
}

// restoreAfterFailedInstall re-renders the proxy configuration after Install
// restored the previous generation, so dynamic.yml names the restored pair
// even if the failed attempt got as far as writing it. Best effort.
func (r *acmeRun) restoreAfterFailedInstall(ctx context.Context) {
	if r.provider == build.ProviderTraefik {
		_ = r.reloader().Reload(ctx)
	}
}

// probeHosts picks one served host per target. Under nginx it is the first host
// by name whose conf uses that target; traefik has no conf to scan, so (and when
// no conf matches) it is the lineage's first name, a wildcard probed as check.<zone>.
func (r *acmeRun) probeHosts(l acme.Lineage) (out []string) {
	var hosts map[string]string
	if r.provider != build.ProviderTraefik {
		hosts, _ = ssl.ServedHosts(r.res.NginxDir)
	}
	for _, t := range l.Targets {
		pick := ""
		for h, c := range hosts {
			if strings.Contains(c, "/"+t+"/") && (pick == "" || h < pick) {
				pick = h
			}
		}
		out = append(out, cmp.Or(pick, strings.Replace(l.Domains[0], "*.", "check.", 1)))
	}
	return out
}
