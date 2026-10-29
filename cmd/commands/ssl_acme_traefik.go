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
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
	workdir, sslDir string
	roots           []string // host paths the one-shot may mount from: the project and the served stack
	config          func(context.Context, string) ([]byte, error)
	run             func(context.Context, docker.RunSpec) (string, string, error)
}

// Reload re-runs the traefik-config one-shot. A failed render leaves the
// previous dynamic.yml in place (the renderer writes atomically), so a failure
// here means nothing was reloaded.
func (t traefikReloader) Reload(ctx context.Context) error {
	raw, err := t.config(ctx, t.workdir)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", traefikConfigService, err)
	}
	spec, err := traefikConfigSpec(raw, t.roots)
	if err != nil {
		return err
	}
	if _, stderr, err := t.run(ctx, spec); err != nil {
		return fmt.Errorf("%s one-shot failed (build and start the stack once so its image exists): %w\n%s",
			traefikConfigService, err, strings.TrimSpace(stderr))
	}
	return nil
}

// useProvider records the proxy provider and, for traefik, points the preflight
// at the traefik container and its /nself/ssl mount instead of nginx's.
func (r *acmeRun) useProvider(in *acme.Input) error {
	var err error
	if r.provider, err = acmeProxyProvider(r.cfg, r.workdir); err != nil {
		// `nself build` refuses the same configuration; renewal must not stop serving nginx stacks over it.
		slog.Warn("proxy provider not determined; assuming nginx", "error", err)
		r.provider = build.ProviderNginx
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
		return traefikReloader{workdir: r.workdir, sslDir: r.res.SSLDir, roots: []string{r.workdir, r.res.Root},
			config: traefikComposeConfig, run: acmeD.run}
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

// afterFailedInstall re-renders the proxy configuration once Install restored
// the previous generation, so dynamic.yml names the restored pair even if the
// failed attempt got as far as writing it. It always runs (a render that is
// already right is a no-op for traefik) and does not depend on ctx: a cancelled
// or expired ctx is exactly when Install's own re-render could not run. A
// failing re-render is added to the error: the restored pair is on disk but
// traefik may still be pointed at the failed generation.
func (r *acmeRun) afterFailedInstall(ctx context.Context, err error) error {
	if r.provider != build.ProviderTraefik {
		return err
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	if rerr := r.reloader().Reload(ctx); rerr != nil {
		return errors.Join(err, fmt.Errorf("re-rendering %s after the restore also failed (dynamic.yml may still name the failed generation; fix the cause and re-run): %w", traefikConfigService, rerr))
	}
	return err
}

// servedEnv is the served stack's env as the probes read it. traefik publishes
// ${NSELF_HTTPS_PORT:-443} and ${NSELF_HTTP_PORT:-80} on every address, so the
// nginx keys (NGINX_SSL_PORT, NGINX_HTTP_PORT, NGINX_BIND_IP) are replaced by
// those, else the probes would dial a port traefik does not serve.
func (r *acmeRun) servedEnv() map[string]string {
	env, _ := ssl.ServedEnv(r.res.Root, r.cfg.Env)
	if r.provider != build.ProviderTraefik {
		return env
	}
	out := map[string]string{}
	for k, v := range env {
		out[k] = v
	}
	for _, k := range []string{"NGINX_SSL_PORT", "NGINX_HTTPS_PORT", "NGINX_HTTP_PORT", "NGINX_BIND_IP"} {
		delete(out, k)
	}
	out["NGINX_HTTPS_PORT"], out["NGINX_HTTP_PORT"] = env["NSELF_HTTPS_PORT"], env["NSELF_HTTP_PORT"]
	return out
}

// probeHosts picks one served host per target. Under nginx it is the first host
// by name whose conf uses that target; traefik has no conf to scan, so (and when
// no conf matches) it is the lineage's first name, a wildcard probed as check.<zone>.
func (r *acmeRun) probeHosts(l acme.Lineage) (out []string) {
	var hosts map[string]string
	if r.provider != build.ProviderTraefik {
		hosts, _ = ssl.ServedHosts(r.res.NginxDir)
		hosts, _ = acme.ResolveServedLinks(r.res.SSLDir, hosts)
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

func (r *acmeRun) installOwner() *acme.FileOwner {
	if r.provider != "traefik" || traefikEuid() != 0 {
		return nil
	}
	uid, gid := hostOwner(r.workdir)
	if uid < 0 {
		return nil
	}
	return &acme.FileOwner{UID: uid, GID: gid}
}
