package acme

// install_test.go: the pair switch (crash safety, rollback, one reload),
// preflight mount refusal, lego argv, hooks, adoption planning and lineages.
// No docker and no network; certificates are generated in the test.

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
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/docker"
)

// testPair returns a self-signed certificate (PEM) for names and its key (PEM).
func testPair(t *testing.T, names []string, notAfter time.Time) (cert, key []byte) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<60))
	tpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: names[0]}, DNSNames: names,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalECPrivateKey(k)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb})
}

func writePair(t *testing.T, dir string, cert, key []byte) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for n, b := range map[string][]byte{"fullchain.pem": cert, "privkey.pem": key} {
		if err := os.WriteFile(filepath.Join(dir, n), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// served returns the certificate PEM nginx would read through the target link.
func served(t *testing.T, ssl, target string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(ssl, target, "fullchain.pem"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type fakeReloader struct {
	calls int
	err   error
	check func()
}

func (f *fakeReloader) Reload(context.Context) error {
	f.calls++
	if f.check != nil {
		f.check()
	}
	return f.err
}

const tgt = "certificates/api-example-org"

// posixOnly skips tests of the symlink switch and file modes: the ACME path
// targets Linux (Windows runs the CLI under WSL2), and renaming over a
// directory symlink is not atomic there.
func posixOnly(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlink generation switch and POSIX modes")
	}
}

// fixture: a real directory holding pair A, as on a box certbot copied into.
func fixture(t *testing.T) (ssl string, certA []byte) {
	t.Helper()
	ssl = t.TempDir()
	certA, keyA := testPair(t, []string{"api.example.org"}, time.Now().Add(10*24*time.Hour))
	writePair(t, filepath.Join(ssl, tgt), certA, keyA)
	return ssl, certA
}

func TestACMEReloaderOnce(t *testing.T) {
	posixOnly(t)
	ssl, certA := fixture(t)
	certB, keyB := testPair(t, []string{"api.example.org"}, time.Now().Add(90*24*time.Hour))
	rel := &fakeReloader{}
	rel.check = func() {
		if string(served(t, ssl, tgt)) != string(certB) {
			t.Error("reload ran before the switch")
		}
	}
	gens, err := Install(context.Background(), InstallReq{SSLDir: ssl, Targets: []string{tgt}, Cert: certB, Key: keyB, Reloader: rel})
	if err != nil || rel.calls != 1 || gens[tgt] != 1 {
		t.Fatalf("err=%v reloads=%d gens=%v", err, rel.calls, gens)
	}
	if fi, _ := os.Lstat(filepath.Join(ssl, tgt)); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("target is not a symlink after the first install")
	}
	if string(served(t, ssl, filepath.Dir(tgt)+"/.api-example-org.gen-0")) != string(certA) {
		t.Error("previous generation was not kept")
	}
	if fi, _ := os.Stat(filepath.Join(ssl, ".acme")); fi != nil {
		t.Error("install must not create .acme")
	}
}

func TestACMEInstallCrash(t *testing.T) {
	posixOnly(t)
	ssl, certA := fixture(t)
	certB, keyB := testPair(t, []string{"api.example.org"}, time.Now().Add(90*24*time.Hour))
	type killed struct{}
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("fault hook did not fire")
			} else if _, ok := r.(killed); !ok {
				panic(r)
			}
		}()
		_, _ = Install(context.Background(), InstallReq{SSLDir: ssl, Targets: []string{tgt}, Cert: certB, Key: keyB,
			Reloader: &fakeReloader{}, Fault: "after-generation-write", Exit: func(int) { panic(killed{}) }})
	}()
	// The kill came after the generation was written and before the link rename.
	if string(served(t, ssl, tgt)) != string(certA) || !pairOK(filepath.Join(ssl, tgt)) {
		t.Fatal("served pair changed or mismatched after a kill before the switch")
	}
	// Worse: the kill landed between renaming the directory away and linking.
	if err := os.Remove(filepath.Join(ssl, tgt)); err != nil {
		t.Fatal(err)
	}
	if err := Repair(ssl, []string{tgt}); err != nil || !pairOK(filepath.Join(ssl, tgt)) {
		t.Fatalf("repair did not restore a matching pair: %v", err)
	}
	c, k := mustLoad(t, filepath.Join(ssl, tgt))
	if _, err := tls.X509KeyPair(c, k); err != nil {
		t.Fatal(err)
	}
	// A mismatched pair is repaired from the newest valid generation too.
	_, keyOther := testPair(t, []string{"x.example.org"}, time.Now().Add(time.Hour))
	cur, _ := os.Readlink(filepath.Join(ssl, tgt))
	if err := os.WriteFile(filepath.Join(ssl, filepath.Dir(tgt), cur, "privkey.pem"), keyOther, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Repair(ssl, []string{tgt}); err != nil || !pairOK(filepath.Join(ssl, tgt)) {
		t.Fatalf("mismatch not repaired: %v", err)
	}
	// And the next install still works.
	if _, err := Install(context.Background(), InstallReq{SSLDir: ssl, Targets: []string{tgt}, Cert: certB, Key: keyB, Reloader: &fakeReloader{}}); err != nil {
		t.Fatal(err)
	}
	if string(served(t, ssl, tgt)) != string(certB) {
		t.Error("install after recovery did not switch")
	}
}

