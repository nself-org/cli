package commands

// Tests for the doctor TLS section. The unit tests serve certificates from
// in-process TLS servers (real SNI, real ssl.ProbeServed) and stub only the
// docker lookup; TestDoctorTLSIntegration uses a real nginx container.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/ssl"
)

// tlsDays returns an expiry n whole days (plus a half-day margin) from now.
func tlsDays(n int) time.Time { return time.Now().Add(time.Duration(n)*24*time.Hour + 12*time.Hour) }

// tlsWrite creates p and its parents.
func tlsWrite(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tlsMakeCert returns a self-signed certificate for names plus its PEM files.
func tlsMakeCert(t *testing.T, notAfter time.Time, names ...string) (tls.Certificate, string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: names[0]},
		DNSNames: names, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key},
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}))
}

// tlsServe starts a TLS server that picks its certificate by SNI and returns
// its port and a handshake counter.
func tlsServe(t *testing.T, certs map[string]tls.Certificate) (string, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.TLS = &tls.Config{GetCertificate: func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
		atomic.AddInt32(&hits, 1)
		c := certs[h.ServerName]
		return &c, nil
	}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	return port, &hits
}

// tlsIsolateEnv blanks the keys config.Load would pick up from the process.
func tlsIsolateEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"ENV", "PROJECT_NAME", "SSL_MODE", "NGINX_FRONTED_BY", "NGINX_HTTPS_PORT", "NGINX_SSL_PORT", "NGINX_BIND_IP"} {
		t.Setenv(k, "")
	}
}

// tlsStack writes a served stack under root: .env (serving port), one conf per
// host naming certDir's fullchain.pem, and returns the ssl dir.
func tlsStack(t *testing.T, root, port string, confs map[string]string) string {
	t.Helper()
	tlsWrite(t, filepath.Join(root, ".env"), "ENV=dev\nPROJECT_NAME=stack\nSSL_MODE=custom\nNGINX_BIND_IP=127.0.0.1\nNGINX_HTTPS_PORT="+port+"\n")
	for host, certDir := range confs {
		tlsWrite(t, filepath.Join(root, "nginx", "sites", host+".conf"), fmt.Sprintf(
			"server {\n  listen 443 ssl;\n  server_name %s;\n  ssl_certificate /etc/nginx/ssl/certificates/%s/fullchain.pem;\n  ssl_certificate_key /etc/nginx/ssl/certificates/%s/privkey.pem;\n}\n",
			host, certDir, certDir))
	}
	return filepath.Join(root, "ssl")
}

// tlsTestDeps builds seams with a running nginx; probe is the real prober.
func tlsTestDeps(t *testing.T) tlsDeps {
	t.Helper()
	return tlsDeps{
		findNginx:      func(context.Context, docker.ServiceMatch) (string, error) { return "stack-nginx-1", nil },
		probe:          ssl.ProbeServed,
		letsencryptDir: filepath.Join(t.TempDir(), "letsencrypt"),
	}
}

// tlsRun runs the check and indexes the results by name.
func tlsRun(t *testing.T, proj string, d tlsDeps) ([]doctorCheckResult, map[string]doctorCheckResult) {
	t.Helper()
	res := checkServedCertificatesWith(context.Background(), proj, false, d)
	by := map[string]doctorCheckResult{}
	for _, r := range res {
		by[r.Name] = r
	}
	return res, by
}

