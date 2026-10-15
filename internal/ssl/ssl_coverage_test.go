package ssl

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

	"github.com/nself-org/cli/internal/config"
)

// ─── DomainToDirName ────────────────────────────────────────────────────────

func TestDomainToDirName_Basic(t *testing.T) {
	cases := []struct{ in, want string }{
		{"nself.org", "nself-org"},
		{"localhost", "localhost"},
		{"api.nself.org", "api-nself-org"},
		{"", "localhost"},
		{"a.b.c.d", "a-b-c-d"},
	}
	for _, c := range cases {
		got := DomainToDirName(c.in)
		if got != c.want {
			t.Errorf("DomainToDirName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ─── fileExists ─────────────────────────────────────────────────────────────

func TestFileExists_True(t *testing.T) {
	f, err := os.CreateTemp("", "ssl-file-exists-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()

	if !fileExists(f.Name()) {
		t.Errorf("fileExists(%q) = false, want true", f.Name())
	}
}

func TestFileExists_FalseNotExist(t *testing.T) {
	if fileExists("/tmp/ssl-no-such-file-99999.pem") {
		t.Error("fileExists on nonexistent path should return false")
	}
}

func TestFileExists_FalseDirectory(t *testing.T) {
	dir := t.TempDir()
	// fileExists returns false for a directory
	if fileExists(dir) {
		t.Errorf("fileExists(%q) = true for a directory, want false", dir)
	}
}

// ─── hostsManualNote ────────────────────────────────────────────────────────

func TestHostsManualNote_ContainsExpectedContent(t *testing.T) {
	hosts := []string{"api.local", "app.local"}
	note := hostsManualNote(hosts)

	if !strings.Contains(note, "127.0.0.1") {
		t.Error("hostsManualNote should contain 127.0.0.1")
	}
	for _, h := range hosts {
		if !strings.Contains(note, h) {
			t.Errorf("hostsManualNote missing hostname %q", h)
		}
	}
	if !strings.Contains(note, "nself dns-setup") {
		t.Error("hostsManualNote should reference nself dns-setup")
	}
}

func TestHostsManualNote_SingleHost(t *testing.T) {
	note := hostsManualNote([]string{"myapp.local"})
	if note == "" {
		t.Error("hostsManualNote should not return empty string")
	}
	if !strings.Contains(note, "myapp.local") {
		t.Error("hostsManualNote should include the provided hostname")
	}
}

// ─── buildOpenSSLConfig ─────────────────────────────────────────────────────

func TestBuildOpenSSLConfig_ContainsDNS(t *testing.T) {
	domains := []string{"localhost", "myapp.local", "127.0.0.1"}
	conf := buildOpenSSLConfig(domains)

	if !strings.Contains(conf, "[req]") {
		t.Error("buildOpenSSLConfig: missing [req] section")
	}
	if !strings.Contains(conf, "[alt_names]") {
		t.Error("buildOpenSSLConfig: missing [alt_names] section")
	}
	// localhost and myapp.local should be DNS entries
	if !strings.Contains(conf, "DNS.1 = localhost") {
		t.Errorf("buildOpenSSLConfig: expected DNS.1 = localhost\ngot:\n%s", conf)
	}
	// 127.0.0.1 is an IP address so should appear as IP.N
	if !strings.Contains(conf, "IP.1 = 127.0.0.1") {
		t.Errorf("buildOpenSSLConfig: expected IP.1 = 127.0.0.1\ngot:\n%s", conf)
	}
	// CN should be the first domain
	if !strings.Contains(conf, "CN = localhost") {
		t.Errorf("buildOpenSSLConfig: expected CN = localhost\ngot:\n%s", conf)
	}
}

func TestBuildOpenSSLConfig_IPv6(t *testing.T) {
	domains := []string{"myapp.local", "::1"}
	conf := buildOpenSSLConfig(domains)
	// ::1 contains a colon so isIPAddress returns true → IP entry
	if !strings.Contains(conf, "IP.1 = ::1") {
		t.Errorf("buildOpenSSLConfig: expected IP.1 = ::1\ngot:\n%s", conf)
	}
}

func TestBuildOpenSSLConfig_MultipleDNS(t *testing.T) {
	domains := []string{"a.local", "b.local", "c.local"}
	conf := buildOpenSSLConfig(domains)
	if !strings.Contains(conf, "DNS.1 = a.local") {
		t.Errorf("buildOpenSSLConfig: expected DNS.1 = a.local\ngot:\n%s", conf)
	}
	if !strings.Contains(conf, "DNS.2 = b.local") {
		t.Errorf("buildOpenSSLConfig: expected DNS.2 = b.local\ngot:\n%s", conf)
	}
	if !strings.Contains(conf, "DNS.3 = c.local") {
		t.Errorf("buildOpenSSLConfig: expected DNS.3 = c.local\ngot:\n%s", conf)
	}
}

// ─── isIPAddress edge cases ──────────────────────────────────────────────────

func TestIsIPAddress_IPv6(t *testing.T) {
	if !isIPAddress("::1") {
		t.Error("isIPAddress(::1) should be true")
	}
	if !isIPAddress("2001:db8::1") {
		t.Error("isIPAddress(2001:db8::1) should be true")
	}
}

func TestIsIPAddress_IPv4(t *testing.T) {
	if !isIPAddress("127.0.0.1") {
		t.Error("isIPAddress(127.0.0.1) should be true")
	}
	if !isIPAddress("192.168.1.1") {
		t.Error("isIPAddress(192.168.1.1) should be true")
	}
}

func TestIsIPAddress_FalseDomain(t *testing.T) {
	if isIPAddress("localhost") {
		t.Error("isIPAddress(localhost) should be false")
	}
	if isIPAddress("api.nself.org") {
		t.Error("isIPAddress(api.nself.org) should be false")
	}
}

func TestIsIPAddress_FalseNonNumericOctet(t *testing.T) {
	// Looks like 4 parts but one is non-numeric
	if isIPAddress("api.db.nself.org") {
		t.Error("isIPAddress(api.db.nself.org) should be false")
	}
}

// ─── filterHostsEntries ─────────────────────────────────────────────────────

func TestFilterHostsEntries_XipIo(t *testing.T) {
	result := filterHostsEntries([]string{"foo.xip.io", "valid.local"})
	for _, h := range result {
		if h == "foo.xip.io" {
			t.Error("filterHostsEntries should exclude .xip.io entries")
		}
	}
	found := false
	for _, h := range result {
		if h == "valid.local" {
			found = true
		}
	}
	if !found {
		t.Error("filterHostsEntries should keep valid.local")
	}
}

func TestFilterHostsEntries_EmptyAndWhitespace(t *testing.T) {
	result := filterHostsEntries([]string{"", "  ", "valid.local"})
	for _, h := range result {
		if strings.TrimSpace(h) == "" {
			t.Error("filterHostsEntries should exclude empty/whitespace entries")
		}
	}
	if len(result) != 1 || result[0] != "valid.local" {
		t.Errorf("filterHostsEntries expected [valid.local], got %v", result)
	}
}

func TestFilterHostsEntries_InternalStar(t *testing.T) {
	// Wildcard that contains * but doesn't start with *.
	result := filterHostsEntries([]string{"foo*.local", "ok.local"})
	for _, h := range result {
		if h == "foo*.local" {
			t.Error("filterHostsEntries should exclude entries containing *")
		}
	}
}

// ─── readHostsFile error branches ────────────────────────────────────────────

func TestReadHostsFile_PermissionError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped on Windows: chmod 0000 is not enforced")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root — permission test meaningless")
	}
	f, err := os.CreateTemp("", "hosts-perm-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()
	// Remove all permissions so Open fails with EACCES
	_ = os.Chmod(f.Name(), 0000)
	defer func() { _ = os.Chmod(f.Name(), 0644) }()

	_, err = readHostsFile(f.Name())
	if err == nil {
		t.Error("expected error for unreadable file, got nil")
	}
}

func TestReadHostsFile_NonexistentNotPermission(t *testing.T) {
	// A path that doesn't exist — readHostsFile wraps with "opening ..."
	_, err := readHostsFile("/tmp/nself-no-such-hosts-file-test-99999")
	if err == nil {
		t.Error("expected error for nonexistent path, got nil")
	}
	// Should NOT be ErrSudoRequired (no EACCES for nonexistent)
	if err == ErrSudoRequired {
		t.Error("expected non-sudo-required error for nonexistent path")
	}
}

// ─── writeHostsFile error branch ─────────────────────────────────────────────

func TestWriteHostsFile_PermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — permission test meaningless")
	}
	f, err := os.CreateTemp("", "hosts-write-perm-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()
	_ = os.Chmod(f.Name(), 0000)
	defer func() { _ = os.Chmod(f.Name(), 0644) }()

	err = writeHostsFile(f.Name(), "127.0.0.1 test.local\n")
	if err != ErrSudoRequired {
		t.Errorf("expected ErrSudoRequired, got: %v", err)
	}
}

func TestWriteHostsFile_NonPermissionError(t *testing.T) {
	// Write to a directory path so WriteFile fails (EISDIR on linux / ENOENT on macOS for dir itself)
	dir := t.TempDir()
	// Try to write to a path that is a directory
	err := writeHostsFile(dir, "content")
	if err == nil {
		t.Error("expected error writing to a directory path, got nil")
	}
	// Should not be ErrSudoRequired for EISDIR
	if err == ErrSudoRequired {
		t.Error("expected non-sudo error for EISDIR, got ErrSudoRequired")
	}
}

// ─── CheckCertExpiry ─────────────────────────────────────────────────────────

// writeSelfSignedCert writes a minimal self-signed cert to a temp file and
// returns the path.
func writeSelfSignedCert(t *testing.T, notBefore, notAfter time.Time) string {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-cert"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.CreateTemp("", "test-cert-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	t.Cleanup(func() { _ = os.Remove(f.Name()) })
	return f.Name()
}

func TestCheckCertExpiry_ValidCert(t *testing.T) {
	now := time.Now()
	path := writeSelfSignedCert(t, now.Add(-24*time.Hour), now.Add(90*24*time.Hour))
	days, err := CheckCertExpiry(path)
	if err != nil {
		t.Fatalf("CheckCertExpiry: unexpected error: %v", err)
	}
	if days < 85 || days > 91 {
		t.Errorf("expected ~90 days remaining, got %d", days)
	}
}

func TestCheckCertExpiry_ExpiredCert(t *testing.T) {
	now := time.Now()
	path := writeSelfSignedCert(t, now.Add(-365*24*time.Hour), now.Add(-24*time.Hour))
	_, err := CheckCertExpiry(path)
	if err == nil {
		t.Error("expected error for expired cert, got nil")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected 'expired' in error message, got: %v", err)
	}
}

func TestCheckCertExpiry_WarnBranch(t *testing.T) {
	// Cert expiring in < 30 days: function should return days >= 0 and nil error
	now := time.Now()
	path := writeSelfSignedCert(t, now.Add(-24*time.Hour), now.Add(15*24*time.Hour))
	days, err := CheckCertExpiry(path)
	if err != nil {
		t.Fatalf("CheckCertExpiry: unexpected error for near-expiry cert: %v", err)
	}
	if days < 10 || days > 20 {
		t.Errorf("expected ~15 days remaining, got %d", days)
	}
}

func TestCheckCertExpiry_NoPEMBlock(t *testing.T) {
	f, err := os.CreateTemp("", "bad-cert-*")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("not a pem file\n")
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()

	_, err = CheckCertExpiry(f.Name())
	if err == nil {
		t.Error("expected error for non-PEM file, got nil")
	}
	if !strings.Contains(err.Error(), "no PEM block") {
		t.Errorf("expected 'no PEM block' in error, got: %v", err)
	}
}

func TestCheckCertExpiry_InvalidPEMBytes(t *testing.T) {
	f, err := os.CreateTemp("", "invalid-cert-*")
	if err != nil {
		t.Fatal(err)
	}
	// Write a PEM block with garbage DER bytes
	_ = pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: []byte("not valid der")})
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()

	_, err = CheckCertExpiry(f.Name())
	if err == nil {
		t.Error("expected error for invalid DER in PEM, got nil")
	}
	if !strings.Contains(err.Error(), "parsing certificate") {
		t.Errorf("expected 'parsing certificate' in error, got: %v", err)
	}
}