func mustLoad(t *testing.T, dir string) (c, k []byte) {
	t.Helper()
	c, _ = os.ReadFile(filepath.Join(dir, "fullchain.pem"))
	k, _ = os.ReadFile(filepath.Join(dir, "privkey.pem"))
	return c, k
}

func TestACMEInstallRollback(t *testing.T) {
	posixOnly(t)
	ssl, certA := fixture(t)
	certB, keyB := testPair(t, []string{"api.example.org"}, time.Now().Add(90*24*time.Hour))
	for name, req := range map[string]InstallReq{
		"nginx -t fails": {Reloader: &fakeReloader{err: errors.New("nginx: [emerg] bad")}},
		"verify fails":   {Reloader: &fakeReloader{}, Verify: func(context.Context) error { return errors.New("wrong fingerprint") }},
	} {
		req.SSLDir, req.Targets, req.Cert, req.Key = ssl, []string{tgt}, certB, keyB
		if _, err := Install(context.Background(), req); err == nil || !strings.Contains(err.Error(), "restored") {
			t.Fatalf("%s: want a restored-previous error, got %v", name, err)
		}
		if string(served(t, ssl, tgt)) != string(certA) || !pairOK(filepath.Join(ssl, tgt)) {
			t.Fatalf("%s: previous generation not restored", name)
		}
	}
	// A mismatched key is refused before anything is switched.
	_, other := testPair(t, []string{"x.example.org"}, time.Now().Add(time.Hour))
	rel := &fakeReloader{}
	if _, err := Install(context.Background(), InstallReq{SSLDir: ssl, Targets: []string{tgt}, Cert: certB, Key: other, Reloader: rel}); err == nil || rel.calls != 0 {
		t.Fatalf("mismatched pair accepted (err=%v reloads=%d)", err, rel.calls)
	}
}

