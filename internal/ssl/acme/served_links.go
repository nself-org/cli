package acme

// served_links.go: let adoption see through a served conf that names a
// top-level symlink in the ssl dir (D-1214). nself-web renders confs such as
// /etc/nginx/ssl/api-task-nself-org/fullchain.pem where api-task-nself-org is
// a symlink to certificates/api-task-nself-org. The link is followed one hop
// and only when it lands on certificates/<one plain name> and the kernel
// resolves it to the same directory (so a ".." behind another link cannot hide
// a different place). Links that are absolute but not under the container ssl
// path (the host path does not exist inside nginx), contain "..", chain through
// another link, point at a missing directory or sit behind a symlinked
// certificates dir are left alone (the lineage is then refused as before).
// Reads only; never writes.

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
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
	names := make([]string, 0, len(hosts))
	for host := range hosts {
		names = append(names, host)
	}
	sort.Strings(names) // stable notes
	for _, host := range names {
		cert := hosts[host]
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
	at := filepath.Join(sslDir, top)
	if fi, err := os.Lstat(at); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return "", ""
	}
	dest, err := os.Readlink(at)
	if err != nil {
		return "", "its target cannot be read"
	}
	// Clean is textual: a ".." behind another link would vanish, so refuse it on the raw text.
	for _, el := range strings.Split(filepath.ToSlash(dest), "/") {
		if el == ".." {
			return "", fmt.Sprintf("%s climbs out with ..", dest)
		}
	}
	container := filepath.IsAbs(dest)
	if container { // nginx resolves the link inside its own mount, so only the container path counts
		rel, err := filepath.Rel(nginxtopo.NginxSSLContainerPath, dest)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return "", fmt.Sprintf("%s is not under %s, the path nginx sees", dest, nginxtopo.NginxSSLContainerPath)
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
	want := filepath.Join(sslDir, filepath.FromSlash(dest))
	real, err := filepath.EvalSymlinks(want)
	if err != nil {
		return "", fmt.Sprintf("%s does not exist", dest)
	}
	base, err := filepath.EvalSymlinks(certs)
	if rel, e := filepath.Rel(base, real); err != nil || e != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Sprintf("%s resolves outside certificates/", dest)
	}
	if !container { // the kernel's answer for the link must be the directory named, whatever the text says
		if got, err := filepath.EvalSymlinks(at); err != nil || got != real {
			return "", fmt.Sprintf("it resolves to %s, not %s", got, real)
		}
	}
	return strings.TrimPrefix(dest, "certificates/"), ""
}