// TestDoctorTLSFronted pins the prod layout: the project sits in
// nself-web/backend with NGINX_FRONTED_BY=nself-web, and the TLS lines come
// from the fronting stack's confs and nginx.
func TestDoctorTLSFronted(t *testing.T) {
	tlsIsolateEnv(t)
	base := t.TempDir()
	root, proj := filepath.Join(base, "nself-web"), filepath.Join(base, "nself-web", "backend")
	cert, certPEM, keyPEM := tlsMakeCert(t, tlsDays(40), "api.example.com")
	port, hits := tlsServe(t, map[string]tls.Certificate{"api.example.com": cert})
	sslDir := tlsStack(t, root, port, map[string]string{"api.example.com": "example-com"})
	tlsWrite(t, filepath.Join(sslDir, "certificates", "example-com", "fullchain.pem"), certPEM)
	tlsWrite(t, filepath.Join(sslDir, "certificates", "example-com", "privkey.pem"), keyPEM)
	tlsWrite(t, filepath.Join(proj, ".env"), "ENV=dev\nPROJECT_NAME=ntask\nSSL_MODE=custom\nNGINX_FRONTED_BY=nself-web\n")

	d := tlsTestDeps(t)
	var match docker.ServiceMatch
	d.findNginx = func(_ context.Context, m docker.ServiceMatch) (string, error) {
		match = m
		return "nself-web-nginx-1", nil
	}
	res, by := tlsRun(t, proj, d)

	r, ok := by["TLS api.example.com"]
	if len(res) != 1 || !ok || r.Status != "pass" {
		t.Fatalf("want one passing line from the fronting stack's confs, got %+v", res)
	}
	if !strings.Contains(r.Message, "api.example.com expires ") || !strings.Contains(r.Message, "(40 days)") {
		t.Errorf("message = %q", r.Message)
	}
	if atomic.LoadInt32(hits) == 0 {
		t.Error("the fronting stack's nginx was never dialled")
	}
	if match.Service != "nginx" || match.Project != "ntask" || len(match.WorkingDirs) != 2 || match.WorkingDirs[0] != root || match.WorkingDirs[1] != proj {
		t.Errorf("nginx lookup = %+v, want service nginx, project ntask, dirs [%s %s]", match, root, proj)
	}
}

// TestDoctorTLSThresholds pins 21/7-day verdicts and that the section fails.
func TestDoctorTLSThresholds(t *testing.T) {
	tlsIsolateEnv(t)
	root := t.TempDir()
	certs, confs := map[string]tls.Certificate{}, map[string]string{}
	for host, n := range map[string]int{"ok.example.com": 40, "warn.example.com": 15, "fail.example.com": 5, "dead.example.com": -2} {
		var certPEM string
		certs[host], certPEM, _ = tlsMakeCert(t, tlsDays(n), host)
		confs[host] = host
		tlsWrite(t, filepath.Join(root, "ssl", "certificates", host, "fullchain.pem"), certPEM)
	}
	port, _ := tlsServe(t, certs)
	tlsStack(t, root, port, confs)

	res, by := tlsRun(t, root, tlsTestDeps(t))
	want := map[string]string{"ok": "pass", "warn": "warn", "fail": "fail", "dead": "fail"}
	for k, st := range want {
		if got := by["TLS "+k+".example.com"].Status; got != st {
			t.Errorf("%s: status %q, want %q (%+v)", k, got, st, by["TLS "+k+".example.com"])
		}
	}
	if !strings.HasSuffix(by["TLS dead.example.com"].Message, "EXPIRED") {
		t.Errorf("expired line must say so: %q", by["TLS dead.example.com"].Message)
	}
	if by["TLS ok.example.com"].Detail == "" {
		t.Error("a self-signed leaf must report its chain error in Detail, separately from the verdict")
	}
	if rep := buildDoctorReport(res); rep.Summary.Failed != 2 || rep.Summary.Warnings != 1 || rep.Summary.Passed != 1 {
		t.Errorf("summary = %+v", rep.Summary)
	}
}

