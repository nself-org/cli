package commands

// ssl_acme_add_test.go: `trust ssl add <domain> --acme` (HTTP-01, P7-LIVE-24)
// against the prod-layout ACME fixture of ssl_acme_test.go, with docker, the
// served-nginx probes and the CA replaced by seams. No CA, no network.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ssl/acme"
)

// runAdd runs `trust ssl add` with flags and returns its output.
func (f *acmeFix) runAdd(domain string, flags ...string) (string, error) {
	f.t.Helper()
	acmeResetFlags(sslAddCmd)
	for _, fl := range flags {
		k, v, hasV := strings.Cut(strings.TrimPrefix(fl, "--"), "=")
		if !hasV {
			v = "true"
		}
		if err := sslAddCmd.Flags().Set(k, v); err != nil {
			f.t.Fatalf("flag %s: %v", fl, err)
		}
	}
	var out bytes.Buffer
	sslAddCmd.SetOut(&out)
	sslAddCmd.SetContext(context.Background())
	err := runSSLAdd(sslAddCmd, []string{domain})
	return out.String(), err
}

// addFix is the fixture with a per-host served-certificate prober and a
// recording HTTP-01 reachability probe.
func addFix(t *testing.T, probeErr error) (f *acmeFix, probed *[]string) {
	t.Helper()
	f = newACMEFix(t)
	t.Cleanup(func() { acmeResetFlags(sslAddCmd) })
	acmeD.probe = func(_ context.Context, _, sni string, _ time.Duration) (ssl.ServedCert, error) {
		b, err := os.ReadFile(filepath.Join(f.ssl, "certificates", strings.ReplaceAll(sni, ".", "-"), "fullchain.pem"))
		if err != nil {
			return ssl.ServedCert{}, err
		}
		blk, _ := pem.Decode(b)
		sum := sha256.Sum256(blk.Bytes)
		return ssl.ServedCert{Host: sni, SHA256: hex.EncodeToString(sum[:])}, nil
	}
	probed = new([]string)
	old := acmeProbeHTTP
	acmeProbeHTTP = func(_ context.Context, _, addr, host string, _ func(*http.Request) (*http.Response, error)) error {
		*probed = append(*probed, host+"@"+addr)
		return probeErr
	}
	t.Cleanup(func() { acmeProbeHTTP = old })
	return f, probed
}

