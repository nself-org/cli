package acme

// http01.go: HTTP-01 for the CLI-owned ACME engine (ADR 0026, P7-LIVE-24).
// IssueHTTP01 runs the pinned lego image once with `--http --http.webroot`;
// the webroot is <served ssl>/.acme-webroot, which nginx serves (read only,
// through its whole-dir ssl mount) at /.well-known/acme-challenge/. Only nself
// writes that directory (0755 dir, 0644 token files, written by lego); the
// account key stays under .acme (0700). NginxChallengeLocation is the one
// nginx snippet every port-80 server block carries; the nginx templates hold
// the same text and TestAcmeChallengeLocation fails when they drift.
// ProbeWebroot proves a served nginx answers the location (adoption).

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/httptimeout"
)

// ChallengeHTTP is the lineage challenge name for HTTP-01.
const ChallengeHTTP = "http-01"

// WebrootName is the directory under the served ssl dir that holds challenge
// files; inside nginx it is NginxSSLContainerPath + "/" + WebrootName.
const WebrootName = ".acme-webroot"

// ChallengePrefix is the URL prefix HTTP-01 uses.
const ChallengePrefix = "/.well-known/acme-challenge/"

// NginxChallengeLocation is the location block (4-space indented, newline
// terminated) placed in the default server and in every server block that
// listens on port 80, before any catch-all redirect or proxy. `^~` on the ACME
// prefix only, so no application path is shadowed; a request is served only
// when it is a bare token ([A-Za-z0-9_-]+) and a GET or HEAD, and never through
// a symlink below the webroot.
const NginxChallengeLocation = `    # ACME HTTP-01 (nself trust ssl add --acme): one bare token under the
    # challenge prefix is served from the nself-owned webroot; every other
    # path under it, a subdirectory, a traversal and any non-GET is refused.
    # The shared snippet: internal/ssl/acme.NginxChallengeLocation is the
    # same text for the custom-domain confs, guarded by a test.
    location ^~ /.well-known/acme-challenge/ {
        root /etc/nginx/ssl/.acme-webroot;
        disable_symlinks on from=$document_root;
        limit_except GET { deny all; }
        if ($uri !~ "^/[.]well-known/acme-challenge/[A-Za-z0-9_-]+$") { return 404; }
        try_files $uri =404;
    }
`

// WebrootDir returns <sslDir>/.acme-webroot.
func WebrootDir(sslDir string) string { return filepath.Join(sslDir, WebrootName) }

// EnsureWebroot creates the webroot and its challenge directory (0755: nginx
// runs as another user and must traverse it).
func EnsureWebroot(sslDir string) error {
	dir := filepath.Join(WebrootDir(sslDir), ".well-known", "acme-challenge")
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // nginx must read it; holds only public tokens
		return err
	}
	for _, d := range []string{WebrootDir(sslDir), filepath.Dir(dir), dir} {
		if err := os.Chmod(d, 0o755); err != nil { //nolint:gosec // see above
			return err
		}
	}
	return nil
}

// newDirsCleanup returns a func that removes, innermost first, those of the
// webroot directories that do not exist yet (os.Remove: only when empty).
func newDirsCleanup(sslDir string) func() {
	root := WebrootDir(sslDir)
	var created []string
	for _, d := range []string{filepath.Join(root, ".well-known", "acme-challenge"), filepath.Join(root, ".well-known"), root} {
		if _, err := os.Stat(d); err != nil {
			created = append(created, d)
		}
	}
	return func() {
		for _, d := range created {
			_ = os.Remove(d)
		}
	}
}

// tightenAccountKeys sets every ACME account key under root to 0600: lego
// writes them 0644 and only the 0700 .acme directory kept them private.
func tightenAccountKeys(root string) {
	keys, _ := filepath.Glob(filepath.Join(root, "accounts", "*", "*", "keys", "*.key"))
	for _, k := range keys {
		_ = os.Chmod(k, 0o600)
	}
}