func TestCheckCertExpiry_ReadError(t *testing.T) {
	_, err := CheckCertExpiry("/tmp/nself-no-such-cert-99999.pem")
	if err == nil {
		t.Error("expected error for nonexistent cert path, got nil")
	}
	if !strings.Contains(err.Error(), "reading certificate") {
		t.Errorf("expected 'reading certificate' in error, got: %v", err)
	}
}

// ─── copyMkcertCerts ────────────────────────────────────────────────────────

func TestCopyMkcertCerts_Success(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	chainPath := filepath.Join(src, "fullchain.pem")
	keyPath := filepath.Join(src, "privkey.pem")
	_ = os.WriteFile(chainPath, []byte("chain data"), 0640)
	_ = os.WriteFile(keyPath, []byte("key data"), 0640)

	if err := copyMkcertCerts(chainPath, keyPath, dst); err != nil {
		t.Fatalf("copyMkcertCerts: unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "fullchain.pem")); err != nil {
		t.Error("fullchain.pem not created in dest dir")
	}
	if _, err := os.Stat(filepath.Join(dst, "privkey.pem")); err != nil {
		t.Error("privkey.pem not created in dest dir")
	}
}

func TestCopyMkcertCerts_ReadFullchainFail(t *testing.T) {
	dst := t.TempDir()
	err := copyMkcertCerts("/tmp/no-such-fullchain-99999.pem", "/tmp/no-such-privkey-99999.pem", dst)
	if err == nil {
		t.Error("expected error when source fullchain is missing")
	}
	if !strings.Contains(err.Error(), "reading") {
		t.Errorf("expected 'reading' in error, got: %v", err)
	}
}

func TestCopyMkcertCerts_ReadPrivkeyFail(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	chainPath := filepath.Join(src, "fullchain.pem")
	_ = os.WriteFile(chainPath, []byte("chain data"), 0640)

	err := copyMkcertCerts(chainPath, "/tmp/no-such-privkey-99999.pem", dst)
	if err == nil {
		t.Error("expected error when source privkey is missing")
	}
	if !strings.Contains(err.Error(), "reading") {
		t.Errorf("expected 'reading' in error, got: %v", err)
	}
}

func TestCopyMkcertCerts_WriteFullchainFail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped on Windows: chmod 0555 does not restrict writes")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root — permission test meaningless")
	}
	src := t.TempDir()
	chainPath := filepath.Join(src, "fullchain.pem")
	keyPath := filepath.Join(src, "privkey.pem")
	_ = os.WriteFile(chainPath, []byte("chain data"), 0640)
	_ = os.WriteFile(keyPath, []byte("key data"), 0640)

	dst := t.TempDir()
	_ = os.Chmod(dst, 0555)
	t.Cleanup(func() { _ = os.Chmod(dst, 0755) })

	err := copyMkcertCerts(chainPath, keyPath, dst)
	if err == nil {
		t.Error("expected error writing to read-only dest dir")
	}
}