func TestACMEMountRefusal(t *testing.T) {
	ssl := t.TempDir()
	key := filepath.Join(t.TempDir(), "age.txt")
	_ = os.WriteFile(key, []byte("k"), 0o600)
	in := Input{ProjectDir: filepath.Join(ssl, "p"), NginxContainer: "web-nginx-1", Contact: "a@example.org", AgeKeyPath: key,
		LookPath: func(string) (string, error) { return "/usr/bin/age", nil }}
	in.ProjectDir = filepath.Dir(ssl) // project dir whose ssl dir is <parent>/ssl
	ssl = filepath.Join(in.ProjectDir, "ssl")
	_ = os.MkdirAll(filepath.Join(ssl, "certificates", "api-example-org"), 0o750)
	for name, mounts := range map[string][]docker.Mount{
		"dir mount":  {{Source: filepath.Join(ssl, "certificates", "api-example-org"), Destination: "/etc/nginx/ssl/certificates/api-example-org"}},
		"file mount": {{Source: filepath.Join(ssl, "certificates", "api-example-org", "fullchain.pem"), Destination: "/etc/nginx/ssl/certificates/api-example-org/fullchain.pem"}},
		"none":       {},
		"whole dir plus a nested mount that shadows it": {{Source: ssl, Destination: "/etc/nginx/ssl"},
			{Source: filepath.Join(ssl, "certificates", "api-example-org"), Destination: "/etc/nginx/ssl/certificates/api-example-org"}},
	} {
		in.Mounts = func(context.Context, string) ([]docker.Mount, error) { return mounts, nil }
		_, err := Resolve(context.Background(), in)
		var ae *Error
		if !errors.As(err, &ae) || !strings.Contains(ae.What, "web-nginx-1") {
			t.Fatalf("%s: want a refusal naming the container, got %v", name, err)
		}
		if name != "none" && !strings.Contains(ae.What, "/etc/nginx/ssl/certificates/api-example-org") {
			t.Errorf("%s: refusal does not name the mount: %s", name, ae.What)
		}
	}
	in.Mounts = func(context.Context, string) ([]docker.Mount, error) {
		return []docker.Mount{{Source: ssl, Destination: "/etc/nginx/ssl", ReadOnly: true}}, nil
	}
	if r, err := Resolve(context.Background(), in); err != nil || r.Container != "web-nginx-1" {
		t.Fatalf("whole-dir mount refused: %v", err)
	}
}

func TestACMEHooksAndArgs(t *testing.T) {
	get := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	for name, m := range map[string]map[string]string{
		"fault with production": {"NSELF_ACME_FAULT": "after-generation-write"},
		"upper case host":       {"NSELF_ACME_DIRECTORY": "https://ACME-V02.API.LETSENCRYPT.ORG/directory", "NSELF_ACME_FAULT": "after-generation-write"},
		"letsencrypt directory": {"NSELF_ACME_DIRECTORY": "https://acme-v02.api.letsencrypt.org/directory"},
		"staging letsencrypt":   {"NSELF_ACME_DIRECTORY": "https://acme-staging-v02.api.letsencrypt.org/directory", "NSELF_ACME_DNS_RESOLVERS": "x:1"},
	} {
		if _, err := HooksFromEnv(get(m)); err == nil {
			t.Errorf("%s: hook accepted against Let's Encrypt", name)
		}
	}
	h, err := HooksFromEnv(get(map[string]string{"NSELF_ACME_DIRECTORY": "https://pebble:14000/dir", "NSELF_ACME_DNS_RESOLVERS": "chall:8053"}))
	if err != nil || h.Resolvers != "chall:8053" {
		t.Fatalf("pebble hooks refused: %v", err)
	}
	var got docker.RunSpec
	ssl := t.TempDir()
	_, _, err = Issue(context.Background(), IssueReq{SSLDir: ssl, Name: "n", Provider: "cloudflare", Contact: "a@example.org",
		Domains: []string{"*.example.org", "example.org"}, Hooks: h, AcceptTOS: true,
		Secret: func(string) (string, error) { return "tok-123456789012", nil },
		Run:    func(_ context.Context, s docker.RunSpec) (string, string, error) { got = s; return "", "", nil }})
	if err != nil {
		t.Fatal(err)
	}
	want := "--path /ssl/.acme --email a@example.org --server https://pebble:14000/dir --key-type ec256 --dns cloudflare --dns.resolvers chall:8053 --dns.propagation-disable-ans --domains *.example.org --domains example.org --accept-tos run"
	if g := strings.Join(got.Args, " "); g != want {
		t.Errorf("args = %q", g)
	}
	if got.EnvPass["CF_DNS_API_TOKEN"] != "tok-123456789012" || strings.Contains(strings.Join(got.Args, " "), "tok-1234") {
		t.Error("credential must be in EnvPass only")
	}
	if got.Image != LegoImage || !strings.Contains(LegoImage, "@sha256:") {
		t.Error("lego image is not digest pinned")
	}
	if _, _, err := Issue(context.Background(), IssueReq{SSLDir: ssl, Provider: "exec", Domains: []string{"a"}}); err == nil {
		t.Error("provider exec accepted without a test directory")
	}
}

