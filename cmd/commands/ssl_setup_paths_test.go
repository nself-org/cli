package commands

// ssl_setup_paths_test.go — Regression tests for `nself ssl add` path agreement.
//
// Purpose: Lock in that the host directory certbot writes into and the container
//          path the generated nginx server block references describe the SAME
//          certificate. They previously disagreed in two independent ways —
//          the "certificates/" path segment was missing, and the directory was
//          named with the dotted domain instead of the dash-safe form — so
//          `ssl add` reported success while nginx kept serving the self-signed
//          wildcard.
// Inputs:  domain strings; a temp dir standing in for the project root.
// Outputs: assertions on the written conf and the cert directory layout.
// Constraints: No certbot, docker, or network. Pure filesystem + string checks.
//
// Compose mounts "./ssl:/etc/nginx/ssl:ro", so host <workdir>/ssl/X must be
// visible to nginx as /etc/nginx/ssl/X. Any test here that fails means a cert
// would be issued but unreadable by nginx.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDomainToFilesafe_MatchesInternalSSLConvention pins the naming used for
// certificate directories. internal/ssl writes certificates to
// certificates/<domain with dots replaced by dashes>; ssl add must agree.
func TestDomainToFilesafe_MatchesInternalSSLConvention(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"stack-two.nself.org":   "stack-two-nself-org",
		"nself.org":             "nself-org",
		"api.task.nself.org":    "api-task-nself-org",
		"localhost:8443":        "localhost-8443",
		"a.b.c.d.example.co.uk": "a-b-c-d-example-co-uk",
	}
	for in, want := range cases {
		in, want := in, want
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			got := domainToFilesafe(in)
			if got != want {
				t.Errorf("domainToFilesafe(%q) = %q, want %q", in, got, want)
			}
			if strings.Contains(got, ".") {
				t.Errorf("domainToFilesafe(%q) = %q still contains a dot", in, got)
			}
		})
	}
}

// TestWriteCustomDomainConf_CertPathMatchesHostLayout is the core regression
// test. It asserts the conf points at /etc/nginx/ssl/certificates/<safe>/,
// which is exactly where the host directory <workdir>/ssl/certificates/<safe>/
// surfaces inside the nginx container.
func TestWriteCustomDomainConf_CertPathMatchesHostLayout(t *testing.T) {
	t.Parallel()

	const domain = "stack-two.nself.org"
	safe := domainToFilesafe(domain)
	workdir := t.TempDir()

	if err := writeCustomDomainConf(workdir, domain, ""); err != nil {
		t.Fatalf("writeCustomDomainConf: %v", err)
	}

	confPath := filepath.Join(workdir, "nginx", "conf.d", fmt.Sprintf("custom-%s.conf", safe))
	raw, err := os.ReadFile(confPath) //nolint:gosec // path built from t.TempDir
	if err != nil {
		t.Fatalf("conf not written where ssl add reports it: %v", err)
	}
	conf := string(raw)

	wantCert := fmt.Sprintf("/etc/nginx/ssl/certificates/%s/fullchain.pem", safe)
	wantKey := fmt.Sprintf("/etc/nginx/ssl/certificates/%s/privkey.pem", safe)
	for _, want := range []string{wantCert, wantKey} {
		if !strings.Contains(conf, want) {
			t.Errorf("conf missing %q\n--- conf ---\n%s", want, conf)
		}
	}

	// The two historical bugs, asserted directly so a regression names itself.
	if strings.Contains(conf, "/etc/nginx/ssl/"+domain+"/") {
		t.Error("conf references the dotted domain directory; must use the dash-safe name")
	}
	if strings.Contains(conf, "ssl_certificate     /etc/nginx/ssl/"+safe) {
		t.Error("conf omits the 'certificates/' path segment")
	}
}

// TestWriteCustomDomainConf_ServerNameStaysDotted guards the other direction:
// only the certificate PATH is dash-safe. server_name must remain the real
// domain or nginx will never match the request.
func TestWriteCustomDomainConf_ServerNameStaysDotted(t *testing.T) {
	t.Parallel()

	const domain = "stack-two.nself.org"
	workdir := t.TempDir()

	if err := writeCustomDomainConf(workdir, domain, ""); err != nil {
		t.Fatalf("writeCustomDomainConf: %v", err)
	}
	confPath := filepath.Join(workdir, "nginx", "conf.d",
		fmt.Sprintf("custom-%s.conf", domainToFilesafe(domain)))
	raw, err := os.ReadFile(confPath) //nolint:gosec // path built from t.TempDir
	if err != nil {
		t.Fatalf("read conf: %v", err)
	}
	conf := string(raw)

	if !strings.Contains(conf, "server_name "+domain+";") {
		t.Errorf("server_name must be the dotted domain %q\n--- conf ---\n%s", domain, conf)
	}
	if strings.Contains(conf, "server_name "+domainToFilesafe(domain)+";") {
		t.Error("server_name was written in dash-safe form; nginx would never match the host")
	}
}