func TestCopyMkcertCerts_WritePrivkeyFail(t *testing.T) {
	src := t.TempDir()
	chainPath := filepath.Join(src, "fullchain.pem")
	keyPath := filepath.Join(src, "privkey.pem")
	_ = os.WriteFile(chainPath, []byte("chain data"), 0640)
	_ = os.WriteFile(keyPath, []byte("key data"), 0640)

	// Make dst/privkey.pem a directory so WriteFile fails (EISDIR)
	dst := t.TempDir()
	privkeyDir := filepath.Join(dst, "privkey.pem")
	if err := os.MkdirAll(privkeyDir, 0755); err != nil {
		t.Fatal(err)
	}

	err := copyMkcertCerts(chainPath, keyPath, dst)
	if err == nil {
		t.Error("expected error writing privkey.pem to a directory path")
	}
	if !strings.Contains(err.Error(), "writing privkey.pem") {
		t.Errorf("expected 'writing privkey.pem' in error, got: %v", err)
	}
}

// ─── NewGenerator / CollectDomains (pure logic, no external deps) ─────────────

func TestNewGenerator_NotNil(t *testing.T) {
	g := NewGenerator(&config.Config{BaseDomain: "test.local"})
	if g == nil {
		t.Fatal("NewGenerator should never return nil")
	}
}