// TestDoctorTLSRenewedNotInstalled is the D-0121 regression: the served tree
// holds the old certificate while a newer one sits in the ACME store or in a
// certbot live lineage.
func TestDoctorTLSRenewedNotInstalled(t *testing.T) {
	for _, tc := range []struct{ name, rel string }{
		{"certbot live lineage", filepath.Join("live", "api.example.com", "cert.pem")},
		{"acme store", filepath.Join("ssl", ".acme", "certificates", "api.example.com.crt")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tlsIsolateEnv(t)
			root := t.TempDir()
			old, oldPEM, oldKey := tlsMakeCert(t, tlsDays(30), "api.example.com")
			port, _ := tlsServe(t, map[string]tls.Certificate{"api.example.com": old})
			sslDir := tlsStack(t, root, port, map[string]string{"api.example.com": "example-com"})
			tlsWrite(t, filepath.Join(sslDir, "certificates", "example-com", "fullchain.pem"), oldPEM)
			tlsWrite(t, filepath.Join(sslDir, "certificates", "example-com", "privkey.pem"), oldKey)

			d := tlsTestDeps(t)
			_, newPEM, _ := tlsMakeCert(t, tlsDays(89), "api.example.com")
			if strings.HasPrefix(tc.rel, "live") {
				tlsWrite(t, filepath.Join(d.letsencryptDir, tc.rel), newPEM)
			} else {
				tlsWrite(t, filepath.Join(root, tc.rel), newPEM)
			}
			res, by := tlsRun(t, root, d)

			if by["TLS api.example.com"].Status != "pass" {
				t.Errorf("the served certificate itself has 30 days: %+v", by["TLS api.example.com"])
			}
			r := by["TLS api.example.com renewal"]
			oldDay, newDay := tlsDays(30).UTC().Format("2006-01-02"), tlsDays(89).UTC().Format("2006-01-02")
			if r.Status != "warn" || len(res) != 2 {
				t.Fatalf("want exactly the host line and one renewal warning, got %+v", res)
			}
			for _, s := range []string{"renewed but not installed", oldDay, newDay, "install/reload"} {
				if !strings.Contains(r.Message, s) {
					t.Errorf("renewal message %q lacks %q", r.Message, s)
				}
			}
		})
	}
}

// TestDoctorTLSServedNotDisk pins the nginx-not-reloaded warning.
func TestDoctorTLSServedNotDisk(t *testing.T) {
	tlsIsolateEnv(t)
	root := t.TempDir()
	served, _, _ := tlsMakeCert(t, tlsDays(30), "api.example.com")
	_, diskPEM, _ := tlsMakeCert(t, tlsDays(80), "api.example.com")
	port, _ := tlsServe(t, map[string]tls.Certificate{"api.example.com": served})
	sslDir := tlsStack(t, root, port, map[string]string{"api.example.com": "example-com"})
	tlsWrite(t, filepath.Join(sslDir, "certificates", "example-com", "fullchain.pem"), diskPEM)

	_, by := tlsRun(t, root, tlsTestDeps(t))
	r := by["TLS api.example.com disk"]
	if r.Status != "warn" || !strings.Contains(r.Message, sslDir) || !strings.Contains(r.Message, "reload nginx") {
		t.Fatalf("want a served-vs-disk warning naming %s and `reload nginx`, got %+v", sslDir, r)
	}

	if err := os.Remove(filepath.Join(sslDir, "certificates", "example-com", "fullchain.pem")); err != nil {
		t.Fatal(err)
	}
	if _, by = tlsRun(t, root, tlsTestDeps(t)); by["TLS api.example.com disk"].Status != "warn" {
		t.Errorf("a missing ssl_certificate file must warn: %+v", by)
	}
}

// TestDoctorTLSSubdomainCertDir pins that a generated route block (api.<base>)
// is compared against the base domain's certificate directory.
func TestDoctorTLSSubdomainCertDir(t *testing.T) {
	tlsIsolateEnv(t)
	root := t.TempDir()
	cert, certPEM, _ := tlsMakeCert(t, tlsDays(60), "example.com", "api.example.com")
	port, _ := tlsServe(t, map[string]tls.Certificate{"api.example.com": cert})
	sslDir := tlsStack(t, root, port, map[string]string{"api.example.com": "example-com"})
	tlsWrite(t, filepath.Join(sslDir, "certificates", "example-com", "fullchain.pem"), certPEM)

	res, by := tlsRun(t, root, tlsTestDeps(t))
	if len(res) != 1 || by["TLS api.example.com"].Status != "pass" {
		t.Fatalf("api.example.com is served from example-com's certificate: no served!=disk warning expected, got %+v", res)
	}
	if _, err := os.Stat(filepath.Join(sslDir, "certificates", "api-example-com")); err == nil {
		t.Fatal("fixture must not contain a per-host cert dir")
	}
}

