package commands

// ssl_acme_traefik_test.go: the provider-aware reload of the ACME install step
// (P7-DEPL-25). docker, compose and the prober are the fixture's seams; the
// traefik-config one-shot is recognised by its image.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ssl/acme"
)

// Every test in the package reads a hermetic provider (never the developer's
// real plugin directory); the traefik tests switch it with useTraefik.
func init() {
	acmeProxyProvider = func(*config.Config, string) (string, error) { return build.ProviderNginx, nil }
}

const traefikTestImage = "nself/nself-traefik-render:r2"

// traefikComposeJSON is `docker compose config --format json` for traefik-config,
// with its mounts under the fixture's project and served stack.
func traefikComposeJSON(project, served string) string {
	return `{"services":{"traefik-config":{"image":"` + traefikTestImage + `","user":"0:0",
 "command":["--routes","/nself/routes.json","--out","/dynamic","--ssl","/nself/ssl","--static-out","/static"],
 "read_only":true,"cap_drop":["ALL"],"security_opt":["no-new-privileges:true"],
 "volumes":[{"type":"bind","source":"` + project + `/.nself/generated/routes.json","target":"/nself/routes.json","read_only":true},
  {"type":"bind","source":"` + served + `/ssl","target":"/nself/ssl","read_only":true},
  {"type":"bind","source":"` + project + `/.nself/generated/traefik","target":"/dynamic"}]}}}`
}

// useTraefik switches the fixture to the traefik provider. renders records the
// traefik-config one-shots; failOn (1-based) makes that render fail.
func (f *acmeFix) useTraefik(failOn int) (renders *[]docker.RunSpec) {
	renders = new([]docker.RunSpec)
	oldP, oldC := acmeProxyProvider, traefikComposeConfig
	t := f.t
	t.Cleanup(func() { acmeProxyProvider, traefikComposeConfig = oldP, oldC })
	acmeProxyProvider = func(*config.Config, string) (string, error) { return build.ProviderTraefik, nil }
	traefikComposeConfig = func(context.Context, string) ([]byte, error) {
		return []byte(traefikComposeJSON(f.project, f.served)), nil
	}
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
	if spec.Network != "none" || !spec.PullNever || !spec.ReadOnly || !reflect.DeepEqual(spec.CapDrop, []string{"ALL"}) ||
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
	if !reflect.DeepEqual((*renders)[2], (*renders)[1]) {
		t.Error("the re-render after the restore must run the same one-shot")
	}
}

// A failed reload whose re-render also fails says so: the restored pair is on
// disk but dynamic.yml may still name the failed generation.
func TestAcmeTraefikRestoreRenderFailureIsReported(t *testing.T) {
	f := newACMEFix(t)
	renders := f.useTraefik(0)
	if _, err := f.run(sslSetupCmd, "--acme", "--agree-tos"); err != nil {
		t.Fatal(err)
	}
	inner := acmeD.run
	acmeD.run = func(ctx context.Context, s docker.RunSpec) (string, string, error) {
		if s.Image == traefikTestImage {
			*renders = append(*renders, s)
			return "", "render: refused", errors.New("exit status 1")
		}
		return inner(ctx, s)
	}
	_, err := f.run(sslRenewCmd, "--acme", "--force")
	if err == nil || !strings.Contains(err.Error(), "previous certificates restored") || !strings.Contains(err.Error(), "also failed") {
		t.Fatalf("want the restore and the failed re-render reported, got %v", err)
	}
}