func TestCollectDomains_EmptyBaseDomain(t *testing.T) {
	g := NewGenerator(&config.Config{BaseDomain: ""})
	domains := g.CollectDomains()
	// With no BASE_DOMAIN, only fixed entries: localhost, 127.0.0.1, ::1,
	// local.nself.org, *.local.nself.org.
	for _, d := range domains {
		if d == "" {
			t.Error("CollectDomains should not return empty strings")
		}
	}
	// localhost must always appear.
	found := false
	for _, d := range domains {
		if d == "localhost" {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains: localhost missing; got %v", domains)
	}
}

func TestCollectDomains_BaseDomainIncluded(t *testing.T) {
	g := NewGenerator(&config.Config{BaseDomain: "myapp.local"})
	domains := g.CollectDomains()
	want := []string{"myapp.local", "*.myapp.local"}
	for _, w := range want {
		found := false
		for _, d := range domains {
			if d == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("CollectDomains: expected %q in domains; got %v", w, domains)
		}
	}
}

func TestCollectDomains_HasuraRoute_Explicit(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Hasura:     config.HasuraConfig{Route: "api.myapp.local"},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if d == "api.myapp.local" {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains: expected explicit hasura route api.myapp.local; got %v", domains)
	}
}

func TestCollectDomains_HasuraRoute_Default(t *testing.T) {
	// When Hasura.Route is empty, CollectDomains builds default via BuildServiceURL.
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Hasura:     config.HasuraConfig{Route: ""},
	})
	domains := g.CollectDomains()
	// Default should be api.myapp.local or similar.
	found := false
	for _, d := range domains {
		if strings.Contains(d, "myapp.local") && strings.Contains(d, "api") {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains: expected default hasura domain containing 'api' and 'myapp.local'; got %v", domains)
	}
}

