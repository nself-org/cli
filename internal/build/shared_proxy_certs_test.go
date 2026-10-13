package build

// shared_proxy_certs_test.go — P7-ADOPT-03: certificate copy, ownership and
// removal on a shared proxy (EPIC ADOPT D2, review F4, R2, R5).

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/sharedproxy"
)

// writeCert writes a self-signed fullchain.pem and privkey.pem into dir.
func writeCert(t *testing.T, dir string, notAfter time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "fixture.test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)
	writeFixtureFile(t, filepath.Join(dir, "fullchain.pem"), string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), 0o644)
	writeFixtureFile(t, filepath.Join(dir, "privkey.pem"), string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})), 0o600)
}

// tlsEnv is a fronted project's env with letsencrypt mode, so its confs
// reference the certificate pair it already holds.
func (x *sharedFx) tlsEnv(project, domain string, extra ...string) string {
	return x.frontedEnv(project, domain, append([]string{"SSL_MODE=letsencrypt"}, extra...)...)
}

func (x *sharedFx) served(dir string) string {
	return filepath.Join(x.owner, "ssl", "certificates", dir)
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func TestSharedProxyCerts(t *testing.T) {
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	soon, later := time.Now().Add(48*time.Hour), time.Now().Add(96*time.Hour)

	// F4: the pair a conf references is copied, key 0600, dir 0700, and listed.
	own := filepath.Join(x.backend(), "ssl", "certificates", "b-test")
	writeCert(t, own, soon)
	x.mustBuild(t, x.backend(), x.tlsEnv("bproj", "b.test"))
	if runtime.GOOS != "windows" && (mode(t, x.served("b-test")) != 0o700 || mode(t, filepath.Join(x.served("b-test"), "privkey.pem")) != 0o600) {
		t.Errorf("modes: dir %v key %v", mode(t, x.served("b-test")), mode(t, filepath.Join(x.served("b-test"), "privkey.pem")))
	}
	if !strings.Contains(readText(t, filepath.Join(x.sites(), "bproj.hasura.conf")), "/certificates/b-test/fullchain.pem") {
		t.Fatal("the conf does not reference the copied pair")
	}
	set, _ := sharedproxy.Load(x.shared(), x.owner)
	if reg := set.Find("bproj"); reg == nil || !inSorted(reg.Files, "ssl/certificates/b-test/fullchain.pem") || !inSorted(reg.Files, "ssl/certificates/b-test/privkey.pem") {
		t.Fatalf("registration does not list the pair: %+v", reg)
	}

	// R2: a rotated certificate replaces B's own copy ...
	writeCert(t, own, later)
	x.mustBuild(t, x.backend(), x.tlsEnv("bproj", "b.test"))
	if readText(t, filepath.Join(x.served("b-test"), "fullchain.pem")) != readText(t, filepath.Join(own, "fullchain.pem")) {
		t.Error("a rotated certificate was not copied")
	}
	// ... but a newer pair in the served dir (an ACME renewal) is kept.
	writeCert(t, x.served("b-test"), time.Now().Add(240*time.Hour))
	renewed := readText(t, filepath.Join(x.served("b-test"), "fullchain.pem"))
	x.mustBuild(t, x.backend(), x.tlsEnv("bproj", "b.test"))
	if readText(t, filepath.Join(x.served("b-test"), "fullchain.pem")) != renewed {
		t.Error("a newer served pair was overwritten")
	}

	// R2: a directory that is not this project's, with other bytes, is E503.
	writeCert(t, filepath.Join(x.other(), "ssl", "certificates", "c-test"), soon)
	writeCert(t, x.served("c-test"), soon)
	guard := func() string { return treeDigest(t, filepath.Join(x.owner, "nginx"), filepath.Join(x.owner, "ssl")) }
	before := guard()
	wantCode(t, x.build(t, x.other(), x.tlsEnv("cproj", "c.test")), "E503", "c-test")
	if guard() != before {
		t.Error("the refused cproj build changed the owner's nginx/ssl tree")
	}
	// R2: a symlink in the owner's ssl/certificates is refused, nothing written.
	writeCert(t, filepath.Join(x.other(), "ssl", "certificates", "d-test"), soon)
	if err := os.Symlink(t.TempDir(), x.served("d-test")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	before = guard()
	wantCode(t, x.build(t, x.other(), x.tlsEnv("dproj", "d.test")), "E503", "d-test")
	if guard() != before {
		t.Error("the refused dproj build changed the owner's nginx/ssl tree")
	}
	if _, err := os.Stat(filepath.Join(x.shared(), "dproj.json")); err == nil {
		t.Error("a refused build registered itself")
	}
	if _, err := os.Stat(filepath.Join(x.sites(), "dproj.hasura.conf")); err == nil {
		t.Error("a refused build wrote a conf")
	}
}

// R5: a pair LIVE-15 manages (symlink to a generation dir, recorded in
// lineages.json) is left alone, not copied over and not listed.
func TestSharedProxyCertsAcmeManaged(t *testing.T) {
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	writeCert(t, filepath.Join(x.other(), "ssl", "certificates", "e-test"), time.Now().Add(48*time.Hour))
	gen := filepath.Join(x.owner, "ssl", "certificates", ".e-test.gen-1")
	writeCert(t, gen, time.Now().Add(240*time.Hour))
	if err := os.Symlink(".e-test.gen-1", x.served("e-test")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	writeFixtureFile(t, filepath.Join(x.owner, "ssl", ".acme", "lineages.json"),
		`{"schema_version":"1","contact":"","lineages":[{"name":"e","domains":["e.test"],"targets":["certificates/e-test"]}]}`, 0o600)
	want := readText(t, filepath.Join(gen, "fullchain.pem"))
	x.mustBuild(t, x.other(), x.tlsEnv("eproj", "e.test"))
	if readText(t, filepath.Join(gen, "fullchain.pem")) != want {
		t.Error("the ACME-managed pair was overwritten")
	}
	set, _ := sharedproxy.Load(x.shared(), x.owner)
	reg := set.Find("eproj")
	if reg == nil {
		t.Fatal("eproj did not register")
	}
	for _, f := range reg.Files {
		if strings.HasPrefix(f, "ssl/") {
			t.Errorf("the ACME-managed pair must not be listed: %s", f)
		}
	}
	// Dropping the registration leaves the pair in place.
	x.mustBuild(t, x.other(), x.env("eproj", "e.test", "SSL_MODE=letsencrypt"))
	if readText(t, filepath.Join(gen, "fullchain.pem")) != want {
		t.Error("unregistering removed the ACME-managed pair")
	}
}

func TestSharedProxyUnregister(t *testing.T) {
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	writeCert(t, filepath.Join(x.backend(), "ssl", "certificates", "b-test"), time.Now().Add(48*time.Hour))
	x.mustBuild(t, x.backend(), x.tlsEnv("bproj", "b.test"))
	ownerConf := readText(t, filepath.Join(x.sites(), "hasura.conf"))

	// Unsetting NGINX_FRONTED_BY removes every listed file, then the registration.
	x.mustBuild(t, x.backend(), x.env("bproj", "b.test", "SSL_MODE=letsencrypt"))
	for _, p := range []string{filepath.Join(x.shared(), "bproj.json"), filepath.Join(x.sites(), "bproj.hasura.conf"), x.served("b-test")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should be gone: %v", p, err)
		}
	}
	if readText(t, filepath.Join(x.sites(), "hasura.conf")) != ownerConf {
		t.Error("the owner's own conf changed")
	}

	// The owner dropping shared mode (its _owner.json goes) retires the project too.
	x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test"))
	if _, err := os.Stat(filepath.Join(x.shared(), "bproj.json")); err != nil {
		t.Fatal("bproj did not re-register")
	}
	x.mustBuild(t, x.owner, x.env("owna", "a.test"))
	if _, err := os.Stat(filepath.Join(x.shared(), sharedproxy.OwnerFile)); !os.IsNotExist(err) {
		t.Fatal("the owner's _owner.json should go when NGINX_MODE is unset")
	}
	x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test"))
	if _, err := os.Stat(filepath.Join(x.shared(), "bproj.json")); !os.IsNotExist(err) {
		t.Error("the registration should go once the owner dropped shared mode")
	}
}

// A pair that does not exist locally yet (first build, or ssl/ cleaned).
func TestSharedProxyCertsWithoutSource(t *testing.T) {
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	abs := x.backend()
	sp := &sharedProxy{project: "bproj", workdir: abs, ownerRoot: x.owner, sslDir: filepath.Join(x.owner, "ssl"),
		prev: &sharedproxy.Registration{Files: []string{"ssl/certificates/b-test/fullchain.pem"}}}
	writeCert(t, x.served("b-test"), time.Now().Add(48*time.Hour))
	if a, err := sp.planCert("b-test", false); err != nil || a != certKeep {
		t.Fatalf("own dir without a local pair: %v %v, want keep (never removed as stale)", a, err)
	}
	writeCert(t, x.served("c-test"), time.Now().Add(48*time.Hour))
	if _, err := sp.planCert("c-test", false); err == nil {
		t.Fatal("a foreign dir cannot be proven identical without a local pair: want E503 before any write")
	}
	if a, err := sp.planCert("new-test", false); err != nil || a != certCopy {
		t.Fatalf("absent dir: %v %v, want copy", a, err)
	}
}

// PROJECT_NAME equal to one of its own conf stems would overwrite the owner's file.
func TestSharedProxyProjectNameCollision(t *testing.T) {
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	before := treeDigest(t, filepath.Join(x.owner, "nginx"))
	wantCode(t, x.build(t, x.backend(), x.frontedEnv("hasura", "b.test")), "E503", "hasura.conf")
	if treeDigest(t, filepath.Join(x.owner, "nginx")) != before {
		t.Error("the refused build changed the owner's nginx tree")
	}
}

// Without nginx/shared/ a fronted project stays in legacy mode: bare names and upstreams.
func TestSharedProxyLegacyFrontedUnchanged(t *testing.T) {
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.env("owna", "a.test")) // no NGINX_MODE
	x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test"))
	conf := readText(t, filepath.Join(x.sites(), "hasura.conf"))
	if !strings.Contains(conf, "http://hasura:8080") || strings.Contains(conf, "(project") {
		t.Errorf("legacy fronted conf changed:\n%s", conf)
	}
	if _, err := os.Stat(filepath.Join(x.sites(), "bproj.hasura.conf")); err == nil {
		t.Error("a prefixed file appeared without nginx/shared/")
	}
	if _, err := os.Stat(x.shared()); err == nil {
		t.Error("nginx/shared/ appeared without NGINX_MODE=shared")
	}
}

// An owner ssl/certificates that cannot be written fails the build before any conf is written.
func TestSharedProxyCertsUnwritableOwner(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory modes do not stop writes here (Windows, or root)")
	}
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	certs := filepath.Join(x.owner, "ssl", "certificates")
	if err := os.MkdirAll(certs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(certs, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(certs, 0o755) })
	writeCert(t, filepath.Join(x.backend(), "ssl", "certificates", "b-test"), time.Now().Add(48*time.Hour))
	before := treeDigest(t, filepath.Join(x.owner, "nginx"))
	wantCode(t, x.build(t, x.backend(), x.tlsEnv("bproj", "b.test")), "E503", "not writable")
	if treeDigest(t, filepath.Join(x.owner, "nginx")) != before {
		t.Error("a refused build wrote into the owner's nginx tree")
	}
}
