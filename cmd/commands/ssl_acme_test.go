package commands

// ssl_acme_test.go: `trust ssl setup|renew --acme` against a prod-layout
// fixture (project nself-web/backend served by nself-web's nginx) with docker,
// the secret store, the prober and exit replaced by seams; plus the golden
// argv of the untouched certbot paths (TestSSLCertbotUnchanged).

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ssl/acme"
)

// acmeFix is the prod-layout fixture plus the calls its fakes recorded.
type acmeFix struct {
	t                          *testing.T
	root, served, project, ssl string
	mounts                     []docker.Mount
	runs                       []docker.RunSpec
	execs                      [][]string
	secrets, set               map[string]string
	timers, exits              int
	tos                        bool
	asked                      int
}

// acmeTestCert returns a self-signed certificate and key (PEM) for names.
func acmeTestCert(t *testing.T, names []string, notAfter time.Time) (cert, key []byte) {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<60))
	tpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: names[0]}, DNSNames: names,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalECPrivateKey(k)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb})
}

func acmeWriteFile(t *testing.T, p string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// acmeResetFlags restores every flag of the shared ssl commands to its default.
func acmeResetFlags(c *cobra.Command) {
	c.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
}

func newACMEFix(t *testing.T) *acmeFix {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("ACME fixtures use POSIX symlinks and paths")
	}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	f := &acmeFix{t: t, root: root, served: filepath.Join(root, "nself-web"), secrets: map[string]string{"SSL_DNS_CLOUDFLARE_API_TOKEN": "cf-token-0123456789abcdef"}, set: map[string]string{}}
	f.project, f.ssl = filepath.Join(f.served, "backend"), filepath.Join(f.served, "ssl")
	acmeWriteFile(t, filepath.Join(f.project, ".env"), []byte("BASE_DOMAIN=task.nself.org\nENV=dev\nPROJECT_NAME=backend\nADMIN_EMAIL=ops@example.org\nNGINX_FRONTED_BY=nself-web\n"))
	for _, d := range []string{filepath.Join(f.ssl, "certificates"), filepath.Join(f.served, "nginx", "conf.d")} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	key := filepath.Join(root, "age-key.txt")
	acmeWriteFile(t, key, []byte("AGE-SECRET-KEY-FAKE\n"))
	t.Setenv("SECRETS_AGE_KEY_PATH", key)
	t.Setenv("ENV", "dev") // a prod ENV would demand the full prod secret set; the layout is what matters
	for _, k := range []string{"NSELF_ACME_DIRECTORY", "NSELF_ACME_CA_BUNDLE", "NSELF_ACME_NETWORK", "NSELF_ACME_DNS_RESOLVERS", "NSELF_ACME_FAULT", "ACME_EMAIL"} {
		t.Setenv(k, "")
	}
	t.Chdir(f.project)
	f.mounts = []docker.Mount{{Source: f.ssl, Destination: "/etc/nginx/ssl", Type: "bind"}}

	oldIn, oldD := acmeInput, acmeD
	acmeInput = acme.Input{
		FindNginx: func(context.Context, docker.ServiceMatch) (string, error) { return "nself-web-nginx-1", nil },
		Mounts:    func(context.Context, string) ([]docker.Mount, error) { return f.mounts, nil },
		LookPath:  func(string) (string, error) { return "/usr/bin/age", nil },
	}
	acmeD.run = f.fakeLego
	acmeD.exec = func(_ context.Context, _ string, cmd []string) (string, string, error) {
		f.execs = append(f.execs, cmd)
		return "", "", nil
	}
	acmeD.probe = f.fakeProbe
	acmeD.secGet = func(_, _, k string) (string, error) {
		if v, ok := f.secrets[k]; ok {
			return v, nil
		}
		return "", errors.New("no such secret")
	}
	acmeD.secSet = func(_, _, k, v string) error { f.set[k] = v; return nil }
	acmeD.timer = func(string, string) error { f.timers++; return nil }
	acmeD.ask = func(string) bool { f.asked++; return f.tos }
	acmeD.exit = func(int) { f.exits++ }
	t.Cleanup(func() {
		acmeInput, acmeD = oldIn, oldD
		acmeResetFlags(sslSetupCmd)
		acmeResetFlags(sslRenewCmd)
	})
	acmeResetFlags(sslSetupCmd)
	acmeResetFlags(sslRenewCmd)
	return f
}

