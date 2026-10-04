package commands

// ssl_acme_adopt_test.go: adopting a certbot tree (one dns-cloudflare and one
// standalone lineage, both served by the fixture's nginx confs). Adoption must
// never touch certbot state, served files, confs or the container, and never issue.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/ssl/acme"
)

// adoptFix builds the certbot tree and the served confs and certificates.
func adoptFix(t *testing.T, f *acmeFix) (letsencrypt string) {
	t.Helper()
	letsencrypt = filepath.Join(f.root, "letsencrypt")
	for name, auth := range map[string]string{"api.task.nself.org": "dns-cloudflare", "auth.task.nself.org": "standalone"} {
		cert, key := acmeTestCert(t, []string{name}, time.Now().Add(20*24*time.Hour))
		dir := strings.ReplaceAll(name, ".", "-")
		acmeWriteFile(t, filepath.Join(letsencrypt, "renewal", name+".conf"),
			[]byte("version = 2.9.0\n[renewalparams]\nauthenticator = "+auth+"\npost_hook = docker stop nginx\n"))
		for _, p := range []string{filepath.Join(letsencrypt, "live", name), filepath.Join(f.ssl, "certificates", dir)} {
			acmeWriteFile(t, filepath.Join(p, "fullchain.pem"), cert)
			acmeWriteFile(t, filepath.Join(p, "privkey.pem"), key)
		}
		acmeWriteFile(t, filepath.Join(letsencrypt, "live", name, "cert.pem"), cert)
		acmeWriteFile(t, filepath.Join(f.served, "nginx", "conf.d", dir+".conf"), []byte(fmt.Sprintf(
			"server {\n  listen 443 ssl;\n  server_name %s;\n  ssl_certificate /etc/nginx/ssl/certificates/%s/fullchain.pem;\n  ssl_certificate_key /etc/nginx/ssl/certificates/%s/privkey.pem;\n}\n", name, dir, dir)))
	}
	return letsencrypt
}

func TestACMEAdoptStandalone(t *testing.T) {
	f := newACMEFix(t)
	le := adoptFix(t, f)
	cred := filepath.Join(f.root, "cloudflare.ini")
	acmeWriteFile(t, cred, []byte("# certbot\ndns_cloudflare_api_token = cf-token-0123456789abcdef\n"))

	// Without a credential the standalone lineage is refused, with a remediation, and nothing is written.
	before := acmeTree(t, f.root)
	out, err := f.run(sslSetupCmd, "--acme", "--adopt-certbot="+le)
	if err == nil || !strings.Contains(out, "refused auth.task.nself.org") || !strings.Contains(out, "--dns-credential-file") {
		t.Fatalf("standalone without a credential: err=%v\n%s", err, out)
	}
	if acmeTree(t, f.root) != before || len(f.set) != 0 {
		t.Fatal("a refused adoption wrote something")
	}

	// A dry run prints the mapping and writes nothing.
	out, err = f.run(sslSetupCmd, "--acme", "--adopt-certbot="+le, "--dns-credential-file="+cred, "--dry-run")
	if err != nil || !strings.Contains(out, "certificates/api-task-nself-org") || !strings.Contains(out, "dns-01 via cloudflare") {
		t.Fatalf("dry run: err=%v\n%s", err, out)
	}
	if acmeTree(t, f.root) != before || len(f.set) != 0 {
		t.Fatal("dry run wrote something")
	}

	// The real run converts both lineages to dns-01.
	served0 := acmeTree(t, filepath.Join(f.ssl, "certificates"))
	confs0 := acmeTree(t, filepath.Join(f.served, "nginx"))
	le0 := acmeTree(t, le)
	out, err = f.run(sslSetupCmd, "--acme", "--adopt-certbot="+le, "--dns-credential-file="+cred)
	if err != nil {
		t.Fatalf("adopt: %v\n%s", err, out)
	}
	lf, _ := acme.Load(f.ssl)
	if len(lf.Lineages) != 2 {
		t.Fatalf("lineages = %+v", lf.Lineages)
	}
	for _, l := range lf.Lineages {
		if l.Challenge != "dns-01" || l.DNSProvider != "cloudflare" || l.AdoptedFrom == nil || l.LastIssued != nil ||
			len(l.Targets) != 1 || l.Targets[0] != "certificates/"+strings.ReplaceAll(l.Name, ".", "-") ||
			len(l.CredentialSecrets) != 1 || l.CredentialSecrets[0] != "SSL_DNS_CLOUDFLARE_API_TOKEN" {
			t.Errorf("lineage %+v", l)
		}
	}
	if f.set["SSL_DNS_CLOUDFLARE_API_TOKEN"] != "cf-token-0123456789abcdef" || len(f.set) != 1 {
		t.Errorf("secret store writes = %v", f.set)
	}
	if acmeTree(t, filepath.Join(f.ssl, "certificates")) != served0 || acmeTree(t, filepath.Join(f.served, "nginx")) != confs0 || acmeTree(t, le) != le0 {
		t.Error("served files, nginx confs or certbot state changed")
	}
	if len(f.runs) != 0 || len(f.execs) != 0 {
		t.Errorf("adoption issued or reloaded: runs=%d execs=%v", len(f.runs), f.execs)
	}
	if strings.Contains(out, "cf-token") {
		t.Error("credential printed")
	}

	// A target that is missing is imported from certbot's live files.
	_ = os.RemoveAll(filepath.Join(f.ssl, "certificates", "api-task-nself-org"))
	_ = os.RemoveAll(filepath.Join(f.ssl, ".acme"))
	if out, err = f.run(sslSetupCmd, "--acme", "--adopt-certbot="+le, "--dns-credential-file="+cred, "--lineage=api.task.nself.org"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if link, _ := os.Readlink(filepath.Join(f.ssl, "certificates", "api-task-nself-org")); link != ".api-task-nself-org.gen-0" || len(f.execs) != 2 {
		t.Errorf("missing target not imported: link=%q execs=%v", link, f.execs)
	}
}