// TestWriteCustomDomainConf_UpstreamProxies verifies the --upstream branch still
// produces a proxy_pass rather than the placeholder response.
func TestWriteCustomDomainConf_UpstreamProxies(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	if err := writeCustomDomainConf(workdir, "app.example.com", "app:3000"); err != nil {
		t.Fatalf("writeCustomDomainConf: %v", err)
	}
	confPath := filepath.Join(workdir, "nginx", "conf.d", "custom-app-example-com.conf")
	raw, err := os.ReadFile(confPath) //nolint:gosec // path built from t.TempDir
	if err != nil {
		t.Fatalf("read conf: %v", err)
	}
	conf := string(raw)

	if !strings.Contains(conf, "proxy_pass http://app:3000;") {
		t.Errorf("expected proxy_pass to the upstream\n--- conf ---\n%s", conf)
	}
	if strings.Contains(conf, "configure --upstream") {
		t.Error("placeholder response emitted even though an upstream was supplied")
	}
}

// TestInstallIssuedCert_CopiesFromLetsEncryptLive covers the step that was
// missing entirely: certbot `certonly` writes to /etc/letsencrypt/live/<domain>/
// and ignores --cert-path, so the certificate has to be copied into the tree
// nginx reads or `ssl add` is a silent no-op.
func TestInstallIssuedCert_CopiesFromLetsEncryptLive(t *testing.T) {
	const domain = "stack-two.nself.org"

	root := t.TempDir()
	live := filepath.Join(root, "live", domain)
	if err := os.MkdirAll(live, 0o750); err != nil {
		t.Fatalf("mkdir live: %v", err)
	}
	for name, body := range map[string]string{
		"fullchain.pem": "FULLCHAIN-CONTENT",
		"privkey.pem":   "PRIVKEY-CONTENT",
	} {
		if err := os.WriteFile(filepath.Join(live, name), []byte(body), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	orig := letsEncryptLiveDir
	letsEncryptLiveDir = filepath.Join(root, "live")
	t.Cleanup(func() { letsEncryptLiveDir = orig })

	certDir := filepath.Join(root, "ssl", "certificates", domainToFilesafe(domain))
	if err := os.MkdirAll(certDir, 0o750); err != nil {
		t.Fatalf("mkdir certDir: %v", err)
	}
	if err := installIssuedCert(domain, certDir); err != nil {
		t.Fatalf("installIssuedCert: %v", err)
	}

	for name, want := range map[string]string{
		"fullchain.pem": "FULLCHAIN-CONTENT",
		"privkey.pem":   "PRIVKEY-CONTENT",
	} {
		p := filepath.Join(certDir, name)
		got, err := os.ReadFile(p) //nolint:gosec // temp dir
		if err != nil {
			t.Fatalf("%s not installed: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		// privkey.pem is a private key; 0600 is required, not cosmetic.
		//
		// Windows has no Unix permission bits: Go's os.WriteFile maps the mode
		// onto a read-only flag only, so a file written 0600 stats back as 0666
		// there. The production write in ssl_setup.go is correct and unchanged;
		// only the assertion is unportable, so it stays strict everywhere the
		// bits actually exist.
		if runtime.GOOS != "windows" {
			if perm := fi.Mode().Perm(); perm != 0o600 {
				t.Errorf("%s mode = %o, want 600", name, perm)
			}
		}
	}
}

// TestInstallIssuedCert_MissingLineageErrors ensures a failed/absent issuance is
// reported instead of leaving nginx pointed at a cert that never arrived.
func TestInstallIssuedCert_MissingLineageErrors(t *testing.T) {
	root := t.TempDir()
	orig := letsEncryptLiveDir
	letsEncryptLiveDir = filepath.Join(root, "live")
	t.Cleanup(func() { letsEncryptLiveDir = orig })

	certDir := filepath.Join(root, "dest")
	if err := os.MkdirAll(certDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := installIssuedCert("absent.example.com", certDir); err == nil {
		t.Fatal("expected an error when the lineage directory does not exist")
	}
}

// TestSSLRenewalServiceUnit_RenewsIntoNginxTree guards the renewal path: without
// a deploy-hook the served certificate silently expires ~90 days after issue,
// and without WorkingDirectory the compose reload cannot find the project.
func TestSSLRenewalServiceUnit_RenewsIntoNginxTree(t *testing.T) {
	t.Parallel()

	const workdir = "/opt/nself-web/backend"
	unit := sslRenewalServiceUnit(workdir)

	for _, want := range []string{
		"WorkingDirectory=" + workdir,
		"--deploy-hook",
		workdir + "/ssl/certificates/$safe",
		"install -m 600",
		"--post-hook",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("renewal unit missing %q\n--- unit ---\n%s", want, unit)
		}
	}

	// The hook must derive the same dash-safe name installIssuedCert uses.
	if !strings.Contains(unit, `tr '.:' '--'`) {
		t.Errorf("renewal unit does not dash-normalise the lineage name\n--- unit ---\n%s", unit)
	}
}