func TestACMELineagesAndPlan(t *testing.T) {
	ssl := t.TempDir()
	f := File{Contact: "a@example.org"}
	f.Lineages = []Lineage{{Name: "b", Targets: []string{"certificates/b"}}, {Name: "a", Domains: []string{"x"}}}
	if err := Save(ssl, f); err != nil {
		t.Fatal(err)
	}
	if gi, _ := os.ReadFile(filepath.Join(ssl, ".acme", ".gitignore")); string(gi) != "*\n" {
		t.Errorf(".gitignore = %q", gi)
	}
	if fi, _ := os.Stat(filepath.Join(ssl, ".acme", "lineages.json")); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("lineages.json mode %v", fi.Mode().Perm())
	}
	got, err := Load(ssl)
	if err != nil || len(got.Lineages) != 2 || got.Lineages[0].Name != "a" || got.SchemaVersion != "1" {
		t.Fatalf("load: %+v %v", got, err)
	}
	now := time.Now()
	if !Due(now.Add(10*24*time.Hour), now, false) || Due(now.Add(60*24*time.Hour), now, false) || !Due(now.Add(60*24*time.Hour), now, true) {
		t.Error("Due thresholds wrong")
	}
	if ad, refused := Plan([]Certbot{{Name: "x", Authenticator: "dns-cloudflare", Domains: []string{"x.example.org"}}},
		map[string]string{"x.example.org": "/etc/nginx/ssl/fullchain.pem"}, "", ""); len(ad) != 0 || len(refused) != 1 || !strings.Contains(refused[0], `"."`) {
		t.Errorf("a conf pointing at the ssl dir itself must refuse the lineage: %+v %v", ad, refused)
	}
	hosts := map[string]string{
		"api.example.org":  "/etc/nginx/ssl/certificates/api-example-org/fullchain.pem",
		"auth.example.org": "/etc/nginx/ssl/certificates/auth-example-org/fullchain.pem",
		"x.example.org":    "$ssl/fullchain.pem",
	}
	cbs := []Certbot{
		{Name: "api", Authenticator: "dns-cloudflare", Conf: "/c/api.conf", Domains: []string{"api.example.org"}},
		{Name: "auth", Authenticator: "standalone", Conf: "/c/auth.conf", Domains: []string{"auth.example.org"}},
		{Name: "wild", Authenticator: "dns-google", Domains: []string{"*.example.org"}},
	}
	ad, refused := Plan(cbs, hosts, "", "")
	if len(ad) != 1 || ad[0].Name != "api" || ad[0].Targets[0] != "certificates/api-example-org" || ad[0].DNSProvider != "cloudflare" {
		t.Fatalf("plan without credential: %+v", ad)
	}
	if len(refused) != 2 || !strings.Contains(refused[0], "--dns-credential-file") {
		t.Fatalf("refusals: %v", refused)
	}
	ad, refused = Plan(cbs[:2], hosts, "cloudflare", "")
	if len(ad) != 2 || len(refused) != 0 || ad[1].DNSProvider != "cloudflare" || ad[1].Challenge != "dns-01" {
		t.Fatalf("plan with credential: %+v %v", ad, refused)
	}
	if ad, _ = Plan(cbs[:2], hosts, "cloudflare", "auth"); len(ad) != 1 || ad[0].Name != "auth" {
		t.Errorf("--lineage filter: %+v", ad)
	}
}

func TestACMELockSerializesInstall(t *testing.T) {
	posixOnly(t)
	ssl, _ := fixture(t)
	for round := 0; round < 200; round++ {
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i := range errs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				c, k := testPair(t, []string{"api.example.org"}, time.Now().Add(90*24*time.Hour))
				release, err := Lock(ssl, time.Minute)
				if err != nil {
					errs[i] = err
					return
				}
				defer release()
				_, errs[i] = Install(context.Background(), InstallReq{SSLDir: ssl, Targets: []string{tgt}, Cert: c, Key: k, Reloader: &fakeReloader{}})
			}(i)
		}
		wg.Wait()
		if errs[0] != nil || errs[1] != nil || !pairOK(filepath.Join(ssl, tgt)) {
			t.Fatalf("round %d: errs=%v pair ok=%v", round, errs, pairOK(filepath.Join(ssl, tgt)))
		}
	}
}