// TestDoctorTLSServedEnv pins that the fronting root's .env decides the dial
// address and that this process's environment is unchanged by the check.
func TestDoctorTLSServedEnv(t *testing.T) {
	tlsIsolateEnv(t)
	base := t.TempDir()
	root, proj := filepath.Join(base, "nself-web"), filepath.Join(base, "nself-web", "backend")
	_, certPEM, _ := tlsMakeCert(t, tlsDays(60), "api.example.com")
	sslDir := tlsStack(t, root, "8443", map[string]string{"api.example.com": "example-com"})
	tlsWrite(t, filepath.Join(sslDir, "certificates", "example-com", "fullchain.pem"), certPEM)
	tlsWrite(t, filepath.Join(proj, ".env"), "ENV=dev\nPROJECT_NAME=ntask\nSSL_MODE=custom\nNGINX_FRONTED_BY=nself-web\nNGINX_HTTPS_PORT=1111\n")
	disk, err := ssl.ReadDiskCert(filepath.Join(sslDir, "certificates", "example-com", "fullchain.pem"))
	if err != nil {
		t.Fatal(err)
	}

	var dialled string
	d := tlsTestDeps(t)
	d.probe = func(_ context.Context, addr, sni string, _ time.Duration) (ssl.ServedCert, error) {
		dialled = addr
		return ssl.ServedCert{Host: sni, IssuerCN: "x", NotAfter: disk.NotAfter, SHA256: disk.SHA256, DNSNames: []string{sni}}, nil
	}
	before := os.Environ()
	sort.Strings(before)
	_, by := tlsRun(t, proj, d)
	after := os.Environ()
	sort.Strings(after)

	if dialled != "127.0.0.1:8443" {
		t.Errorf("dialled %q, want 127.0.0.1:8443 from the fronting root's .env (not the project's 1111)", dialled)
	}
	if by["TLS api.example.com"].Status != "pass" {
		t.Errorf("got %+v", by)
	}
	if strings.Join(before, "\x00") != strings.Join(after, "\x00") {
		t.Error("the check changed this process's environment")
	}
}

// TestDoctorTLSSkip pins every not-applicable case: one skip line without the
// word "expires", no probe, and no effect on the pass/warn/fail counts.
func TestDoctorTLSSkip(t *testing.T) {
	cases := []struct {
		name  string
		env   string
		find  func(context.Context, docker.ServiceMatch) (string, error)
		extra func(dir string)
	}{
		{name: "ssl local", env: "SSL_MODE=local\n"},
		{name: "ssl none", env: "SSL_MODE=none\n"},
		{name: "no nginx container", env: "SSL_MODE=letsencrypt\n",
			find: func(context.Context, docker.ServiceMatch) (string, error) {
				return "", fmt.Errorf("%w: none", docker.ErrServiceContainerNotFound)
			}},
		{name: "docker unavailable", env: "SSL_MODE=letsencrypt\n",
			find: func(context.Context, docker.ServiceMatch) (string, error) {
				return "", errors.New("cannot connect to docker")
			}},
		{name: "fronting unresolved", env: "SSL_MODE=letsencrypt\nNGINX_FRONTED_BY=nope\n"},
		{name: "no tls hosts", env: "SSL_MODE=letsencrypt\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tlsIsolateEnv(t)
			dir := t.TempDir()
			tlsWrite(t, filepath.Join(dir, ".env"), "ENV=dev\nPROJECT_NAME=p\n"+tc.env)
			d := tlsTestDeps(t)
			if tc.find != nil {
				d.findNginx = tc.find
			}
			d.probe = func(context.Context, string, string, time.Duration) (ssl.ServedCert, error) {
				t.Error("a skipped check must not dial")
				return ssl.ServedCert{}, errors.New("unexpected")
			}
			res, _ := tlsRun(t, dir, d)
			if len(res) != 1 || res[0].Status != "skip" || !strings.HasPrefix(res[0].Message, "skipped (") || strings.Contains(res[0].Message, "expires") {
				t.Fatalf("want one skip line without `expires`, got %+v", res)
			}
			if rep := buildDoctorReport(res); rep.Summary.Passed+rep.Summary.Warnings+rep.Summary.Failed != 0 {
				t.Errorf("a skip must count as nothing: %+v", rep.Summary)
			}
		})
	}
}