func TestCollectDomains_AuthRoute_Explicit(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Auth:       config.AuthConfig{Route: "auth.myapp.local"},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if d == "auth.myapp.local" {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains: expected explicit auth route auth.myapp.local; got %v", domains)
	}
}

func TestCollectDomains_AuthRoute_Default(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Auth:       config.AuthConfig{Route: ""},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if strings.Contains(d, "auth") && strings.Contains(d, "myapp.local") {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains: expected default auth domain; got %v", domains)
	}
}

func TestCollectDomains_MinioEnabled_ExplicitRoutes(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Minio: config.MinioConfig{
			Enabled:      true,
			StorageRoute: "storage.myapp.local",
			ConsoleRoute: "storage-console.myapp.local",
		},
	})
	domains := g.CollectDomains()
	for _, want := range []string{"storage.myapp.local", "storage-console.myapp.local"} {
		found := false
		for _, d := range domains {
			if d == want {
				found = true
			}
		}
		if !found {
			t.Errorf("CollectDomains: expected %q; got %v", want, domains)
		}
	}
}

func TestCollectDomains_MinioEnabled_DefaultRoutes(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Minio: config.MinioConfig{
			Enabled:      true,
			StorageRoute: "",
			ConsoleRoute: "",
		},
	})
	domains := g.CollectDomains()
	// Expect default storage and storage-console domains.
	var hasStorage, hasConsole bool
	for _, d := range domains {
		if strings.Contains(d, "storage") && !strings.Contains(d, "console") {
			hasStorage = true
		}
		if strings.Contains(d, "console") {
			hasConsole = true
		}
	}
	if !hasStorage {
		t.Errorf("CollectDomains with Minio: missing default storage domain; got %v", domains)
	}
	if !hasConsole {
		t.Errorf("CollectDomains with Minio: missing default console domain; got %v", domains)
	}
}