// A failed served-certificate check under traefik: Install re-renders itself
// (with the dead ctx when the check timed out), and the command renders once
// more, detached from ctx, so the restored pair is what dynamic.yml names.
func TestAcmeTraefikVerifyRollback(t *testing.T) {
	f := newACMEFix(t)
	renders := f.useTraefik(0)
	if _, err := f.run(sslSetupCmd, "--acme", "--agree-tos"); err != nil {
		t.Fatal(err)
	}
	before := acmeServed(t, f)
	acmeVerifyWait = time.Millisecond
	t.Cleanup(func() { acmeVerifyWait = 500 * time.Millisecond })
	acmeD.probe = func(context.Context, string, string, time.Duration) (ssl.ServedCert, error) {
		return ssl.ServedCert{SHA256: "deadbeef"}, nil
	}
	if _, err := f.run(sslRenewCmd, "--acme", "--force"); err == nil || !strings.Contains(err.Error(), "serves another certificate") {
		t.Fatalf("want a served-certificate mismatch, got %v", err)
	}
	if string(acmeServed(t, f)) != string(before) {
		t.Error("previous generation not restored after a fingerprint mismatch")
	}
	if len(*renders) != 4 { // setup, the renewal's reload, Install's re-render after the failed check, ours (idempotent)
		t.Errorf("renders = %d, want 4", len(*renders))
	}
}

// The renderer runs as root without capabilities: generation directories (0700
// as core writes them) must be traversable before it runs.
func TestAcmeTraefikGenerationDirsTraversable(t *testing.T) {
	f := newACMEFix(t)
	f.useTraefik(0)
	var seen []os.FileMode
	inner := acmeD.run
	acmeD.run = func(ctx context.Context, s docker.RunSpec) (string, string, error) {
		if s.Image == traefikTestImage {
			dirs, _ := filepath.Glob(filepath.Join(f.ssl, "certificates", ".*.gen-*"))
			for _, d := range dirs {
				fi, err := os.Stat(d)
				if err != nil {
					t.Fatal(err)
				}
				seen = append(seen, fi.Mode().Perm())
			}
		}
		return inner(ctx, s)
	}
	if _, err := f.run(sslSetupCmd, "--acme", "--agree-tos"); err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 {
		t.Fatal("no generation directory existed when the renderer ran")
	}
	for _, m := range seen {
		if m&0o011 != 0o011 {
			t.Errorf("generation dir mode %v is not traversable by others", m)
		}
	}
}

func TestAcmeTraefikSpecRefusals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the spec carries POSIX host paths")
	}
	roots := []string{"/p"}
	mount := func(src, dst string, ro bool) string {
		return `{"type":"bind","source":"` + src + `","target":"` + dst + `","read_only":` + map[bool]string{true: "true", false: "false"}[ro] + `}`
	}
	svc := func(extra string) string {
		return `{"services":{"traefik-config":{"image":"nself/r:1","command":["x"]` + extra + `}}}`
	}
	for name, raw := range map[string]string{
		"no service":        `{"services":{}}`,
		"named volume":      svc(`,"volumes":[{"type":"volume","source":"v","target":"/d"}]`),
		"not json":          `nope`,
		"no image":          `{"services":{"traefik-config":{"command":["x"]}}}`,
		"no command":        `{"services":{"traefik-config":{"image":"i"}}}`,
		"entrypoint":        svc(`,"entrypoint":["/bin/sh"]`),
		"tmpfs":             svc(`,"tmpfs":["/tmp"]`),
		"environment":       svc(`,"environment":{"A":"b"}`),
		"image as a flag":   `{"services":{"traefik-config":{"image":"-v=/:/h","command":["x"]}}}`,
		"image with space":  `{"services":{"traefik-config":{"image":"a b","command":["x"]}}}`,
		"mount outside":     svc(`,"volumes":[` + mount("/etc", "/nself/ssl", true) + `]`),
		"mount of the root": svc(`,"volumes":[` + mount("/p", "/nself/ssl", true) + `]`),
		"dotdot source":     svc(`,"volumes":[` + mount("/p/../etc", "/nself/ssl", true) + `]`),
		"writable ssl":      svc(`,"volumes":[` + mount("/p/ssl", "/nself/ssl", false) + `]`),
		"relative target":   svc(`,"volumes":[` + mount("/p/x", "dynamic", false) + `]`),
	} {
		if _, err := traefikConfigSpec([]byte(raw), roots); err == nil {
			t.Errorf("%s: want a refusal", name)
		}
	}
	ok := svc(`,"volumes":[` + mount("/p/ssl", "/nself/ssl", true) + `,` + mount("/p/d", "/dynamic", false) + `]`)
	if spec, err := traefikConfigSpec([]byte(ok), roots); err != nil || !spec.PullNever || len(spec.Mounts) != 2 {
		t.Errorf("valid spec refused or without --pull never: %+v %v", spec, err)
	}
}