// fakeLego stands in for the lego container: it writes a certificate for the
// requested names where lego would, and registers an account.
func (f *acmeFix) fakeLego(_ context.Context, s docker.RunSpec) (string, string, error) {
	f.runs = append(f.runs, s)
	var path, email string
	var names []string
	for i, a := range s.Args {
		switch a {
		case "--path":
			path = s.Args[i+1]
		case "--email":
			email = s.Args[i+1]
		case "--domains":
			names = append(names, s.Args[i+1])
		}
	}
	host := filepath.Join(f.ssl, strings.TrimPrefix(path, "/ssl"))
	cert, key := acmeTestCert(f.t, names, time.Now().Add(90*24*time.Hour))
	base := filepath.Join(host, "certificates", strings.ReplaceAll(names[0], "*", "_"))
	acmeWriteFile(f.t, base+".crt", cert)
	acmeWriteFile(f.t, base+".key", key)
	acmeWriteFile(f.t, filepath.Join(host, "accounts", "fake", email, "account.json"), []byte("{}"))
	return "", "", nil
}

// fakeProbe answers with the fingerprint of whatever certificates/<dir> serves now.
func (f *acmeFix) fakeProbe(_ context.Context, _, sni string, _ time.Duration) (ssl.ServedCert, error) {
	dir := "task-nself-org"
	if strings.HasPrefix(sni, "api.") || strings.HasPrefix(sni, "auth.") {
		dir = strings.ReplaceAll(sni, ".", "-")
	}
	b, err := os.ReadFile(filepath.Join(f.ssl, "certificates", dir, "fullchain.pem"))
	if err != nil {
		return ssl.ServedCert{}, err
	}
	blk, _ := pem.Decode(b)
	sum := sha256.Sum256(blk.Bytes)
	return ssl.ServedCert{Host: sni, SHA256: hex.EncodeToString(sum[:])}, nil
}

