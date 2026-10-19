package acme

// served_links.go: let adoption see through a served conf that names a
// top-level symlink in the ssl dir (D-1214). nself-web renders confs such as
// /etc/nginx/ssl/api-task-nself-org/fullchain.pem where api-task-nself-org is
// a symlink to certificates/api-task-nself-org. The link is followed one hop,
// lexically, and only when it lands on certificates/<one plain name>; links
// that are absolute outside the ssl dir, climb out with "..", chain through
// another link, or sit behind a symlinked certificates dir are left alone
// (the lineage is then refused as before). Reads only; never writes.

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/nginxtopo"
)

var topNameRE = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*$`)

// ResolveServedLinks returns hosts with each ssl_certificate path that goes
// through a top-level symlink of sslDir rewritten to the certificates/<name>
// it points at, plus one note per link followed or refused. hosts is not
// modified. Paths already under certificates/, outside the container ssl dir or
// through a plain directory pass through unchanged.
func ResolveServedLinks(sslDir string, hosts map[string]string) (map[string]string, []string) {
	out, notes, seen := make(map[string]string, len(hosts)), []string{}, map[string]bool{}
	prefix := nginxtopo.NginxSSLContainerPath + "/"
	for host, cert := range hosts {
		out[host] = cert
		p := path.Clean(cert)
		top, rest, ok := strings.Cut(strings.TrimPrefix(p, prefix), "/")
		if cert == "" || strings.Contains(cert, "$") || !strings.HasPrefix(p, prefix) || !ok || top == "certificates" || !topNameRE.MatchString(top) {
			continue
		}
		name, why := linkedLineageDir(sslDir, top)
		if name != "" {
			out[host] = prefix + "certificates/" + name + "/" + rest
			if !seen[top] {
				notes = append(notes, fmt.Sprintf("served path %s is a symlink to certificates/%s; adopting that directory", top, name))
			}
		} else if why != "" && !seen[top] {
			notes = append(notes, fmt.Sprintf("served path %s is a symlink that is not followed: %s", top, why))
		}
		seen[top] = true
	}
	return out, notes
}

// linkedLineageDir returns <name> when sslDir/top is a symlink to
// certificates/<name>. It returns ("", "") when top is not a symlink and
// ("", reason) when it is one that must not be followed.
func linkedLineageDir(sslDir, top string) (name, why string) {
	if fi, err := os.Lstat(filepath.Join(sslDir, top)); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return "", ""
	}
	dest, err := os.Readlink(filepath.Join(sslDir, top))
	if err != nil {
		return "", "its target cannot be read"
	}
	if filepath.IsAbs(dest) {
		rel, ok := relUnder(dest, sslDir, nginxtopo.NginxSSLContainerPath)
		if !ok {
			return "", fmt.Sprintf("%s is outside the ssl directory", dest)
		}
		dest = rel
	}
	dest = filepath.ToSlash(filepath.Clean(dest))
	if !ValidTarget(dest) {
		return "", fmt.Sprintf("%s is not certificates/<name> inside the ssl directory", dest)
	}
	certs := filepath.Join(sslDir, "certificates")
	if fi, err := os.Lstat(certs); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return "", "certificates/ is itself a symlink"
	}
	if real, err := filepath.EvalSymlinks(filepath.Join(sslDir, filepath.FromSlash(dest))); err == nil {
		base, berr := filepath.EvalSymlinks(certs)
		if rel, e := filepath.Rel(base, real); berr != nil || e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Sprintf("%s resolves outside certificates/", dest)
		}
	}
	return strings.TrimPrefix(dest, "certificates/"), ""
}

// relUnder returns p relative to the first root that contains it.
func relUnder(p string, roots ...string) (string, bool) {
	for _, r := range roots {
		for _, cand := range []string{r, evalOr(r)} {
			if rel, err := filepath.Rel(cand, p); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return rel, true
			}
		}
	}
	return "", false
}

// evalOr resolves symlinks in p, or returns p when it cannot.
func evalOr(p string) string {
	if e, err := filepath.EvalSymlinks(p); err == nil {
		return e
	}
	return p
}