// Root renewals select the exported owner; interactive renewals keep theirs.
func TestAcmeTraefikGenerationHandOver(t *testing.T) {
	root := t.TempDir()
	r := &acmeRun{provider: build.ProviderTraefik, workdir: root}
	old := traefikEuid
	t.Cleanup(func() { traefikEuid = old })
	traefikEuid = func() int { return 0 }
	if got := r.installAccess(); got.UID != -1 || got.GID != -1 {
		t.Fatalf("missing ids: %+v", got)
	}
	acmeWriteFile(t, filepath.Join(root, ".nself", "compose.env"), []byte("NSELF_HOST_UID=1001\nNSELF_HOST_GID=1002\n"))
	if got := r.installAccess(); got.UID != 1001 || got.GID != 1002 || got.DirMode != 0o711 || got.ParentMode != 0o751 {
		t.Fatalf("root policy: %+v", got)
	}
	traefikEuid = func() int { return 1001 }
	if got := r.installAccess(); got.UID != -1 || got.GID != -1 {
		t.Fatalf("interactive policy: %+v", got)
	}
	r.provider = build.ProviderNginx
	if r.installAccess() != nil {
		t.Fatal("nginx defaults changed")
	}
}

// Install's own re-render uses the dead ctx when a served-certificate check timed
// out; ours must not.
func TestAcmeTraefikRerenderIgnoresCancelledContext(t *testing.T) {
	f := newACMEFix(t)
	f.useTraefik(0)
	var live []bool
	acmeD.run = func(ctx context.Context, s docker.RunSpec) (string, string, error) {
		live = append(live, ctx.Err() == nil)
		return "", "", nil
	}
	r := &acmeRun{provider: build.ProviderTraefik, workdir: f.project, res: &acme.Resolution{SSLDir: f.ssl, Root: f.served}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cause := errors.New("served certificate check failed")
	if err := r.afterFailedInstall(ctx, cause); !errors.Is(err, cause) || len(live) != 1 || !live[0] {
		t.Errorf("err=%v live=%v: the re-render must run with a live context", err, live)
	}
}

// traefik publishes NSELF_HTTPS_PORT / NSELF_HTTP_PORT, not the nginx keys.
func TestAcmeTraefikServedEnv(t *testing.T) {
	f := newACMEFix(t)
	acmeWriteFile(t, filepath.Join(f.served, ".env"), []byte("NGINX_SSL_PORT=8443\nNGINX_HTTP_PORT=8080\nNGINX_BIND_IP=10.0.0.5\nNSELF_HTTPS_PORT=9443\n"))
	r := &acmeRun{provider: build.ProviderNginx, cfg: &config.Config{Env: "dev"}, res: &acme.Resolution{Root: f.served}}
	if got := ssl.ServedAddr(r.servedEnv()); got != "10.0.0.5:8443" {
		t.Errorf("nginx addr = %s", got)
	}
	r.provider = build.ProviderTraefik
	if got, http := ssl.ServedAddr(r.servedEnv()), httpProbeAddr(r.servedEnv()); got != "127.0.0.1:9443" || http != "127.0.0.1:80" {
		t.Errorf("traefik addrs = %s %s, want 127.0.0.1:9443 and 127.0.0.1:80", got, http)
	}
}

// A symlink inside the project that leads out of it must not pass the mount-root check.
func TestAcmeTraefikSpecRefusesSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX symlinks")
	}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	outside, _ := filepath.EvalSymlinks(t.TempDir())
	if err := os.Symlink(outside, filepath.Join(root, "out")); err != nil {
		t.Fatal(err)
	}
	raw := `{"services":{"traefik-config":{"image":"nself/r:1","command":["x"],"volumes":[{"type":"bind","source":"` +
		filepath.Join(root, "out") + `","target":"/dynamic"}]}}}`
	if _, err := traefikConfigSpec([]byte(raw), []string{root}); err == nil {
		t.Error("a mount source that is a symlink out of the project was accepted")
	}
}
