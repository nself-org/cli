package ssl

// served_hosts.go — the filesystem half of the served-certificate check.
//
// Purpose: `nself doctor` compares what nginx serves (ProbeServed) with what
// its own conf names on disk and with any newer certificate that was renewed
// but never installed (D-0121: certbot renewed a lineage under /etc/letsencrypt
// while nginx kept serving a stale copy from the stack's ssl tree).
// Inputs: the served stack's nginx dir, ssl dir (mounted at /etc/nginx/ssl),
// root (holds the .env files) and a host name.
// Outputs: ServedHosts, DiskCertFor, NewestKnownCert, ServedEnv, ServedAddr.
// Constraints: nothing is dialled or executed; env files are read with
// godotenv.Read, never Overload, so the fronting stack's .env cannot leak into
// this process.

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/nself-org/cli/internal/nginxtopo"
)

// ServedHosts returns each host the nginx under nginxDir terminates TLS for,
// mapped to the ssl_certificate path of the SAME 443 server block ("" when it
// names none, e.g. via `include`; an http- or file-level one is inherited).
// Only sites/*.conf and conf.d/*.conf are read (SAN-only names land on the
// default server). Wildcards, regexes, IPs, localhost, `_` and .local,
// .localhost, .test names are dropped; the first block wins a duplicate. An
// unreadable file does not hide the others: hosts found come back with the
// joined read errors.
func ServedHosts(nginxDir string) (map[string]string, error) {
	out, errs := map[string]string{}, []error{}
	for _, sub := range []string{"sites", "conf.d"} {
		files, _ := filepath.Glob(filepath.Join(nginxDir, sub, "*.conf"))
		sort.Strings(files)
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			findServers(parseNginx(string(data)), "", func(srv *nginxNode, inherited string) { addServerHosts(out, srv, inherited) })
		}
	}
	return out, errors.Join(errs...)
}

// addServerHosts records the hosts of a server block that listens on 443.
func addServerHosts(out map[string]string, srv *nginxNode, cert string) {
	var names []string
	listens443, own := false, ""
	for _, k := range srv.kids {
		if k.block || len(k.words) < 2 {
			continue
		}
		switch k.words[0] {
		case "listen":
			listens443 = listens443 || k.words[1] == "443" || strings.HasSuffix(k.words[1], ":443")
		case "server_name":
			names = append(names, k.words[1:]...)
		case "ssl_certificate":
			if own == "" {
				own = k.words[1]
			}
		}
	}
	if own != "" {
		cert = own
	}
	for _, n := range names {
		h := strings.ToLower(n)
		if !listens443 || h == "_" || h == "localhost" || strings.ContainsAny(h, "*$~") || strings.HasPrefix(h, ".") ||
			net.ParseIP(strings.Trim(h, "[]")) != nil || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".test") {
			continue
		}
		if _, dup := out[h]; !dup {
			out[h] = cert
		}
	}
}

// DiskCertFor maps a path under nginxtopo.NginxSSLContainerPath onto the same
// file in the served ssl dir; ok is false outside the mount or with a `$variable`.
func DiskCertFor(sslDir, certPath string) (string, bool) {
	prefix := nginxtopo.NginxSSLContainerPath + "/"
	clean := path.Clean(certPath)
	ok := !strings.Contains(certPath, "$") && strings.HasPrefix(clean, prefix)
	return filepath.Join(sslDir, filepath.FromSlash(strings.TrimPrefix(clean, prefix))), ok
}

// KnownCert is the leaf of a certificate file: its path, expiry and DER SHA-256 (hex).
type KnownCert struct {
	Path     string
	NotAfter time.Time
	SHA256   string
	leaf     *x509.Certificate
}

// ReadDiskCert reads the leaf (first certificate) of the PEM file p.
func ReadDiskCert(p string) (KnownCert, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return KnownCert{}, err
	}
	for {
		var b *pem.Block
		if b, data = pem.Decode(data); b == nil {
			return KnownCert{}, fmt.Errorf("no certificate in %s", p)
		}
		if b.Type != "CERTIFICATE" {
			continue
		}
		c, perr := x509.ParseCertificate(b.Bytes)
		if perr != nil {
			return KnownCert{}, perr
		}
		sum := sha256.Sum256(c.Raw)
		return KnownCert{Path: p, NotAfter: c.NotAfter, SHA256: hex.EncodeToString(sum[:]), leaf: c}, nil
	}
}