// TestDoctorTLSSkipFailures pins the cases that must fail rather than skip.
func TestDoctorTLSSkipFailures(t *testing.T) {
	tlsIsolateEnv(t)
	root := t.TempDir()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, closedPort, _ := net.SplitHostPort(l.Addr().String())
	_ = l.Close()
	tlsStack(t, root, closedPort, map[string]string{"api.example.com": "example-com"})

	res, by := tlsRun(t, root, tlsTestDeps(t))
	r := by["TLS api.example.com"]
	if len(res) != 1 || r.Status != "fail" || !strings.Contains(r.Message, "127.0.0.1:"+closedPort) {
		t.Fatalf("a running nginx on the wrong port must fail naming the address, got %+v", res)
	}

	d := tlsTestDeps(t)
	d.findNginx = func(context.Context, docker.ServiceMatch) (string, error) {
		return "", fmt.Errorf("%w: \"nginx\" matches a-nginx-1, b-nginx-1", docker.ErrServiceContainerAmbiguous)
	}
	res, _ = tlsRun(t, root, d)
	if len(res) != 1 || res[0].Status != "fail" || !strings.Contains(res[0].Message, "a-nginx-1") || !strings.Contains(res[0].Message, "b-nginx-1") {
		t.Fatalf("an ambiguous nginx must fail naming the containers, got %+v", res)
	}
}

// TestDoctorTLSIntegration serves three certificates (40, 15 and 5 days left)
// from a real nginx container and runs the check end to end. It skips only
// when no Docker daemon is reachable.
func TestDoctorTLSIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("docker integration test skipped in -short mode")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	// nginx:alpine is a Linux image: a Windows-container engine cannot run it.
	if out, err := exec.Command("docker", "info", "--format", "{{.OSType}}").Output(); err != nil {
		t.Skipf("docker daemon unavailable: %v", err)
	} else if strings.TrimSpace(string(out)) != "linux" {
		t.Skipf("docker engine is %q, not linux", strings.TrimSpace(string(out)))
	}
	tlsIsolateEnv(t)
	root := t.TempDir()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	_ = l.Close()

	confs := map[string]string{}
	want := map[string]string{"a.example.com": "pass", "b.example.com": "warn", "c.example.com": "fail"}
	sslDir := filepath.Join(root, "ssl")
	for host, n := range map[string]int{"a.example.com": 40, "b.example.com": 15, "c.example.com": 5} {
		_, certPEM, keyPEM := tlsMakeCert(t, tlsDays(n), host)
		dir := ssl.DomainToDirName(host)
		tlsWrite(t, filepath.Join(sslDir, "certificates", dir, "fullchain.pem"), certPEM)
		tlsWrite(t, filepath.Join(sslDir, "certificates", dir, "privkey.pem"), keyPEM)
		confs[host] = dir
	}
	tlsStack(t, root, port, confs)

	name := fmt.Sprintf("lv16-it-%d", time.Now().UnixNano())
	// Files are copied in with `docker cp`, not bind-mounted: a VM-backed
	// engine (Colima, Docker Desktop) shares only some host paths, and an
	// unshared t.TempDir() mounts as an empty directory.
	docker := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
			t.Fatalf("docker %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	docker("create", "--name", name,
		"--label", "com.docker.compose.service=nginx", "--label", "com.docker.compose.project=lv16it",
		"--label", "com.docker.compose.project.working_dir="+root,
		"-p", "127.0.0.1:"+port+":443", "nginx:alpine")
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	docker("cp", filepath.Join(root, "nginx", "sites")+string(filepath.Separator)+".", name+":/etc/nginx/conf.d")
	docker("cp", sslDir+string(filepath.Separator)+".", name+":/etc/nginx/ssl")
	docker("start", name)

	ctx := context.Background()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, perr := ssl.ProbeServed(ctx, "127.0.0.1:"+port, "a.example.com", 2*time.Second); perr == nil {
			break
		} else if time.Now().After(deadline) {
			logs, _ := exec.Command("docker", "logs", name).CombinedOutput()
			t.Fatalf("nginx never served TLS: %v\n%s", perr, logs)
		}
		time.Sleep(500 * time.Millisecond)
	}

	res := checkServedCertificates(ctx, root, false)
	if len(res) != 3 {
		t.Fatalf("want three TLS lines, got %+v", res)
	}
	for _, r := range res {
		host := strings.TrimPrefix(r.Name, "TLS ")
		if want[host] != r.Status {
			t.Errorf("%s: status %q, want %q (%s)", host, r.Status, want[host], r.Message)
		}
	}
	if rep := buildDoctorReport(res); rep.Summary.Failed == 0 {
		t.Errorf("the TLS section must fail: %+v", rep.Summary)
	}
}