func TestSSLAddACME(t *testing.T) {
	f, probed := addFix(t, nil)
	before := acmeTree(t, f.root)

	out, err := f.runAdd("app.example.org", "--acme", "--dry-run")
	if err != nil || !strings.Contains(out, "lineage app-example-org: http-01 for app.example.org") ||
		!strings.Contains(out, filepath.Join(f.ssl, ".acme-webroot")) || !strings.Contains(out, "dry run: nothing written") {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	if acmeTree(t, f.root) != before || len(f.runs)+len(f.execs)+len(*probed) != 0 {
		t.Fatal("dry run wrote or called out")
	}

	out, err = f.runAdd("App.Example.org.", "--acme", "--agree-tos", "--upstream=app:3000")
	if err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	if len(f.runs) != 1 {
		t.Fatalf("lego runs: %d", len(f.runs))
	}
	args := strings.Join(f.runs[0].Args, " ")
	if !strings.Contains(args, "--http --http.webroot /ssl/.acme-webroot") || !strings.Contains(args, "--domains app.example.org") ||
		!strings.Contains(args, "--accept-tos") || strings.Contains(args, "--dns") || len(f.runs[0].EnvPass) != 0 {
		t.Errorf("lego argv/env: %s %v", args, f.runs[0].EnvPass)
	}
	if got := *probed; len(got) != 1 || got[0] != "app.example.org@127.0.0.1:80" {
		t.Errorf("reachability probe: %v", got)
	}
	if dest, err := os.Readlink(filepath.Join(f.ssl, "certificates", "app-example-org")); err != nil || !strings.HasPrefix(dest, ".app-example-org.gen-") {
		t.Errorf("certificate target is not a generation link: %q %v", dest, err)
	}
	conf, err := os.ReadFile(filepath.Join(f.served, "nginx", "conf.d", "custom-app-example-org.conf"))
	if err != nil {
		t.Fatal(err)
	}
	c80 := strings.SplitN(string(conf), "listen 443", 2)[0]
	if strings.Count(c80, acme.NginxChallengeLocation) != 1 || strings.Index(c80, "acme-challenge") > strings.Index(c80, "return 301") {
		t.Errorf("custom-domain conf: the port-80 block needs the challenge location before the redirect\n%s", conf)
	}
	if !strings.Contains(string(conf), "http://app:3000") {
		t.Errorf("--upstream not applied:\n%s", conf)
	}
	reloads := 0
	for _, e := range f.execs {
		if strings.Join(e, " ") == "nginx -s reload" {
			reloads++
		}
	}
	if reloads < 2 {
		t.Errorf("nginx reloads: %d (%v)", reloads, f.execs)
	}
	file, err := acme.Load(f.ssl)
	if err != nil || len(file.Lineages) != 1 || file.Lineages[0].Challenge != "http-01" || file.Lineages[0].DNSProvider != "" ||
		len(file.Lineages[0].CredentialSecrets) != 0 || file.Lineages[0].LastIssued == nil || file.Contact != "ops@example.org" {
		t.Errorf("lineages.json: %+v %v", file, err)
	}
	if _, err := f.runAdd("app.example.org", "--acme"); err == nil || !strings.Contains(err.Error(), "already managed") {
		t.Errorf("a second add must refuse: %v", err)
	}
}

func TestSSLAddACMERefusals(t *testing.T) {
	f, _ := addFix(t, http.ErrServerClosed)
	if _, err := f.runAdd("app.example.org", "--acme", "--agree-tos"); err == nil || !strings.Contains(err.Error(), "does not answer the HTTP-01 challenge location") ||
		len(f.runs) != 0 {
		t.Errorf("an unreachable challenge location must refuse before the CA is asked: %v (%d runs)", err, len(f.runs))
	}
	for _, d := range []string{"*.example.org", "example", "a b.example.org", "ex_ample.org", "app.example.org:8443"} {
		if _, err := f.runAdd(d, "--acme"); err == nil || !strings.Contains(err.Error(), "cannot be issued over HTTP-01") {
			t.Errorf("%q: %v", d, err)
		}
	}
	if _, err := f.runAdd("app.example.org", "--agree-tos"); err == nil || !strings.Contains(err.Error(), "only works with --acme") {
		t.Errorf("--agree-tos without --acme: %v", err)
	}
	g, _ := addFix(t, nil)
	if _, err := g.runAdd("app.example.org", "--acme"); err == nil || !strings.Contains(err.Error(), "terms of service") {
		t.Errorf("a first account without --agree-tos or a TTY must refuse: %v", err)
	}
	f.runs = append(f.runs, g.runs...)
	if len(f.runs) != 0 {
		t.Error("no refusal may reach the CA")
	}
}

// TestSSLRenewACMEHTTP01: `renew --acme` renews an http-01 lineage through the
// HTTP-01 issuer: no provider, no secret, no age key, the challenge location
// probed first, a new generation installed; --staging installs nothing.
func TestSSLRenewACMEHTTP01(t *testing.T) {
	f, probed := addFix(t, nil)
	if _, err := f.runAdd("app.example.org", "--acme", "--agree-tos"); err != nil {
		t.Fatal(err)
	}
	f.runs, f.execs, *probed = nil, nil, nil
	t.Setenv("SECRETS_AGE_KEY_PATH", filepath.Join(f.root, "no-such-key"))
	acmeInput.LookPath = func(string) (string, error) { return "", os.ErrNotExist }
	link := func() string { d, _ := os.Readlink(filepath.Join(f.ssl, "certificates", "app-example-org")); return d }
	if !strings.Contains(link(), "gen-0") {
		t.Fatalf("setup: %q", link())
	}

	if out, err := f.run(sslRenewCmd, "--acme", "--staging", "--agree-tos"); err != nil || strings.Contains(out, "age:") {
		t.Fatalf("staging renew: %v\n%s", err, out)
	}
	if len(f.runs) != 1 || !strings.Contains(strings.Join(f.runs[0].Args, " "), "--path /ssl/.acme/staging") || !strings.Contains(strings.Join(f.runs[0].Args, " "), "acme-staging-v02") || link() != ".app-example-org.gen-0" {
		t.Errorf("--staging must issue from the staging CA and install nothing: %v %q", f.runs, link())
	}

	f.runs, *probed = nil, nil
	if out, err := f.run(sslRenewCmd, "--acme", "--force"); err != nil {
		t.Fatalf("renew: %v\n%s", err, out)
	}
	args := ""
	if len(f.runs) == 1 {
		args = strings.Join(f.runs[0].Args, " ")
	}
	if !strings.Contains(args, "--http --http.webroot /ssl/.acme-webroot") || strings.Contains(args, "--dns") || len(f.runs[0].EnvPass) != 0 || strings.Contains(args, "--accept-tos") {
		t.Errorf("renew argv: %q", args)
	}
	if len(*probed) != 1 || !strings.HasPrefix((*probed)[0], "app.example.org@") {
		t.Errorf("the challenge location must be probed before renewing: %v", *probed)
	}
	if link() != ".app-example-org.gen-1" {
		t.Errorf("a new generation must be installed: %q", link())
	}
	if file, _ := acme.Load(f.ssl); len(file.Lineages) != 1 || file.Lineages[0].LastIssued == nil {
		t.Errorf("lineages.json: %+v", file)
	}

	f.runs = nil
	acmeProbeHTTP = func(context.Context, string, string, string, func(*http.Request) (*http.Response, error)) error {
		return http.ErrNotSupported
	}
	if _, err := f.run(sslRenewCmd, "--acme", "--force"); err == nil || !strings.Contains(err.Error(), "does not answer the HTTP-01 challenge location") || len(f.runs) != 0 || link() != ".app-example-org.gen-1" {
		t.Errorf("a missing challenge location must refuse before the CA is asked: %v (%d runs, %q)", err, len(f.runs), link())
	}
}

// TestACMEAdoptHTTP01Command: `setup --acme --adopt-certbot` adopts a webroot
// lineage as http-01 when the probe passes, and refuses it with a remediation
// (nothing written) when the served nginx lacks the challenge location.
func TestACMEAdoptHTTP01Command(t *testing.T) {
	f, probed := addFix(t, nil)
	le := filepath.Join(f.root, "letsencrypt")
	name, dir := "shop.example.org", "shop-example-org"
	cert, key := acmeTestCert(t, []string{name}, time.Now().Add(20*24*time.Hour))
	acmeWriteFile(t, filepath.Join(le, "renewal", name+".conf"), []byte("version = 2.9.0\n[renewalparams]\nauthenticator = webroot\nwebroot_path = /var/www/certbot\n"))
	for _, p := range []string{filepath.Join(le, "live", name), filepath.Join(f.ssl, "certificates", dir)} {
		acmeWriteFile(t, filepath.Join(p, "fullchain.pem"), cert)
		acmeWriteFile(t, filepath.Join(p, "privkey.pem"), key)
	}
	acmeWriteFile(t, filepath.Join(le, "live", name, "cert.pem"), cert)
	acmeWriteFile(t, filepath.Join(f.served, "nginx", "conf.d", dir+".conf"), []byte("server {\n  listen 443 ssl;\n  server_name "+name+";\n  ssl_certificate /etc/nginx/ssl/certificates/"+dir+"/fullchain.pem;\n  ssl_certificate_key /etc/nginx/ssl/certificates/"+dir+"/privkey.pem;\n}\n"))

	before := acmeTree(t, f.root)
	acmeProbeHTTP = func(context.Context, string, string, string, func(*http.Request) (*http.Response, error)) error {
		return http.ErrNotSupported
	}
	out, err := f.run(sslSetupCmd, "--acme", "--adopt-certbot="+le)
	if err == nil || !strings.Contains(out, "refused "+name) || !strings.Contains(out, "run `nself build`") || !strings.Contains(out, "--dns-credential-file") {
		t.Fatalf("a webroot lineage without the challenge location must be refused with a remediation: %v\n%s", err, out)
	}
	if acmeTree(t, f.root) != before || len(f.set) != 0 {
		t.Fatal("a refused adoption wrote something")
	}

	acmeProbeHTTP = func(_ context.Context, _, addr, host string, _ func(*http.Request) (*http.Response, error)) error {
		*probed = append(*probed, host+"@"+addr)
		return nil
	}
	out, err = f.run(sslSetupCmd, "--acme", "--adopt-certbot="+le)
	if err != nil || !strings.Contains(out, "adopted 1 lineage(s) (1 http-01)") || !strings.Contains(out, "lineage "+name+": http-01") {
		t.Fatalf("adopt: %v\n%s", err, out)
	}
	file, _ := acme.Load(f.ssl)
	if len(file.Lineages) != 1 {
		t.Fatalf("lineages: %+v", file.Lineages)
	}
	l := file.Lineages[0]
	if l.Challenge != "http-01" || l.DNSProvider != "" || len(l.CredentialSecrets) != 0 || l.Targets[0] != "certificates/"+dir || l.AdoptedFrom == nil || len(f.set) != 0 || len(f.runs) != 0 {
		t.Errorf("lineage %+v (secrets stored %v, lego runs %d)", l, f.set, len(f.runs))
	}
	if len(*probed) != 1 || !strings.HasPrefix((*probed)[0], name+"@") {
		t.Errorf("probe calls: %v", *probed)
	}
}
