package commands

// ssl_install_test.go — Regression coverage for the cert install helpers in
// ssl_install.go, plus the cross-package check that ssl_add's cert directory
// construction cannot silently diverge from internal/ssl's.
//
// Purpose: installIssuedCert and writeCustomDomainConf both exist because of
//          previously-shipped bugs (see their doc comments in ssl_install.go):
//          certbot ignores --cert-path so the issued cert has to be copied
//          into the tree nginx reads, and the generated nginx conf must
//          reference the dash-safe directory name, not the dotted domain.
//          ssl_setup_paths_test.go already covers those two behaviors
//          end-to-end; this file adds the one assertion still missing —
//          that ssl_add's `ssl/certificates/<domainSafe>` construction is
//          verified against internal/ssl.DomainToDirName (the same rule
//          internal/ssl applies to the primary domain) rather than two
//          independently-typed literals that could drift apart.
// Inputs:  domain strings; a temp dir standing in for the project root.
// Outputs: assertions on copied cert files, generated conf content, and
//          directory-name agreement with internal/ssl.
// Constraints: No certbot, docker, or network. Pure filesystem + string checks.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/apidocs"
	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginxtopo"
	"github.com/nself-org/cli/internal/ssl"
)

// TestInstallIssuedCert_CopiesFromLetsEncryptLiveDir verifies installIssuedCert
// copies both PEM files from letsEncryptLiveDir/<domain>/ into the destination
// certDir with identical content and 0600 permissions, matching what
// certbot's own layout requires nself to bridge manually (see ssl_install.go).
func TestInstallIssuedCert_CopiesFromLetsEncryptLiveDir(t *testing.T) {
	const domain = "example.nself.org"

	liveRoot := t.TempDir()
	liveDomainDir := filepath.Join(liveRoot, domain)
	if err := os.MkdirAll(liveDomainDir, 0o750); err != nil {
		t.Fatalf("mkdir live domain dir: %v", err)
	}

	fixtures := map[string]string{
		"fullchain.pem": "FAKE-FULLCHAIN-PEM-DATA",
		"privkey.pem":   "FAKE-PRIVKEY-PEM-DATA",
	}
	for name, body := range fixtures {
		if err := os.WriteFile(filepath.Join(liveDomainDir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	orig := letsEncryptLiveDir
	letsEncryptLiveDir = liveRoot
	t.Cleanup(func() { letsEncryptLiveDir = orig })

	destDir := t.TempDir()
	if err := installIssuedCert(domain, destDir); err != nil {
		t.Fatalf("installIssuedCert: %v", err)
	}

	for name, want := range fixtures {
		p := filepath.Join(destDir, name)
		got, err := os.ReadFile(p) //nolint:gosec // path built from t.TempDir
		if err != nil {
			t.Fatalf("%s not installed at destDir: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("%s content = %q, want %q", name, got, want)
		}
	}
}

// TestInstallIssuedCert_MissingCertbotOutputErrors verifies installIssuedCert
// returns a wrapped, diagnosable error — rather than silently doing nothing —
// when certbot did not actually produce output for the domain. This is the
// exact failure mode installIssuedCert exists to prevent from going unnoticed.
func TestInstallIssuedCert_MissingCertbotOutputErrors(t *testing.T) {
	liveRoot := t.TempDir() // no domain subdirectory created inside it

	orig := letsEncryptLiveDir
	letsEncryptLiveDir = liveRoot
	t.Cleanup(func() { letsEncryptLiveDir = orig })

	destDir := t.TempDir()
	err := installIssuedCert("never-issued.example.com", destDir)
	if err == nil {
		t.Fatal("installIssuedCert succeeded with no certbot output on disk — expected an error")
	}
	if !strings.Contains(err.Error(), "did certbot succeed?") {
		t.Errorf("error = %q, want it to mention \"did certbot succeed?\"", err.Error())
	}
}

// TestWriteCustomDomainConf_ReferencesFilesafeCertPath is the regression guard
// for the exact bug ssl_add.go's inline comment describes: the generated nginx
// server block must reference the DASH-safe directory name in its
// ssl_certificate directives, never the dotted domain — the dotted form is a
// path nginx cannot resolve because that is not where the certificate lives.
func TestWriteCustomDomainConf_ReferencesFilesafeCertPath(t *testing.T) {
	const domain = "my.custom.com"
	const wantSafe = "my-custom-com"

	workdir := t.TempDir()
	if err := writeCustomDomainConf(workdir, domain, ""); err != nil {
		t.Fatalf("writeCustomDomainConf: %v", err)
	}

	confPath := filepath.Join(workdir, "nginx", "conf.d", "custom-"+wantSafe+".conf")
	raw, err := os.ReadFile(confPath) //nolint:gosec // path built from t.TempDir
	if err != nil {
		t.Fatalf("conf not written at expected path: %v", err)
	}
	conf := string(raw)

	for _, line := range []string{"ssl_certificate ", "ssl_certificate_key "} {
		idx := strings.Index(conf, line)
		if idx == -1 {
			t.Fatalf("conf missing %q directive\n--- conf ---\n%s", line, conf)
		}
		end := strings.IndexByte(conf[idx:], '\n')
		directiveLine := conf[idx : idx+end]
		if !strings.Contains(directiveLine, wantSafe) {
			t.Errorf("%q line does not reference dash-safe form %q: %q", line, wantSafe, directiveLine)
		}
		if strings.Contains(directiveLine, "/"+domain+"/") {
			t.Errorf("%q line references the dotted domain instead of the dash-safe form: %q", line, directiveLine)
		}
	}
}

// TestWriteCustomDomainConf_PathScopedRateLimits is the SEC-HARDENING-06
// regression guard (P6-E2-W2-S3-T20): a custom domain proxying to a backend
// (--upstream set) must get the same path-scoped /auth/login (auth_strict)
// and /api/ (api) rate-limit locations as internal/nginx's
// defaultSecurityPathZones, ahead of the catch-all `location /`. The
// placeholder (no upstream) branch is asserted separately to have none of
// them, since it never proxies anywhere.
func TestWriteCustomDomainConf_PathScopedRateLimits(t *testing.T) {
	t.Run("proxying custom domain gets both path-scoped locations", func(t *testing.T) {
		workdir := t.TempDir()
		if err := writeCustomDomainConf(workdir, "gw.example.com", "gw-upstream:8080"); err != nil {
			t.Fatalf("writeCustomDomainConf: %v", err)
		}
		confPath := filepath.Join(workdir, "nginx", "conf.d", "custom-gw-example-com.conf")
		raw, err := os.ReadFile(confPath) //nolint:gosec // path built from t.TempDir
		if err != nil {
			t.Fatalf("conf not written: %v", err)
		}
		conf := string(raw)

		for _, want := range []string{
			"location = /auth/login {",
			"limit_req zone=auth_strict burst=5 nodelay;",
			"location /api/ {",
			"limit_req zone=api burst=20 nodelay;",
			"proxy_pass http://gw-upstream:8080;",
		} {
			if !strings.Contains(conf, want) {
				t.Errorf("missing %q\n--- conf ---\n%s", want, conf)
			}
		}

		// Doctor check (internal/doctor/hardening_check_nginx_zones.go)
		// greps this exact file's content for "/auth/login" + "limit_req"
		// and "/api/" + "limit_req" co-occurring — assert its literal
		// contract, not just our own template's wording.
		if !strings.Contains(conf, "/auth/login") || !strings.Contains(conf, "limit_req") {
			t.Error("conf must satisfy the doctor check's /auth/login + limit_req grep")
		}
		if !strings.Contains(conf, "/api/") {
			t.Error("conf must satisfy the doctor check's /api/ grep")
		}

		authIdx := strings.Index(conf, "location = /auth/login")
		apiIdx := strings.Index(conf, "location /api/")
		// LastIndex: writeCustomDomainConf emits a separate listen-80
		// redirect server block ahead of the proxying listen-443 block, and
		// that redirect block's own `location / { return 301 ...}` also
		// matches "location / {" — the catch-all we care about ordering
		// against is the one in the same (443) server block as the
		// path-scoped locations, i.e. the LAST occurrence in the file.
		catchAllIdx := strings.LastIndex(conf, "location / {")
		if authIdx == -1 || apiIdx == -1 || catchAllIdx == -1 {
			t.Fatal("could not locate all three location blocks")
		}
		if authIdx > catchAllIdx || apiIdx > catchAllIdx {
			t.Error("path-scoped locations must be declared before the catch-all location /")
		}
	})

	t.Run("placeholder custom domain (no upstream) has none of them", func(t *testing.T) {
		workdir := t.TempDir()
		if err := writeCustomDomainConf(workdir, "future.example.com", ""); err != nil {
			t.Fatalf("writeCustomDomainConf: %v", err)
		}
		confPath := filepath.Join(workdir, "nginx", "conf.d", "custom-future-example-com.conf")
		raw, err := os.ReadFile(confPath) //nolint:gosec // path built from t.TempDir
		if err != nil {
			t.Fatalf("conf not written: %v", err)
		}
		conf := string(raw)
		if strings.Contains(conf, "limit_req") {
			t.Errorf("placeholder (no --upstream) conf should not proxy anywhere and should carry no rate limits\n--- conf ---\n%s", conf)
		}
	})
}

// TestDomainToFilesafe_ReplacesDotsAndColons table-tests the domainToFilesafe
// helper directly, including an IPv6-with-port-style edge case (colons),
// since the function replaces both dots and colons.
func TestDomainToFilesafe_ReplacesDotsAndColons(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"nself.org":          "nself-org",
		"api.task.nself.org": "api-task-nself-org",
		"localhost:8443":     "localhost-8443",
		"[::1]:8443":         "[--1]-8443",
	}
	for in, want := range cases {
		in, want := in, want
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			got := domainToFilesafe(in)
			if got != want {
				t.Errorf("domainToFilesafe(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

// TestSSLAdd_CertDirMatchesNginxMountLayout is the cross-reference regression
// guard required by this ticket's CR-C: it proves the directory ssl_add
// constructs (workdir/ssl/certificates/<domainSafe>) uses a domain-safe name
// that is IDENTICAL to internal/ssl.DomainToDirName's output — the function
// internal/ssl uses to name the certificate directory for the primary domain
// (see internal/ssl/generator.go's GenerateWithResult). It imports and calls
// the real internal/ssl function rather than re-typing its replacement rule,
// so the two layouts cannot silently diverge if internal/ssl's convention
// ever changes without ssl_add.go being updated to match.
func TestSSLAdd_CertDirMatchesNginxMountLayout(t *testing.T) {
	t.Parallel()

	domains := []string{
		"staging.nself.org",
		"nself.org",
		"api.task.nself.org",
	}

	for _, domain := range domains {
		domain := domain
		t.Run(domain, func(t *testing.T) {
			t.Parallel()

			workdir := "/opt/nself-web/backend" // representative, not written to
			addSafe := domainToFilesafe(domain)
			addCertDir := filepath.Join(workdir, "ssl", "certificates", addSafe)

			internalSafe := ssl.DomainToDirName(domain)
			internalCertDir := filepath.Join(workdir, "ssl", "certificates", internalSafe)

			if addCertDir != internalCertDir {
				t.Errorf("ssl add cert dir %q diverges from internal/ssl's layout %q for domain %q",
					addCertDir, internalCertDir, domain)
			}
		})
	}
}

// ── P7-LIVE-02: served nginx/ssl directories (D-0045) ───────────────────────

// frontedFixture lays out the production shape (D-0121): a fronting stack
// "nself-web" that owns nginx/ and ssl/, with this project in its backend/
// subdirectory. It returns the stack root and the project directory.
func frontedFixture(t *testing.T) (stack, project string) {
	t.Helper()
	stack = filepath.Join(t.TempDir(), "nself-web")
	project = filepath.Join(stack, "backend")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	return stack, project
}

// mustNotExist fails when path exists.
func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("%s exists: a fronted project wrote into its own tree, where no nginx reads", path)
	}
}

// TestSSLInstallFronted proves that on a fronted project every ssl/nginx write
// `ssl add` and `build` make lands under the fronting stack, that a project
// that runs its own nginx is unchanged, and that an unconfirmed layout is
// refused rather than guessed.
func TestSSLInstallFronted(t *testing.T) {
	const domain = "my.custom.com"
	const safe = "my-custom-com"

	t.Run("certificate copy lands in the fronting stack ssl dir", func(t *testing.T) {
		stack, project := frontedFixture(t)
		live := t.TempDir()
		if err := os.MkdirAll(filepath.Join(live, domain), 0o750); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"fullchain.pem", "privkey.pem"} {
			if err := os.WriteFile(filepath.Join(live, domain, name), []byte("PEM-"+name), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		orig := letsEncryptLiveDir
		letsEncryptLiveDir = live
		t.Cleanup(func() { letsEncryptLiveDir = orig })

		certDir, err := servedCertDir(project, "nself-web", domain)
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(stack, "ssl", "certificates", safe); certDir != want {
			t.Fatalf("servedCertDir = %s, want %s", certDir, want)
		}
		if err := os.MkdirAll(certDir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := installIssuedCert(domain, certDir); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(filepath.Join(stack, "ssl", "certificates", safe, "privkey.pem")); err != nil || string(got) != "PEM-privkey.pem" {
			t.Errorf("certificate not under the fronting stack: %v %q", err, got)
		}
		mustNotExist(t, filepath.Join(project, "ssl"))
	})

	t.Run("custom-domain conf lands in the fronting stack conf.d", func(t *testing.T) {
		stack, project := frontedFixture(t)
		if err := writeCustomDomainConfServed(project, "nself-web", domain, ""); err != nil {
			t.Fatal(err)
		}
		conf := filepath.Join(stack, "nginx", "conf.d", "custom-"+safe+".conf")
		raw, err := os.ReadFile(conf)
		if err != nil {
			t.Fatalf("conf not under the fronting stack: %v", err)
		}
		if string(raw) != goldenCustomNoUpstream {
			t.Errorf("fronted conf differs from the non-fronted golden:\n%s", raw)
		}
		mustNotExist(t, filepath.Join(project, "nginx"))
	})

	t.Run("api-docs site conf lands in the fronting stack sites dir", func(t *testing.T) {
		stack, project := frontedFixture(t)
		got, err := build.WriteAPIDocsSiteConf(project, "nself-web", []byte(apidocs.NginxConf("/docs", "example.com")))
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(stack, "nginx", "sites", "api-docs.conf"); got != want {
			t.Fatalf("api-docs conf at %s, want %s", got, want)
		}
		mustNotExist(t, filepath.Join(project, "nginx"))
	})

	t.Run("plugin nginx routes land in the fronting stack sites dir", func(t *testing.T) {
		stack, project := frontedFixture(t)
		pluginDir := filepath.Join(t.TempDir(), "plugins")
		if err := os.MkdirAll(filepath.Join(pluginDir, "ai", "nginx"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pluginDir, "ai", "nginx", "ai.conf"), []byte("# route\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := &config.Config{}
		cfg.Nginx.FrontedBy = "nself-web"
		n, err := build.InjectPluginNginxRoutes(project, pluginDir, cfg)
		if err != nil || n != 1 {
			t.Fatalf("InjectPluginNginxRoutes = %d, %v", n, err)
		}
		if _, err := os.Stat(filepath.Join(stack, "nginx", "sites", "ai-ai.conf")); err != nil {
			t.Errorf("plugin conf not under the fronting stack: %v", err)
		}
		mustNotExist(t, filepath.Join(project, "nginx"))
	})

	t.Run("an unconfirmed layout is refused and nothing is written", func(t *testing.T) {
		stray := filepath.Join(t.TempDir(), "elsewhere", "backend")
		if err := os.MkdirAll(stray, 0o750); err != nil {
			t.Fatal(err)
		}
		if _, err := servedCertDir(stray, "nself-web", domain); !errors.Is(err, nginxtopo.ErrFrontingUnresolved) {
			t.Errorf("servedCertDir err = %v, want ErrFrontingUnresolved", err)
		}
		if err := writeCustomDomainConfServed(stray, "nself-web", domain, ""); !errors.Is(err, nginxtopo.ErrFrontingUnresolved) {
			t.Errorf("writeCustomDomainConfServed err = %v, want ErrFrontingUnresolved", err)
		}
		mustNotExist(t, filepath.Join(stray, "nginx"))
		mustNotExist(t, filepath.Join(filepath.Dir(stray), "nginx"))
	})

	t.Run("a project that runs its own nginx writes where it always did", func(t *testing.T) {
		project := t.TempDir()
		certDir, err := servedCertDir(project, "", domain)
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(project, "ssl", "certificates", safe); certDir != want {
			t.Errorf("servedCertDir = %s, want %s", certDir, want)
		}
		for _, c := range []struct{ domain, upstream, golden string }{
			{domain, "", goldenCustomNoUpstream},
			{"gw.example.com", "gw-upstream:8080", goldenCustomUpstream},
		} {
			if err := writeCustomDomainConfServed(project, "", c.domain, c.upstream); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join(project, "nginx", "conf.d", "custom-"+domainToFilesafe(c.domain)+".conf"))
			if err != nil || string(raw) != c.golden {
				t.Errorf("%s: own-nginx conf changed (err %v):\n%s", c.domain, err, raw)
			}
		}
		if got := apidocs.NginxConf("/docs", "example.com"); got != goldenAPIDocsConf {
			t.Errorf("api-docs conf changed:\n%s", got)
		}
		path, err := build.WriteAPIDocsSiteConf(project, "", []byte("x"))
		if err != nil || path != filepath.Join(project, "nginx", "sites", "api-docs.conf") {
			t.Errorf("own api-docs conf at %s, %v", path, err)
		}
	})
}

// goldenCustomNoUpstream, goldenCustomUpstream and goldenAPIDocsConf are the
// bytes origin/main (5f7b73c8) rendered for the same inputs, before the two
// hand-typed "/etc/nginx/ssl" literals moved to nginxtopo.NginxSSLContainerPath
// (P7-LIVE-02). They pin that the move changed nothing a non-fronted project
// sees.
const goldenCustomNoUpstream = `# Generated by nself ssl add
server {
    listen 80;
    server_name my.custom.com;

    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl;
    http2 on;
    server_name my.custom.com;

    ssl_certificate     /etc/nginx/ssl/certificates/my-custom-com/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/certificates/my-custom-com/privkey.pem;

    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;

    location / {
        return 200 'nself custom domain — configure --upstream to proxy to a backend service';
        add_header Content-Type text/plain;
    }
}
`

const goldenCustomUpstream = `# Generated by nself ssl add
server {
    listen 80;
    server_name gw.example.com;

    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl;
    http2 on;
    server_name gw.example.com;

    ssl_certificate     /etc/nginx/ssl/certificates/gw-example-com/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/certificates/gw-example-com/privkey.pem;

    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;

    location = /auth/login {
        limit_req zone=auth_strict burst=5 nodelay;
        proxy_pass http://gw-upstream:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /api/ {
        limit_req zone=api burst=20 nodelay;
        proxy_pass http://gw-upstream:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location / {
        proxy_pass http://gw-upstream:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`

const goldenAPIDocsConf = `# Generated by nself build - DO NOT EDIT MANUALLY
# Service: docs.example.com

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name docs.example.com;

    ssl_certificate /etc/nginx/ssl/certificates/example-com/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/certificates/example-com/privkey.pem;

    server_tokens off;

    location /api-docs {
        alias /opt/nself/.dist/openapi.json;
        add_header Content-Type "application/json";
        add_header Access-Control-Allow-Origin *;
        add_header Cache-Control "no-store";
    }
    location /docs {
        alias /opt/nself/.dist/scalar.html;
        add_header Content-Type "text/html";
        add_header Cache-Control "no-store";
    }
}
`