func TestCollectDomains_MinioDisabled(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Minio:      config.MinioConfig{Enabled: false},
	})
	domains := g.CollectDomains()
	for _, d := range domains {
		if strings.Contains(d, "storage") {
			t.Errorf("CollectDomains with Minio disabled: unexpected storage domain %q", d)
		}
	}
}

func TestCollectDomains_AdminEnabled_ExplicitRoute(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Admin:      config.AdminConfig{Enabled: true, Route: "admin.myapp.local"},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if d == "admin.myapp.local" {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains with Admin enabled: expected admin.myapp.local; got %v", domains)
	}
}

func TestCollectDomains_AdminEnabled_DefaultRoute(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Admin:      config.AdminConfig{Enabled: true, Route: ""},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if strings.Contains(d, "admin") && strings.Contains(d, "myapp.local") {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains with Admin (default route): expected admin.myapp.local variant; got %v", domains)
	}
}

func TestCollectDomains_SearchEnabled(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Search:     config.SearchConfig{Enabled: true, Route: "search.myapp.local"},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if d == "search.myapp.local" {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains with Search: expected search.myapp.local; got %v", domains)
	}
}

func TestCollectDomains_SearchEnabled_DefaultRoute(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Search:     config.SearchConfig{Enabled: true, Route: ""},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if strings.Contains(d, "search") {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains with Search (default route): expected search domain; got %v", domains)
	}
}

func TestCollectDomains_MailpitEnabled(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Mailpit:    config.MailpitConfig{Enabled: true, Route: "mail.myapp.local"},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if d == "mail.myapp.local" {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains with Mailpit: expected mail.myapp.local; got %v", domains)
	}
}

func TestCollectDomains_MailpitEnabled_DefaultRoute(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Mailpit:    config.MailpitConfig{Enabled: true, Route: ""},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if strings.Contains(d, "mail") {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains with Mailpit (default route): expected mail domain; got %v", domains)
	}
}

func TestCollectDomains_FunctionsEnabled(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Functions:  config.FunctionsConfig{Enabled: true, Route: "fn.myapp.local"},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if d == "fn.myapp.local" {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains with Functions: expected fn.myapp.local; got %v", domains)
	}
}

func TestCollectDomains_FunctionsEnabled_DefaultRoute(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Functions:  config.FunctionsConfig{Enabled: true, Route: ""},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if strings.Contains(d, "function") {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains with Functions (default route): expected functions domain; got %v", domains)
	}
}

func TestCollectDomains_MLflowEnabled(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		MLflow:     config.MLflowConfig{Enabled: true, Route: "mlflow.myapp.local"},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if d == "mlflow.myapp.local" {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains with MLflow: expected mlflow.myapp.local; got %v", domains)
	}
}