// tlsOneHost serves cert for host and returns a single-host own stack whose
// conf names example-com's certificate; sslMode is written to the project .env.
func tlsOneHost(t *testing.T, host, sslMode string, cert tls.Certificate, certPEM string) (string, string) {
	t.Helper()
	tlsIsolateEnv(t)
	root := t.TempDir()
	port, _ := tlsServe(t, map[string]tls.Certificate{host: cert})
	sslDir := tlsStack(t, root, port, map[string]string{host: "example-com"})
	tlsWrite(t, filepath.Join(sslDir, "certificates", "example-com", "fullchain.pem"), certPEM)
	tlsWrite(t, filepath.Join(root, ".env"), "ENV=dev\nPROJECT_NAME=stack\nSSL_MODE="+sslMode+"\nNGINX_BIND_IP=127.0.0.1\nNGINX_HTTPS_PORT="+port+"\n")
	return root, port
}

// TestDoctorTLSNameMismatch pins that a served certificate that does not
// cover the host fails with a human-visible line (wildcards span one label).
func TestDoctorTLSNameMismatch(t *testing.T) {
	for _, tc := range []struct {
		name, host string
		sans       []string
		wantFail   bool
	}{
		{"other name", "m.example.com", []string{"notthishost.example.org"}, true},
		{"wildcard covers one label", "api.example.com", []string{"*.example.com"}, false},
		{"wildcard does not span labels", "a.b.example.com", []string{"*.example.com"}, true},
		{"exact san", "api.example.com", []string{"example.com", "api.example.com"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cert, certPEM, _ := tlsMakeCert(t, tlsDays(60), tc.sans...)
			root, _ := tlsOneHost(t, tc.host, "custom", cert, certPEM)
			res, by := tlsRun(t, root, tlsTestDeps(t))
			r, found := by["TLS "+tc.host+" name"]
			if tc.wantFail && (!found || r.Status != "fail" || !strings.Contains(r.Message, tc.host)) {
				t.Fatalf("want a failing name line, got %+v", res)
			}
			if !tc.wantFail && found {
				t.Fatalf("unexpected name line %+v", r)
			}
		})
	}
}