// NewestKnownCert returns the certificate covering host (SANs, wildcards
// included; expiry ignored) with the latest expiry among
// <sslDir>/.acme/certificates/*.crt and <letsencryptDir>/live/*/cert.pem.
// Unreadable files (certbot's live dir is often root-only) are skipped.
func NewestKnownCert(sslDir, host, letsencryptDir string) (KnownCert, bool) {
	files, _ := filepath.Glob(filepath.Join(sslDir, ".acme", "certificates", "*.crt"))
	live, _ := filepath.Glob(filepath.Join(letsencryptDir, "live", "*", "cert.pem"))
	var best KnownCert
	found := false
	for _, f := range append(files, live...) {
		kc, err := ReadDiskCert(f)
		if err != nil || strings.HasSuffix(f, ".issuer.crt") || kc.leaf.VerifyHostname(host) != nil ||
			(found && !kc.NotAfter.After(best.NotAfter)) {
			continue
		}
		best, found = kc, true
	}
	return best, found
}

var envAliases = map[string]string{"development": "dev", "develop": "dev", "devel": "dev", "production": "prod", "stage": "staging"} // as config

// ServedEnv reads <root>/.env, then <root>/.env.<ENV> over it (ENV from that
// .env, else fallbackEnv, aliases applied) without touching the process
// environment. Missing files are fine; an unparsable one is an error.
func ServedEnv(root, fallbackEnv string) (map[string]string, error) {
	env, err := readEnvFile(filepath.Join(root, ".env"))
	if err != nil {
		return nil, err
	}
	name := env["ENV"]
	if name == "" {
		name = fallbackEnv
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if alias, ok := envAliases[name]; ok {
		name = alias
	}
	if name == "" {
		return env, nil
	}
	layer, err := readEnvFile(filepath.Join(root, ".env."+name))
	for k, v := range layer {
		env[k] = v
	}
	return env, err
}

// readEnvFile parses an env file; missing is empty.
func readEnvFile(p string) (map[string]string, error) {
	m, err := godotenv.Read(p)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	return m, err
}

// ServedAddr returns the "host:port" the served nginx accepts TLS on: port
// NGINX_HTTPS_PORT, else NGINX_SSL_PORT, else 443; host NGINX_BIND_IP unless
// empty or a wildcard address (0.0.0.0, ::), then 127.0.0.1.
func ServedAddr(env map[string]string) string {
	port := "443"
	for _, k := range []string{"NGINX_SSL_PORT", "NGINX_HTTPS_PORT"} { // later key wins
		if v := strings.TrimSpace(env[k]); v != "" {
			port = v
		}
	}
	host := strings.Trim(strings.TrimSpace(env["NGINX_BIND_IP"]), "[]")
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// nginxNode is a statement (`words;`) or block (`words { kids }`).
type nginxNode struct {
	words []string
	kids  []*nginxNode
	block bool
}

// maxNginxDepth bounds nesting (deeper blocks are read flat: no stack overflow).
const maxNginxDepth = 64

// parseNginx parses nginx config text (no include or variable expansion).
func parseNginx(src string) []*nginxNode {
	root := &nginxNode{block: true}
	stack, over := []*nginxNode{root}, 0
	var words []string
	var cur strings.Builder
	inWord := false
	flush := func() {
		if inWord {
			words, inWord = append(words, cur.String()), false
			cur.Reset()
		}
	}
	add := func(n *nginxNode) { stack[len(stack)-1].kids = append(stack[len(stack)-1].kids, n) }
	endStmt := func() {
		flush()
		if len(words) > 0 {
			add(&nginxNode{words: words})
		}
		words = nil
	}
	for i := 0; i < len(src); i++ {
		switch c := src[i]; {
		case c == '#' && !inWord:
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '"' || c == '\'':
			inWord = true
			for i++; i < len(src) && src[i] != c; i++ {
				cur.WriteByte(src[i])
			}
		case c == ';':
			endStmt()
		case c == '{':
			flush()
			if len(stack) >= maxNginxDepth {
				over, words = over+1, nil
				continue
			}
			n := &nginxNode{words: words, block: true}
			words = nil
			add(n)
			stack = append(stack, n)
		case c == '}':
			endStmt()
			if over > 0 {
				over--
			} else if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flush()
		default:
			inWord = true
			cur.WriteByte(c)
		}
	}
	endStmt()
	return root.kids
}

// findServers calls visit for every server block at any depth, with the
// ssl_certificate its enclosing scope sets (nginx inherits it into the server).
func findServers(nodes []*nginxNode, inherited string, visit func(*nginxNode, string)) {
	for _, n := range nodes {
		if !n.block && len(n.words) > 1 && n.words[0] == "ssl_certificate" {
			inherited = n.words[1]
		}
	}
	for _, n := range nodes {
		switch {
		case !n.block:
		case len(n.words) == 1 && n.words[0] == "server":
			visit(n, inherited)
		default:
			findServers(n.kids, inherited, visit)
		}
	}
}