func TestCollectDomains_MLflowEnabled_DefaultRoute(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		MLflow:     config.MLflowConfig{Enabled: true, Route: ""},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if strings.Contains(d, "mlflow") {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains with MLflow (default route): expected mlflow domain; got %v", domains)
	}
}

func TestCollectDomains_MonitoringEnabled_BothTrue(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Monitoring: config.MonitoringConfig{
			Enabled:        true,
			GrafanaEnabled: true,
			GrafanaRoute:   "grafana.myapp.local",
		},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if d == "grafana.myapp.local" {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains with Grafana: expected grafana.myapp.local; got %v", domains)
	}
}

func TestCollectDomains_MonitoringEnabled_GrafanaRouteDefault(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Monitoring: config.MonitoringConfig{
			Enabled:        true,
			GrafanaEnabled: true,
			GrafanaRoute:   "",
		},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if strings.Contains(d, "grafana") {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains with Grafana (default route): expected grafana domain; got %v", domains)
	}
}

func TestCollectDomains_MonitoringEnabled_GrafanaDisabled(t *testing.T) {
	// Monitoring.Enabled=true but GrafanaEnabled=false — grafana domain should NOT appear.
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Monitoring: config.MonitoringConfig{
			Enabled:        true,
			GrafanaEnabled: false,
		},
	})
	domains := g.CollectDomains()
	for _, d := range domains {
		if strings.Contains(d, "grafana") {
			t.Errorf("CollectDomains: grafana domain %q found but GrafanaEnabled=false", d)
		}
	}
}

func TestCollectDomains_MonitoringDisabled(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		Monitoring: config.MonitoringConfig{
			Enabled:        false,
			GrafanaEnabled: true,
			GrafanaRoute:   "grafana.myapp.local",
		},
	})
	domains := g.CollectDomains()
	for _, d := range domains {
		if strings.Contains(d, "grafana") {
			t.Errorf("CollectDomains: grafana domain %q found but Monitoring.Enabled=false", d)
		}
	}
}

func TestCollectDomains_CustomService_PublicWithRoute(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		CustomServices: []config.CustomService{
			{Name: "ping", Route: "ping", Public: true},
		},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if strings.Contains(d, "ping") {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains: expected public custom service domain; got %v", domains)
	}
}

func TestCollectDomains_CustomService_PublicWithFullDomain(t *testing.T) {
	// Route already contains baseDomain suffix — should be used as-is.
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		CustomServices: []config.CustomService{
			{Name: "ping", Route: "ping.myapp.local", Public: true},
		},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if d == "ping.myapp.local" {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains: expected ping.myapp.local (full domain); got %v", domains)
	}
}

func TestCollectDomains_CustomService_NotPublic(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		CustomServices: []config.CustomService{
			{Name: "internal-svc", Route: "internal", Public: false},
		},
	})
	domains := g.CollectDomains()
	for _, d := range domains {
		if strings.Contains(d, "internal") {
			t.Errorf("CollectDomains: non-public custom service domain %q should not appear", d)
		}
	}
}

func TestCollectDomains_CustomService_EmptyRoute(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		CustomServices: []config.CustomService{
			{Name: "nosvc", Route: "", Public: true},
		},
	})
	// Route is empty — no domain should be added for this service.
	before := NewGenerator(&config.Config{BaseDomain: "myapp.local"}).CollectDomains()
	after := g.CollectDomains()
	if len(after) != len(before) {
		t.Errorf("CollectDomains: empty-route public CS should add nothing; before=%d after=%d", len(before), len(after))
	}
}

func TestCollectDomains_FrontendApp_WithRoute(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		FrontendApps: []config.FrontendApp{
			{Route: "app"},
		},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if strings.Contains(d, "app") {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains: expected frontend app domain; got %v", domains)
	}
}

