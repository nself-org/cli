package acme

// http01_test.go: the HTTP-01 leg of the ACME engine (P7-LIVE-24): the lego
// argv, the webroot permissions, the probe helper and adoption of webroot
// certbot lineages.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/docker"
)

func TestACMEIssueHTTP01Argv(t *testing.T) {
	ssl := t.TempDir()
	var got docker.RunSpec
	run := func(_ context.Context, s docker.RunSpec) (string, string, error) { got = s; return "", "", nil }
	cert, key, err := IssueHTTP01(context.Background(), IssueReq{SSLDir: ssl, Name: "app", Contact: "ops@example.org",
		Domains: []string{"app.example.org"}, AcceptTOS: true, Run: run,
		Secret: func(string) (string, error) { t.Fatal("HTTP-01 must not read a secret"); return "", nil }})
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(got.Args, " ")
	for _, want := range []string{"--http --http.webroot /ssl/.acme-webroot", "--domains app.example.org", "--accept-tos", "--path /ssl/.acme", "--email ops@example.org"} {
		if !strings.Contains(args, want) {
			t.Errorf("argv %q lacks %q", args, want)
		}
	}
	if strings.Contains(args, "--dns") || len(got.EnvPass) != 0 || got.Image != LegoImage {
		t.Errorf("http-01 run must carry no DNS flag or credential env: %v %v", args, got.EnvPass)
	}
	if len(got.Mounts) != 1 || got.Mounts[0].Source != ssl || got.Mounts[0].Destination != "/ssl" || got.Mounts[0].ReadOnly {
		t.Errorf("mounts: %+v", got.Mounts)
	}
	if !strings.HasSuffix(cert, filepath.Join(".acme", "certificates", "app.example.org.crt")) || !strings.HasSuffix(key, ".key") {
		t.Errorf("paths: %s %s", cert, key)
	}
	if runtime.GOOS != "windows" {
		for _, d := range []string{"", ".well-known", ".well-known/acme-challenge"} {
			fi, err := os.Stat(filepath.Join(WebrootDir(ssl), d))
			if err != nil || fi.Mode().Perm() != 0o755 {
				t.Errorf("webroot %q: %v %v", d, fi, err)
			}
		}
	}
	for _, d := range []string{"*.example.org", "bücher.example.org"} {
		if _, _, err := IssueHTTP01(context.Background(), IssueReq{SSLDir: ssl, Domains: []string{d}, Run: run}); err == nil {
			t.Errorf("%q must be refused over HTTP-01", d)
		}
	}
}

// webrootServer answers like the served nginx: a bare token from the webroot, else 404.
func webrootServer(t *testing.T, ssl string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.URL.Path, ChallengePrefix)
		b, err := os.ReadFile(filepath.Join(WebrootDir(ssl), ".well-known", "acme-challenge", filepath.Base(tok)))
		if !strings.HasPrefix(r.URL.Path, ChallengePrefix) || err != nil {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
}

func TestACMEAdoptHTTP01Probe(t *testing.T) {
	ssl := t.TempDir()
	srv := webrootServer(t, ssl)
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")
	if err := ProbeWebroot(context.Background(), ssl, addr, "app.example.org", nil); err != nil {
		t.Fatalf("probe against a serving nginx: %v", err)
	}
	if m, _ := filepath.Glob(filepath.Join(WebrootDir(ssl), ".well-known", "acme-challenge", "*")); len(m) != 0 {
		t.Errorf("the probe file must be deleted: %v", m)
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	defer dead.Close()
	if err := ProbeWebroot(context.Background(), ssl, strings.TrimPrefix(dead.URL, "http://"), "app.example.org", nil); err == nil {
		t.Error("a nginx without the location must fail the probe")
	}
	redirect := httptest.NewServer(http.RedirectHandler("https://example.org/", http.StatusMovedPermanently))
	defer redirect.Close()
	if err := ProbeWebroot(context.Background(), ssl, strings.TrimPrefix(redirect.URL, "http://"), "app.example.org", nil); err == nil {
		t.Error("a redirect must fail the probe")
	}

	hosts := map[string]string{
		"app.example.org":  "/etc/nginx/ssl/certificates/app-example-org/fullchain.pem",
		"gone.example.org": "/etc/nginx/ssl/certificates/gone-example-org/fullchain.pem",
	}
	cbs := []Certbot{
		{Name: "app.example.org", Authenticator: "webroot", Conf: "/c/app.conf", Domains: []string{"app.example.org"}},
		{Name: "gone.example.org", Authenticator: "webroot", Conf: "/c/gone.conf", Domains: []string{"gone.example.org"}},
		{Name: "sa", Authenticator: "standalone", Domains: []string{"app.example.org"}},
	}
	probe := func(host string) error {
		if host == "gone.example.org" {
			return errors.New("HTTP 404, not the probe file")
		}
		return nil
	}
	ad, refused := PlanWith(cbs, hosts, "", "", probe)
	if len(ad) != 1 || ad[0].Name != "app.example.org" || ad[0].Challenge != ChallengeHTTP || ad[0].DNSProvider != "" ||
		len(ad[0].CredentialSecrets) != 0 || ad[0].Targets[0] != "certificates/app-example-org" || ad[0].AdoptedFrom == nil {
		t.Fatalf("a webroot lineage with a passing probe must adopt as http-01: %+v", ad)
	}
	if len(refused) != 2 || !strings.Contains(refused[0], "gone.example.org") || !strings.Contains(refused[0], "nself build") ||
		!strings.Contains(refused[1], "stopped or shared port") {
		t.Fatalf("refusals must name the host and the remediation: %v", refused)
	}
	if ad, refused = Plan(cbs[:1], hosts, "", ""); len(ad) != 0 || len(refused) != 1 {
		t.Errorf("without a probe a webroot lineage keeps the old behaviour: %+v %v", ad, refused)
	}
	if ad, _ = PlanWith(cbs[:1], hosts, "cloudflare", "", probe); len(ad) != 1 || ad[0].Challenge != ChallengeDNS {
		t.Errorf("a credential still converts the lineage to dns-01: %+v", ad)
	}
	wild := []Certbot{{Name: "w", Authenticator: "webroot", Domains: []string{"*.example.org"}}}
	if _, refused = PlanWith(wild, hosts, "", "", probe); len(refused) != 1 || !strings.Contains(refused[0], "wildcard") {
		t.Errorf("a wildcard cannot be http-01: %v", refused)
	}
}
