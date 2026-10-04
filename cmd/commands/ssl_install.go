package commands

// Purpose: Installs a certbot-issued certificate where nginx actually
// reads it, and generates the nginx server-block config for a custom
// domain. Split out of ssl_setup.go (CLI-R12) to separate these
// file-writing helpers from the `ssl setup`/`ssl add` command handlers
// (ssl_setup.go, ssl_add.go) and the renewal-hook installers
// (ssl_renewal.go).
// Inputs: a domain name, a cert directory, and (for the nginx writer) the
// project workdir and upstream service name.
// Outputs: copied cert/key files at 0600 and a written nginx conf file.
// Constraints: letsEncryptLiveDir is a var (not const) specifically so tests
// can point it at a temp dir — keep it that way. Where the certificate and
// the conf are written is decided by internal/nginxtopo's served resolver
// (P7-LIVE-02, D-0045): a project with NGINX_FRONTED_BY set writes into the
// fronting stack's ssl/ and nginx/ trees, the ones its nginx actually reads;
// every other project writes where it always did.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/nginxtopo"
)

// letsEncryptLiveDir is where certbot stores issued certificates. Declared as a
// variable so tests can point it at a temp dir.
var letsEncryptLiveDir = "/etc/letsencrypt/live"

// installIssuedCert copies the certificate certbot just issued for domain into
// certDir, which is the directory the generated nginx server block references.
//
// certbot `certonly` always writes to /etc/letsencrypt/live/<domain>/ and
// ignores --cert-path/--key-path, so without this step the certificate is
// issued but invisible to nginx. Files are written 0600 because privkey.pem is
// a private key; the containing directory is created by the caller.
//
// Copies rather than symlinks: /etc/letsencrypt/live entries are themselves
// symlinks into ../archive, and a bind-mounted symlink chain does not resolve
// inside the nginx container.
func installIssuedCert(domain, certDir string) error {
	liveDir := filepath.Join(letsEncryptLiveDir, domain)

	for _, name := range []string{"fullchain.pem", "privkey.pem"} {
		src := filepath.Join(liveDir, name)
		data, err := os.ReadFile(src) //nolint:gosec // path derived from the validated domain
		if err != nil {
			return fmt.Errorf("reading %s (did certbot succeed?): %w", src, err)
		}
		dst := filepath.Join(certDir, name)
		if err := os.WriteFile(dst, data, 0600); err != nil {
			return fmt.Errorf("writing %s: %w", dst, err)
		}
	}
	return nil
}

// servedCertDir returns the directory a custom domain's certificate belongs
// in: <served ssl dir>/certificates/<domain-safe>. frontedBy is
// NGINX_FRONTED_BY ("" for a project that runs its own nginx). An unconfirmed
// fronted layout returns an error wrapping nginxtopo.ErrFrontingUnresolved.
func servedCertDir(workdir, frontedBy, domain string) (string, error) {
	sslDir, err := nginxtopo.ServedSSLDir(workdir, frontedBy)
	if err != nil {
		return "", err
	}
	return filepath.Join(sslDir, "certificates", domainToFilesafe(domain)), nil
}

// servedConfDir returns the served nginx conf.d directory custom-domain confs
// are written to. Same resolution and error as servedCertDir.
func servedConfDir(workdir, frontedBy string) (string, error) {
	nginxDir, err := nginxtopo.ServedNginxDir(workdir, frontedBy)
	if err != nil {
		return "", err
	}
	return filepath.Join(nginxDir, "conf.d"), nil
}

// writeCustomDomainConf writes the custom-domain conf for a project that runs
// its own nginx (NGINX_FRONTED_BY unset). `ssl add` calls
// writeCustomDomainConfServed with the project's real setting.
func writeCustomDomainConf(workdir, domain, upstream string) error {
	return writeCustomDomainConfServed(workdir, "", domain, upstream)
}

// writeCustomDomainConfServed generates an nginx server block for domain and
// writes it to custom-{domain-safe}.conf in the served nginx conf.d directory
// (servedConfDir): <workdir>/nginx/conf.d, or the fronting stack's when
// frontedBy names one.
// When upstream is non-empty the server block proxy_passes to it; otherwise it
// returns a 200 informational response until --upstream is configured.
//
// server_name keeps the real dotted domain, but ssl_certificate must point at
// ssl/certificates/{domain-safe} — the layout internal/ssl writes and that
// compose mounts at /etc/nginx/ssl. Using the dotted domain here produced a
// path nginx could not resolve even once the conf was in place.
func writeCustomDomainConfServed(workdir, frontedBy, domain, upstream string) error {
	confDir, err := servedConfDir(workdir, frontedBy)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(confDir, 0750); err != nil {
		return fmt.Errorf("creating %s: %w", confDir, err)
	}

	domainSafe := domainToFilesafe(domain)
	confPath := filepath.Join(confDir, fmt.Sprintf("custom-%s.conf", domainSafe))

	var locationBlock string
	if upstream != "" {
		// SEC-HARDENING-06: path-scoped rate limits ahead of the catch-all
		// `location /`, same zones/bursts as service.conf.tmpl's
		// defaultSecurityPathZones (internal/nginx/generator.go). Applied
		// unconditionally like the generator does — writeCustomDomainConf
		// has no signal for whether this custom domain's upstream serves
		// auth/API paths, and the blocks are harmless when it doesn't (they
		// just proxy through with a stricter limit on paths the upstream
		// never receives). Only rendered in the proxying branch: the
		// placeholder branch below never proxies to a backend, so there is
		// nothing on those paths to rate-limit.
		locationBlock = fmt.Sprintf(`    location = /auth/login {
        limit_req zone=auth_strict burst=5 nodelay;
        proxy_pass http://%[1]s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /api/ {
        limit_req zone=api burst=20 nodelay;
        proxy_pass http://%[1]s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location / {
        proxy_pass http://%[1]s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }`, upstream)
	} else {
		locationBlock = `    location / {
        return 200 'nself custom domain — configure --upstream to proxy to a backend service';
        add_header Content-Type text/plain;
    }`
	}

	conf := fmt.Sprintf(`# Generated by nself ssl add
server {
    listen 80;
    server_name %s;

    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl;
    http2 on;
    server_name %s;

    ssl_certificate     %s/certificates/%s/fullchain.pem;
    ssl_certificate_key %s/certificates/%s/privkey.pem;

    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;

%s
}
`, domain, domain, nginxtopo.NginxSSLContainerPath, domainSafe, nginxtopo.NginxSSLContainerPath, domainSafe, locationBlock)

	if err := os.WriteFile(confPath, []byte(conf), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", confPath, err)
	}
	return nil
}
