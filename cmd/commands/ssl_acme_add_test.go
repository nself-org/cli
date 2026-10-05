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