// run executes `trust ssl <setup|renew>` with flags and returns its output.
func (f *acmeFix) run(cmd *cobra.Command, flags ...string) (string, error) {
	f.t.Helper()
	acmeResetFlags(cmd)
	for _, fl := range flags {
		k, v, hasV := strings.Cut(strings.TrimPrefix(fl, "--"), "=")
		if !hasV {
			v = "true"
		}
		if err := cmd.Flags().Set(k, v); err != nil {
			f.t.Fatalf("flag %s: %v", fl, err)
		}
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetContext(context.Background())
	var err error
	if cmd == sslSetupCmd {
		err = runSSLSetup(cmd, nil)
	} else {
		err = runSSLRenew(cmd, nil)
	}
	return out.String(), err
}

// tree lists every path under root with its size and link target.
func acmeTree(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, _ error) error {
		dest, _ := os.Readlink(p)
		lines = append(lines, fmt.Sprintf("%s %d %s", p, fi.Size(), dest))
		return nil
	})
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func TestACMEDryRunProdLayout(t *testing.T) {
	f := newACMEFix(t)
	before := acmeTree(t, f.root)
	out, err := f.run(sslSetupCmd, "--acme", "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	for _, want := range []string{"served root:       " + f.served, "served ssl dir:    " + f.ssl, "served nginx dir:  " + filepath.Join(f.served, "nginx"),
		"nginx container:   nself-web-nginx-1", f.ssl + " -> /etc/nginx/ssl", "lineage task-nself-org: dns-01 via cloudflare",
		"api.task.nself.org", "certificates/task-nself-org", "dry run: nothing written"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
	if after := acmeTree(t, f.root); after != before || len(f.runs)+len(f.execs)+len(f.set) != 0 {
		t.Errorf("dry run changed the tree or called out:\n%s\n---\n%s", before, after)
	}
}

func TestACMEMountRefusal(t *testing.T) {
	f := newACMEFix(t)
	dir := filepath.Join(f.ssl, "certificates", "task-nself-org")
	_ = os.MkdirAll(dir, 0o750)
	f.mounts = []docker.Mount{{Source: dir, Destination: "/etc/nginx/ssl/certificates/task-nself-org", Type: "bind"}}
	before := acmeTree(t, f.root)
	for _, flags := range [][]string{{"--acme"}, {"--acme", "--dry-run"}, {"--acme", "--adopt-certbot"}} {
		_, err := f.run(sslSetupCmd, flags...)
		if err == nil || !strings.Contains(err.Error(), "E151") || !strings.Contains(err.Error(), "/etc/nginx/ssl/certificates/task-nself-org") {
			t.Fatalf("%v: want E151 naming the mount, got %v", flags, err)
		}
	}
	if acmeTree(t, f.root) != before || len(f.runs)+len(f.execs) != 0 {
		t.Error("a refused preflight wrote or called something")
	}
}

func TestACMESetupNames(t *testing.T) {
	for _, wild := range []bool{false, true} {
		f := newACMEFix(t)
		flags := []string{"--acme", "--agree-tos"}
		want := "task.nself.org api.task.nself.org auth.task.nself.org"
		if wild {
			flags, want = append(flags, "--wildcard"), "*.task.nself.org task.nself.org"
		}
		out, err := f.run(sslSetupCmd, flags...)
		if err != nil {
			t.Fatalf("setup: %v\n%s", err, out)
		}
		if len(f.runs) != 1 {
			t.Fatalf("lego runs = %d", len(f.runs))
		}
		var names []string
		for i, a := range f.runs[0].Args {
			if a == "--domains" {
				names = append(names, f.runs[0].Args[i+1])
			}
		}
		if strings.Join(names, " ") != want {
			t.Errorf("names = %v, want %s", names, want)
		}
		args := strings.Join(f.runs[0].Args, " ")
		if !strings.Contains(args, "--accept-tos") || !strings.Contains(args, "--dns cloudflare") || strings.Contains(args, "cf-token") {
			t.Errorf("args = %s", args)
		}
		if f.runs[0].EnvPass["CF_DNS_API_TOKEN"] != "cf-token-0123456789abcdef" || f.runs[0].Image != acme.LegoImage {
			t.Errorf("spec = %+v", f.runs[0])
		}
		link, err := os.Readlink(filepath.Join(f.ssl, "certificates", "task-nself-org"))
		if err != nil || link != ".task-nself-org.gen-0" {
			t.Errorf("target link = %q %v (want the first generation)", link, err)
		}
		if len(f.execs) != 2 || strings.Join(f.execs[0], " ") != "nginx -t" || strings.Join(f.execs[1], " ") != "nginx -s reload" {
			t.Errorf("execs = %v", f.execs)
		}
		lf, _ := acme.Load(f.ssl)
		if len(lf.Lineages) != 1 || lf.Lineages[0].Name != "task-nself-org" || lf.Lineages[0].LastIssued == nil || lf.Contact != "ops@example.org" {
			t.Errorf("lineages.json = %+v", lf)
		}
		if gi, _ := os.ReadFile(filepath.Join(f.ssl, ".acme", ".gitignore")); string(gi) != "*\n" {
			t.Error(".acme/.gitignore missing")
		}
		if strings.Contains(out, "cf-token") {
			t.Error("credential printed")
		}
	}
}

func TestACMETermsGate(t *testing.T) {
	f := newACMEFix(t)
	if _, err := f.run(sslSetupCmd, "--acme"); err == nil || !strings.Contains(err.Error(), "terms of service") || len(f.runs) != 0 {
		t.Fatalf("issuing without --agree-tos or a TTY must refuse, got %v (runs %d)", err, len(f.runs))
	}
	f.tos = true
	if _, err := f.run(sslSetupCmd, "--acme"); err != nil || f.asked == 0 || !strings.Contains(strings.Join(f.runs[0].Args, " "), "--accept-tos") {
		t.Fatalf("a TTY confirm should accept: %v", err)
	}
}

func TestACMESetupInstallCronOnly(t *testing.T) {
	f := newACMEFix(t)
	if err := acme.Save(f.ssl, acme.File{Contact: "ops@example.org", Lineages: []acme.Lineage{{Name: "task-nself-org", Challenge: "dns-01", DNSProvider: "cloudflare", Targets: []string{"certificates/task-nself-org"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(sslSetupCmd, "--acme", "--install-cron"); err != nil {
		t.Fatal(err)
	}
	if f.timers != 1 || len(f.runs) != 0 {
		t.Errorf("timers=%d lego runs=%d (want 1 and 0)", f.timers, len(f.runs))
	}
	if _, err := f.run(sslSetupCmd, "--acme"); err == nil || !strings.Contains(err.Error(), "already managed") {
		t.Errorf("setup over existing lineages must refuse, got %v", err)
	}
}

func TestACMERenewDueAndForce(t *testing.T) {
	f := newACMEFix(t)
	mk := func(dir string, days int) {
		c, k := acmeTestCert(t, []string{dir + ".example.org"}, time.Now().Add(time.Duration(days)*24*time.Hour))
		acmeWriteFile(t, filepath.Join(f.ssl, "certificates", dir, "fullchain.pem"), c)
		acmeWriteFile(t, filepath.Join(f.ssl, "certificates", dir, "privkey.pem"), k)
	}
	mk("api-task-nself-org", 10)
	mk("auth-task-nself-org", 60)
	lin := func(n string) acme.Lineage {
		return acme.Lineage{Name: n, Domains: []string{strings.ReplaceAll(n, "-", ".")}, Challenge: "dns-01", DNSProvider: "cloudflare", Targets: []string{"certificates/" + n}}
	}
	_ = acme.Save(f.ssl, acme.File{Contact: "ops@example.org", Lineages: []acme.Lineage{lin("api-task-nself-org"), lin("auth-task-nself-org")}})
	acmeWriteFile(t, filepath.Join(f.ssl, ".acme", "accounts", "x", "ops@example.org", "a.json"), []byte("{}"))

	out, err := f.run(sslRenewCmd, "--acme")
	if err != nil || len(f.runs) != 1 || !strings.Contains(strings.Join(f.runs[0].Args, " "), "--domains api.task.nself.org") {
		t.Fatalf("renew: err=%v runs=%d\n%s", err, len(f.runs), out)
	}
	if strings.Contains(strings.Join(f.runs[0].Args, " "), "--accept-tos") {
		t.Error("--accept-tos on an existing account")
	}
	if _, err := f.run(sslRenewCmd, "--acme", "--force"); err != nil || len(f.runs) != 3 {
		t.Fatalf("--force should renew both: err=%v runs=%d", err, len(f.runs))
	}
	out, _ = f.run(sslRenewCmd, "--acme", "--quiet")
	if strings.Contains(out, "lineage") {
		t.Errorf("--quiet printed %q", out)
	}
}

func TestACMERenewStagingInstallsNothing(t *testing.T) {
	f := newACMEFix(t)
	c, k := acmeTestCert(t, []string{"task.nself.org"}, time.Now().Add(60*24*time.Hour))
	acmeWriteFile(t, filepath.Join(f.ssl, "certificates", "task-nself-org", "fullchain.pem"), c)
	acmeWriteFile(t, filepath.Join(f.ssl, "certificates", "task-nself-org", "privkey.pem"), k)
	_ = acme.Save(f.ssl, acme.File{Contact: "ops@example.org", Lineages: []acme.Lineage{{Name: "task-nself-org", Domains: []string{"task.nself.org"}, Challenge: "dns-01", DNSProvider: "cloudflare", Targets: []string{"certificates/task-nself-org"}}}})
	before := acmeServed(t, f)
	if _, err := f.run(sslRenewCmd, "--acme", "--staging", "--agree-tos"); err != nil {
		t.Fatal(err)
	}
	args := strings.Join(f.runs[0].Args, " ")
	if len(f.runs) != 1 || !strings.Contains(args, "--path /ssl/.acme/staging") || !strings.Contains(args, acme.StagingDirectory) {
		t.Errorf("args = %s", args)
	}
	if string(acmeServed(t, f)) != string(before) || len(f.execs) != 0 {
		t.Error("staging installed or reloaded something")
	}
	if _, err := os.Stat(filepath.Join(f.ssl, ".acme", "staging", "certificates", "task.nself.org.crt")); err != nil {
		t.Error("staging certificate not under .acme/staging")
	}
}

func acmeServed(t *testing.T, f *acmeFix) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.ssl, "certificates", "task-nself-org", "fullchain.pem"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestACMEFlagsRequireACME(t *testing.T) {
	f := newACMEFix(t)
	for _, fl := range []string{"--dry-run", "--agree-tos", "--nginx-container=x", "--adopt-certbot"} {
		if _, err := f.run(sslSetupCmd, fl); err == nil || !strings.Contains(err.Error(), "only works with --acme") {
			t.Errorf("%s without --acme: %v", fl, err)
		}
	}
	if _, err := f.run(sslRenewCmd, "--force"); err == nil || !strings.Contains(err.Error(), "only works with --acme") {
		t.Errorf("--force without --acme: %v", err)
	}
}

func TestACMECredentialINI(t *testing.T) {
	ini := func(body string) string {
		p := filepath.Join(t.TempDir(), "cred.ini")
		acmeWriteFile(t, p, []byte(body))
		return p
	}
	p, secs, err := parseDNSCredentialFile(ini("# certbot\n dns_cloudflare_api_token = cf-token-0123456789abcdef \n"))
	if err != nil || p != "cloudflare" || secs["SSL_DNS_CLOUDFLARE_API_TOKEN"] != "cf-token-0123456789abcdef" {
		t.Fatalf("cloudflare INI: %q %v %v", p, secs, err)
	}
	if p, secs, err = parseDNSCredentialFile(ini("aws_access_key_id=AKIAEXAMPLE0000\naws_secret_access_key=sekrit-sekrit-sekrit\n")); err != nil || p != "route53" || len(secs) != 2 {
		t.Fatalf("route53 INI: %q %v %v", p, secs, err)
	}
	bad := map[string]string{
		"global key only": "dns_cloudflare_email = a@b.c\ndns_cloudflare_api_key = globalkey0123456789\n",
		"two providers":   "dns_cloudflare_api_token = cf-token-0123456789abcdef\ndns_digitalocean_token = do-token-0123456789abcdef\n",
		"none":            "# empty\n",
	}
	for name, body := range bad {
		f := newACMEFix(t)
		before := acmeTree(t, f.root)
		_, err := f.run(sslSetupCmd, "--acme", "--adopt-certbot="+t.TempDir(), "--dns-credential-file="+ini(body))
		if err == nil || !strings.Contains(err.Error(), "E151") {
			t.Fatalf("%s: want E151, got %v", name, err)
		}
		if name == "global key only" && !strings.Contains(err.Error(), "Zone:DNS:Edit") {
			t.Errorf("global key refusal lacks the remediation: %v", err)
		}
		for _, secret := range []string{"globalkey0123", "cf-token-0123", "do-token-0123"} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("%s: error leaks a credential value", name)
			}
		}
		if acmeTree(t, f.root) != before || len(f.set) != 0 {
			t.Errorf("%s: something was written", name)
		}
	}
}

func TestACMEUnitsEnvironment(t *testing.T) {
	svc, timer := acmeUnits("/usr/local/bin/nself", "/opt/nself-web/backend", "/home/deploy/.config/nself/age-key.txt", "/home/deploy")
	for _, want := range []string{"Environment=SECRETS_AGE_KEY_PATH=/home/deploy/.config/nself/age-key.txt", "Environment=HOME=/home/deploy",
		"ExecStart=/usr/local/bin/nself trust ssl renew --acme --quiet", "WorkingDirectory=/opt/nself-web/backend"} {
		if !strings.Contains(svc, want) {
			t.Errorf("service unit missing %q", want)
		}
	}
	if !strings.Contains(timer, "OnCalendar=*-*-* 03:30:00") || !strings.Contains(timer, "RandomizedDelaySec=3600") || !strings.Contains(timer, "Persistent=true") {
		t.Errorf("timer = %s", timer)
	}
}

// TestSSLCertbotUnchanged holds the golden argv of every certbot call the
// pre-existing commands make, via a stub certbot on PATH. The --acme work is
// additive: none of these may change.
func TestSSLCertbotUnchanged(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub certbot is a POSIX shell script")
	}
	t.Setenv("NSELF_V15", "")
	t.Setenv("ENV", "dev")
	bin, log := t.TempDir(), filepath.Join(t.TempDir(), "calls.log")
	for _, n := range []string{"certbot", "docker"} {
		acmeWriteFile(t, filepath.Join(bin, n), []byte("#!/bin/sh\necho \""+n+" $*\" >> "+log+"\nexit 0\n"))
		_ = os.Chmod(filepath.Join(bin, n), 0o755)
	}
	t.Setenv("PATH", bin)
	live := t.TempDir()
	for _, d := range []string{"example.org", "add.example.org", "renew.invalid"} {
		acmeWriteFile(t, filepath.Join(live, d, "fullchain.pem"), []byte("c"))
		acmeWriteFile(t, filepath.Join(live, d, "privkey.pem"), []byte("k"))
	}
	old := letsEncryptLiveDir
	letsEncryptLiveDir = live
	t.Cleanup(func() { letsEncryptLiveDir = old })
	project := t.TempDir()
	acmeWriteFile(t, filepath.Join(project, ".env"), []byte("BASE_DOMAIN=example.org\nADMIN_EMAIL=ops@example.org\n"))
	t.Chdir(project)
	t.Cleanup(func() { acmeResetFlags(sslSetupCmd); acmeResetFlags(sslRenewCmd) })

	calls := func(fn func() error) []string {
		_ = os.Remove(log)
		if err := fn(); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(log)
		var out []string
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if strings.HasPrefix(l, "certbot") {
				out = append(out, l)
			}
		}
		return out
	}
	setup := func(flags ...string) func() error {
		return func() error {
			acmeResetFlags(sslSetupCmd)
			for _, f := range flags {
				k, v, _ := strings.Cut(strings.TrimPrefix(f, "--"), "=")
				_ = sslSetupCmd.Flags().Set(k, map[bool]string{true: "true", false: v}[v == ""])
			}
			return runSSLSetup(sslSetupCmd, nil)
		}
	}
	const dns = "certbot certonly --non-interactive --agree-tos --email ops@example.org --dns-cloudflare --dns-cloudflare-credentials /etc/letsencrypt/cloudflare.ini"
	for name, tc := range map[string]struct {
		got  []string
		want string
	}{
		"setup":          {calls(setup()), dns + " -d example.org -d api.example.org -d auth.example.org"},
		"setup wildcard": {calls(setup("--wildcard")), dns + " -d *.example.org -d example.org"},
		"setup cron":     {calls(setup("--install-cron")), dns + " -d example.org -d api.example.org -d auth.example.org"},
		"renew": {calls(func() error {
			acmeResetFlags(sslRenewCmd)
			return runSSLRenew(sslRenewCmd, []string{"renew.invalid"})
		}), "certbot renew --cert-name renew.invalid"},
		"add": {calls(func() error { return runSSLAdd(sslAddCmd, []string{"add.example.org"}) }),
			"certbot certonly --non-interactive --agree-tos --email ops@example.org --webroot --webroot-path /var/www/certbot -d add.example.org"},
	} {
		if len(tc.got) != 1 || tc.got[0] != tc.want {
			t.Errorf("%s: certbot calls = %q, want [%q]", name, tc.got, tc.want)
		}
	}
}

func TestACMENginxFailureRollsBack(t *testing.T) {
	f := newACMEFix(t)
	if _, err := f.run(sslSetupCmd, "--acme", "--agree-tos"); err != nil {
		t.Fatal(err)
	}
	before := acmeServed(t, f)
	acmeD.exec = func(_ context.Context, _ string, cmd []string) (string, string, error) {
		if cmd[len(cmd)-1] == "-t" {
			return "", "nginx: [emerg] cannot load certificate", errors.New("exit status 1")
		}
		return "", "", nil
	}
	_, err := f.run(sslRenewCmd, "--acme", "--force")
	if err == nil || !strings.Contains(err.Error(), "E151") || !strings.Contains(err.Error(), "previous certificates restored") {
		t.Fatalf("want E151 with a restore message, got %v", err)
	}
	if string(acmeServed(t, f)) != string(before) {
		t.Error("the previous generation was not restored")
	}
}

func TestACMEDryRunPrintsUnits(t *testing.T) {
	f := newACMEFix(t)
	_ = acme.Save(f.ssl, acme.File{Contact: "ops@example.org", Lineages: []acme.Lineage{{Name: "x", Targets: []string{"certificates/x"}}}})
	out, err := f.run(sslSetupCmd, "--acme", "--install-cron", "--dry-run")
	if err != nil || f.timers != 0 {
		t.Fatalf("dry run: %v timers=%d", err, f.timers)
	}
	for _, want := range []string{"Environment=SECRETS_AGE_KEY_PATH=" + os.Getenv("SECRETS_AGE_KEY_PATH"), "Environment=HOME=", "ExecStart=", "trust ssl renew --acme --quiet"} {
		if runtime.GOOS == "linux" && !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q\n%s", want, out)
		}
	}
}

func TestACMEVerifyMismatchRollsBack(t *testing.T) {
	f := newACMEFix(t)
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
}

func TestACMERenewUnknownLineage(t *testing.T) {
	f := newACMEFix(t)
	_ = acme.Save(f.ssl, acme.File{Contact: "ops@example.org", Lineages: []acme.Lineage{{Name: "x", Domains: []string{"x.example.org"}, Targets: []string{"certificates/x"}}}})
	acmeResetFlags(sslRenewCmd)
	var out bytes.Buffer
	sslRenewCmd.SetOut(&out)
	sslRenewCmd.SetContext(context.Background())
	_ = sslRenewCmd.Flags().Set("acme", "true")
	if err := runSSLRenew(sslRenewCmd, []string{"nope"}); err == nil || !strings.Contains(err.Error(), `no lineage named "nope"`) {
		t.Fatalf("want an unknown-lineage error, got %v", err)
	}
	_ = acme.Save(f.ssl, acme.File{Contact: "ops@example.org", Lineages: []acme.Lineage{{Name: "x"}}})
	if err := runSSLRenew(sslRenewCmd, nil); err == nil || !strings.Contains(err.Error(), "no domains or targets") {
		t.Fatalf("want a malformed-lineage error, got %v", err)
	}
}