func TestACMELockContention(t *testing.T) {
	posixOnly(t)
	ssl := t.TempDir()
	release, err := Lock(ssl, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Lock(ssl, 150*time.Millisecond)
	var ae *Error
	if !errors.As(err, &ae) || !strings.Contains(ae.What, "another") || !strings.Contains(ae.What, strconv.Itoa(os.Getpid())) {
		t.Fatalf("want a refusal naming the holder pid, got %v", err)
	}
	release()
	if r2, err := Lock(ssl, time.Second); err != nil {
		t.Fatalf("lock not released: %v", err)
	} else {
		r2()
	}
}

func TestACMEValidTarget(t *testing.T) {
	for _, ok := range []string{"certificates/api-example-org", "certificates/a.b_c-1"} {
		if !ValidTarget(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{".", "", "certificates", "../x", "/abs/path", "/etc/nginx/ssl/certificates/x", "certificates/.x.gen-1", "certificates/a/b", "certificates/..", "certificates/a..b", "other/x", "certificates/"} {
		if ValidTarget(bad) {
			t.Errorf("%q should be refused", bad)
		}
	}
	ssl := t.TempDir()
	outside := filepath.Join(filepath.Dir(ssl), "outside-"+filepath.Base(ssl))
	c, k := testPair(t, []string{"a.example.org"}, time.Now().Add(time.Hour))
	for _, bad := range []string{".", "../" + filepath.Base(outside), "certificates"} {
		if _, err := Install(context.Background(), InstallReq{SSLDir: ssl, Targets: []string{bad}, Cert: c, Key: k, Reloader: &fakeReloader{}}); err == nil || !strings.Contains(err.Error(), "refusing target") {
			t.Errorf("Install accepted target %q: %v", bad, err)
		}
	}
	if _, err := os.Stat(outside); err == nil {
		t.Error("Install touched a directory outside the ssl dir")
	}
	if err := os.MkdirAll(filepath.Join(ssl, ".acme"), 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(ssl, ".acme", "lineages.json"), []byte(`{"lineages":[{"name":"x","targets":["../x"]}]}`), 0o600)
	if _, err := Load(ssl); err == nil || !strings.Contains(err.Error(), "invalid target") {
		t.Errorf("Load accepted an invalid target: %v", err)
	}
}

func TestACMEInstallWritesChain(t *testing.T) {
	posixOnly(t)
	ssl := t.TempDir()
	leaf, key := testPair(t, []string{"api.example.org"}, time.Now().Add(90*24*time.Hour))
	issuer, _ := testPair(t, []string{"Issuer CA"}, time.Now().Add(365*24*time.Hour))
	full := append(append([]byte{}, leaf...), issuer...)
	gens, err := Install(context.Background(), InstallReq{SSLDir: ssl, Targets: []string{tgt}, Cert: full, Key: key, Reloader: &fakeReloader{}})
	if err != nil || gens[tgt] != 0 {
		t.Fatalf("install: %v %v", err, gens)
	}
	dir := filepath.Join(ssl, tgt)
	for name, want := range map[string][]byte{"fullchain.pem": full, "cert.pem": leaf, "chain.pem": issuer, "privkey.pem": key} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != string(want) {
			t.Errorf("%s: err=%v, content differs from the expected PEM", name, err)
		}
		if fi, _ := os.Stat(filepath.Join(dir, name)); fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v", name, fi.Mode().Perm())
		}
	}
}

func TestACMENonASCIIName(t *testing.T) {
	_, _, err := Issue(context.Background(), IssueReq{SSLDir: t.TempDir(), Provider: "cloudflare", Contact: "a@example.org", Domains: []string{"bücher.example"},
		Secret: func(string) (string, error) { return "tok-123456789012", nil },
		Run: func(context.Context, docker.RunSpec) (string, string, error) {
			t.Error("lego must not run")
			return "", "", nil
		}})
	var ae *Error
	if !errors.As(err, &ae) || !strings.Contains(ae.Fix, "punycode") {
		t.Fatalf("want a punycode remediation, got %v", err)
	}
}
