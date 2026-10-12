package commands

// ssl_acme_traefik_test.go: the provider-aware reload of the ACME install step
// (P7-DEPL-25). docker, compose and the prober are the fixture's seams; the
// traefik-config one-shot is recognised by its image.

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/ssl/acme"
)

const traefikTestImage = "nself/nself-traefik-render:r2"

// traefikComposeJSON is `docker compose config --format json` for traefik-config.
const traefikComposeJSON = `{"services":{"traefik-config":{"image":"` + traefikTestImage + `","user":"0:0",
 "command":["--routes","/nself/routes.json","--out","/dynamic","--ssl","/nself/ssl","--static-out","/static"],
 "read_only":true,"cap_drop":["ALL"],"security_opt":["no-new-privileges:true"],
 "volumes":[{"type":"bind","source":"/p/.nself/generated/routes.json","target":"/nself/routes.json","read_only":true},
  {"type":"bind","source":"/p/ssl","target":"/nself/ssl","read_only":true},
  {"type":"bind","source":"/p/.nself/generated/traefik","target":"/dynamic"}]}}}`

// useTraefik switches the fixture to the traefik provider. renders records the
// traefik-config one-shots; failOn (1-based) makes that render fail.
func (f *acmeFix) useTraefik(failOn int) (renders *[]docker.RunSpec) {
	renders = new([]docker.RunSpec)
	oldP, oldC := acmeProxyProvider, traefikComposeConfig
	t := f.t
	t.Cleanup(func() { acmeProxyProvider, traefikComposeConfig = oldP, oldC })
	acmeProxyProvider = func(*config.Config, string) (string, error) { return build.ProviderTraefik, nil }
	traefikComposeConfig = func(context.Context, string) ([]byte, error) { return []byte(traefikComposeJSON), nil }
	acmeD.run = func(ctx context.Context, s docker.RunSpec) (string, string, error) {
		if s.Image != traefikTestImage {
			return f.fakeLego(ctx, s)
		}
		*renders = append(*renders, s)
		if len(*renders) == failOn {
			return "", "render: refused", errors.New("exit status 1")
		}
		return "", "", nil
	}
	return renders
}

func TestAcmeReloaderSelect(t *testing.T) {
	f := newACMEFix(t)
	nginx := &acmeRun{provider: build.ProviderNginx, res: &acme.Resolution{Container: "web-nginx-1"}}
	if got, ok := nginx.reloader().(acme.NginxReloader); !ok || got.Container != "web-nginx-1" {
		t.Errorf("provider nginx: reloader = %#v, want NginxReloader for web-nginx-1", nginx.reloader())
	}
	if _, ok := (&acmeRun{res: &acme.Resolution{}}).reloader().(acme.NginxReloader); !ok {
		t.Error("an unset provider must keep the nginx reloader")
	}
	traefik := &acmeRun{provider: build.ProviderTraefik, workdir: "/p", res: &acme.Resolution{Container: "web-traefik-1"}}
	if _, ok := traefik.reloader().(traefikReloader); !ok {
		t.Errorf("provider traefik: reloader = %#v, want traefikReloader", traefik.reloader())
	}

	// End to end through the command: nginx runs nginx -t + reload and no one-shot;
	// traefik runs the one-shot and touches no nginx container.
	if _, err := f.run(sslSetupCmd, "--acme", "--agree-tos"); err != nil {
		t.Fatal(err)
	}
	if len(f.execs) != 2 || !reflect.DeepEqual(f.execs[1], []string{"nginx", "-s", "reload"}) {
		t.Errorf("nginx provider execs = %v", f.execs)
	}
	f.execs = nil
	renders := f.useTraefik(0)
	out, err := f.run(sslRenewCmd, "--acme", "--force")
	if err != nil || len(f.execs) != 0 || len(*renders) != 1 {
		t.Fatalf("traefik renew: err=%v execs=%v renders=%d\n%s", err, f.execs, len(*renders), out)
	}
	spec := (*renders)[0]
	if spec.Network != "none" || !spec.ReadOnly || !reflect.DeepEqual(spec.CapDrop, []string{"ALL"}) ||
		!reflect.DeepEqual(spec.SecurityOpt, []string{"no-new-privileges:true"}) || spec.User != "0:0" || len(spec.Mounts) != 3 {
		t.Errorf("traefik-config run lost the service's hardening: %+v", spec)
	}
	if !strings.Contains(out, "traefik reloaded") || !strings.Contains(out, "traefik container: ") {
		t.Errorf("output does not name traefik:\n%s", out)
	}
}

func TestAcmeTraefikReloadRollback(t *testing.T) {
	f := newACMEFix(t)
	renders := f.useTraefik(2) // the 1st render is the setup install, the 2nd is the renewal's reload
	if _, err := f.run(sslSetupCmd, "--acme", "--agree-tos"); err != nil {
		t.Fatal(err)
	}
	before := acmeServed(t, f)
	_, err := f.run(sslRenewCmd, "--acme", "--force")
	if err == nil || !strings.Contains(err.Error(), "E151") || !strings.Contains(err.Error(), "previous certificates restored") ||
		!strings.Contains(err.Error(), "render: refused") {
		t.Fatalf("want E151 with a restore message and the render's stderr, got %v", err)
	}
	if string(acmeServed(t, f)) != string(before) {
		t.Error("the previous generation was not restored after a failed render")
	}
	if len(*renders) != 3 { // setup, the failed one, and the best-effort re-render of the restored pair
		t.Errorf("renders = %d, want 3 (setup, failed, re-render)", len(*renders))
	}
	if len(f.execs) != 0 {
		t.Errorf("traefik must never exec in an nginx container: %v", f.execs)
	}
}

func TestAcmeTraefikSpecRefusals(t *testing.T) {
	for name, raw := range map[string]string{
		"no service":   `{"services":{}}`,
		"named volume": `{"services":{"traefik-config":{"image":"i","command":["x"],"volumes":[{"type":"volume","source":"v","target":"/d"}]}}}`,
		"not json":     `nope`,
	} {
		if _, err := traefikConfigSpec([]byte(raw)); err == nil {
			t.Errorf("%s: want a refusal", name)
		}
	}
}