// IssueHTTP01 issues r.Domains over HTTP-01 (no wildcard: the CA only
// validates a wildcard over DNS-01) and returns the certificate and key paths.
// r.Provider is ignored; r.Secret is never called.
func IssueHTTP01(ctx context.Context, r IssueReq) (cert, key string, err error) {
	for _, d := range r.Domains {
		switch {
		case strings.Contains(d, "*"):
			return "", "", Refuse("use a DNS provider (setup --acme --wildcard), or list each name", "HTTP-01 cannot issue the wildcard %q", d)
		case strings.IndexFunc(d, func(c rune) bool { return c > 127 }) >= 0:
			return "", "", Refuse("use the punycode (xn--) form of the name", "name %q is not ASCII; lego files it under another name", d)
		}
	}
	if err := EnsureWebroot(r.SSLDir); err != nil {
		return "", "", Refuse("check permissions on the served ssl directory", "creating the HTTP-01 webroot failed: %v", err)
	}
	server, root := ProdDirectory, "/ssl/.acme"
	if r.Staging {
		server, root = StagingDirectory, root+"/staging"
	}
	if r.Hooks.Directory != "" {
		server = r.Hooks.Directory
	}
	args := []string{"--path", root, "--email", r.Contact, "--server", server, "--key-type", "ec256",
		"--http", "--http.webroot", "/ssl/" + WebrootName}
	for _, d := range r.Domains {
		args = append(args, "--domains", d)
	}
	if r.AcceptTOS {
		args = append(args, "--accept-tos")
	}
	spec := docker.RunSpec{Image: LegoImage, Args: append(args, "run"), Network: r.Hooks.Network,
		EnvPass: map[string]string{}, Mounts: []docker.Mount{{Source: r.SSLDir, Destination: "/ssl"}}}
	if os.Getuid() >= 0 {
		spec.User = strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	}
	if r.Hooks.CABundle != "" {
		spec.Mounts = append(spec.Mounts, docker.Mount{Source: r.Hooks.CABundle, Destination: "/ca.pem", ReadOnly: true})
		spec.EnvPass["LEGO_CA_CERTIFICATES"] = "/ca.pem"
	}
	if r.Run == nil {
		r.Run = docker.RunOneShot
	}
	if _, stderr, rerr := r.Run(ctx, spec); rerr != nil {
		return "", "", fmt.Errorf("lego run (http-01) for %s failed: %w\n%s", r.Name, rerr, strings.TrimSpace(stderr[max(0, len(stderr)-400):]))
	}
	tightenAccountKeys(stateRoot(r.SSLDir, r.Staging))
	base := filepath.Join(stateRoot(r.SSLDir, r.Staging), "certificates", r.Domains[0])
	return base + ".crt", base + ".key", nil
}

// ProbeWebroot writes a random token file into the webroot, GETs it from addr
// (host:port of the served nginx over plain HTTP) with Host: host, and removes
// it. A nil error means the served nginx answers the challenge location from
// the webroot. get defaults to a short-timeout HTTP client.
func ProbeWebroot(ctx context.Context, sslDir, addr, host string, get func(*http.Request) (*http.Response, error)) error {
	cleanup := newDirsCleanup(sslDir) // directories the probe created are removed again (a dry run leaves no trace)
	defer cleanup()                   // registered first, so it runs after the probe file is removed
	if err := EnsureWebroot(sslDir); err != nil {
		return fmt.Errorf("creating the HTTP-01 webroot: %w", err)
	}
	var a, b [8]byte // the URL token and the body are independent: an echo or a catch-all cannot pass
	if _, err := rand.Read(a[:]); err != nil {
		return err
	}
	if _, err := rand.Read(b[:]); err != nil {
		return err
	}
	name, want := "nself-probe-"+hex.EncodeToString(a[:]), hex.EncodeToString(b[:])
	file := filepath.Join(WebrootDir(sslDir), ".well-known", "acme-challenge", name)
	if err := os.WriteFile(file, []byte(want), 0o644); err != nil { //nolint:gosec // nginx must read it
		return fmt.Errorf("writing the probe file: %w", err)
	}
	defer func() { _ = os.Remove(file) }()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+ChallengePrefix+name, nil)
	if err != nil {
		return err
	}
	req.Host = host
	if get == nil {
		c := httptimeout.NoProxy(5 * time.Second)
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		get = c.Do
	}
	resp, err := get(req)
	if err != nil {
		return fmt.Errorf("probing %s: %w", addr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != want {
		return fmt.Errorf("probing %s for %s: HTTP %d, not the probe file", addr, host, resp.StatusCode)
	}
	return nil
}