func TestCollectDomains_FrontendApp_FullDomainRoute(t *testing.T) {
	// Route already has baseDomain suffix.
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		FrontendApps: []config.FrontendApp{
			{Route: "app.myapp.local"},
		},
	})
	domains := g.CollectDomains()
	found := false
	for _, d := range domains {
		if d == "app.myapp.local" {
			found = true
		}
	}
	if !found {
		t.Errorf("CollectDomains: expected app.myapp.local from full domain route; got %v", domains)
	}
}

func TestCollectDomains_FrontendApp_EmptyRoute(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain:   "myapp.local",
		FrontendApps: []config.FrontendApp{{Route: ""}},
	})
	base := NewGenerator(&config.Config{BaseDomain: "myapp.local"}).CollectDomains()
	got := g.CollectDomains()
	if len(got) != len(base) {
		t.Errorf("CollectDomains: empty-route frontend app should add nothing; base=%d got=%d", len(base), len(got))
	}
}

func TestCollectDomains_ExtraSSLDomains(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain:      "myapp.local",
		ExtraSSLDomains: "extra1.example.com,extra2.example.com",
	})
	domains := g.CollectDomains()
	for _, want := range []string{"extra1.example.com", "extra2.example.com"} {
		found := false
		for _, d := range domains {
			if d == want {
				found = true
			}
		}
		if !found {
			t.Errorf("CollectDomains: expected ExtraSSLDomains entry %q; got %v", want, domains)
		}
	}
}

func TestCollectDomains_DeduplicatesEntries(t *testing.T) {
	// Both FrontendApp route and ExtraSSLDomains contain the same domain —
	// it should appear exactly once.
	g := NewGenerator(&config.Config{
		BaseDomain:      "myapp.local",
		ExtraSSLDomains: "extra.myapp.local",
		FrontendApps: []config.FrontendApp{
			{Route: "extra.myapp.local"},
		},
	})
	domains := g.CollectDomains()
	count := 0
	for _, d := range domains {
		if d == "extra.myapp.local" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("CollectDomains: expected deduplicated domain, found %d occurrences; all=%v", count, domains)
	}
}

// ─── GenerateWithResult — SSLMode shortcut branches ──────────────────────────

func TestGenerateWithResult_LetsEncrypt_ReturnsZero(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		SSLMode:    "letsencrypt",
	})
	result, err := g.GenerateWithResult(t.TempDir())
	if err != nil {
		t.Fatalf("GenerateWithResult(letsencrypt): unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("GenerateWithResult(letsencrypt): result should not be nil")
	}
	if result.Count != 0 {
		t.Errorf("GenerateWithResult(letsencrypt): expected Count=0, got %d", result.Count)
	}
}

func TestGenerateWithResult_Custom_ReturnsZero(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		SSLMode:    "custom",
	})
	result, err := g.GenerateWithResult(t.TempDir())
	if err != nil {
		t.Fatalf("GenerateWithResult(custom): unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("GenerateWithResult(custom): result should not be nil")
	}
	if result.Count != 0 {
		t.Errorf("GenerateWithResult(custom): expected Count=0, got %d", result.Count)
	}
}

func TestGenerateWithResult_None_ReturnsZero(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		SSLMode:    "none",
	})
	result, err := g.GenerateWithResult(t.TempDir())
	if err != nil {
		t.Fatalf("GenerateWithResult(none): unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("GenerateWithResult(none): result should not be nil")
	}
	if result.Count != 0 {
		t.Errorf("GenerateWithResult(none): expected Count=0, got %d", result.Count)
	}
}

func TestGenerate_LetsEncrypt_ReturnsZero(t *testing.T) {
	g := NewGenerator(&config.Config{
		BaseDomain: "myapp.local",
		SSLMode:    "letsencrypt",
	})
	count, err := g.Generate(t.TempDir())
	if err != nil {
		t.Fatalf("Generate(letsencrypt): unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("Generate(letsencrypt): expected 0, got %d", count)
	}
}