// TestDoctorTLSChain pins that a chain that does not verify warns under
// SSL_MODE=letsencrypt and stays detail-only for custom certificates.
func TestDoctorTLSChain(t *testing.T) {
	cert, certPEM, _ := tlsMakeCert(t, tlsDays(60), "api.example.com")
	root, _ := tlsOneHost(t, "api.example.com", "letsencrypt", cert, certPEM)
	if _, by := tlsRun(t, root, tlsTestDeps(t)); by["TLS api.example.com chain"].Status != "warn" {
		t.Fatalf("letsencrypt + unverifiable chain must warn: %+v", by)
	}
	root, _ = tlsOneHost(t, "api.example.com", "custom", cert, certPEM)
	res, by := tlsRun(t, root, tlsTestDeps(t))
	if len(res) != 1 || by["TLS api.example.com"].Detail == "" {
		t.Fatalf("custom: one line with the chain error in detail, got %+v", res)
	}
}

// TestDoctorTLSOwnStackCascade pins that the own stack's address comes from
// the full env cascade (.env.local beats .env), as compose publishes it.
func TestDoctorTLSOwnStackCascade(t *testing.T) {
	cert, certPEM, _ := tlsMakeCert(t, tlsDays(60), "api.example.com")
	root, port := tlsOneHost(t, "api.example.com", "custom", cert, certPEM)
	tlsWrite(t, filepath.Join(root, ".env"), "ENV=dev\nPROJECT_NAME=stack\nSSL_MODE=custom\nNGINX_BIND_IP=127.0.0.1\nNGINX_HTTPS_PORT=1\n")
	tlsWrite(t, filepath.Join(root, ".env.local"), "NGINX_HTTPS_PORT="+port+"\n")
	if _, by := tlsRun(t, root, tlsTestDeps(t)); by["TLS api.example.com"].Status != "pass" {
		t.Fatalf(".env.local's port must win: %+v", by)
	}
}

// TestDoctorTLSDeadAddress pins that a dead address is dialled once, not once
// per host.
func TestDoctorTLSDeadAddress(t *testing.T) {
	tlsIsolateEnv(t)
	root := t.TempDir()
	tlsStack(t, root, "9", map[string]string{"a.example.com": "x", "b.example.com": "x", "c.example.com": "x"})
	var dials int32
	d := tlsTestDeps(t)
	d.probe = func(ctx context.Context, addr, sni string, to time.Duration) (ssl.ServedCert, error) {
		atomic.AddInt32(&dials, 1)
		return ssl.ServedCert{}, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("refused")}
	}
	res, _ := tlsRun(t, root, d)
	if dials != 1 || len(res) != 3 {
		t.Fatalf("dials = %d (want 1), results = %d (want 3 failures)", dials, len(res))
	}
	for _, r := range res {
		if r.Status != "fail" || !strings.Contains(r.Message, "127.0.0.1:9") {
			t.Errorf("got %+v", r)
		}
	}
}

// TestDoctorTLSBadConf pins that one unreadable conf warns and the other hosts
// are still checked, and that an http-level ssl_certificate is inherited.
func TestDoctorTLSBadConf(t *testing.T) {
	cert, certPEM, _ := tlsMakeCert(t, tlsDays(60), "api.example.com")
	root, _ := tlsOneHost(t, "api.example.com", "custom", cert, certPEM)
	if err := os.MkdirAll(filepath.Join(root, "nginx", "sites", "broken.conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, by := tlsRun(t, root, tlsTestDeps(t))
	if by["TLS conf"].Status != "warn" || by["TLS api.example.com"].Status != "pass" || len(res) != 2 {
		t.Fatalf("want a conf warning plus the healthy host, got %+v", res)
	}
	tlsWrite(t, filepath.Join(root, "nginx", "conf.d", "inherit.conf"),
		"ssl_certificate /etc/nginx/ssl/certificates/example-com/fullchain.pem;\nserver { listen 443 ssl; server_name inh.example.com; }\n")
	if _, by = tlsRun(t, root, tlsTestDeps(t)); by["TLS inh.example.com"].Name == "" {
		t.Fatalf("an inherited ssl_certificate must not drop the host: %+v", by)
	}
}
