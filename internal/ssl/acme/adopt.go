package acme

// adopt.go: read a certbot tree and decide which lineages the CLI can take
// over. Reads renewal/*.conf and live/<name>/cert.pem (never writes), maps each
// lineage to the served ssl dirs its names are served from (host -> the
// ssl_certificate path of the served nginx confs) and returns the dns-01
// lineages to record plus the refusals with remediation. standalone, webroot
// and nginx lineages convert to dns-01 only when a credential is supplied.

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/nself-org/cli/internal/nginxtopo"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Certbot is one certbot lineage as found on disk; Err is set when its live
// certificate could not be read.
type Certbot struct {
	Name, Authenticator, Conf string
	Domains                   []string
	Err                       error
}

// ParseINI parses `key = value` lines (`#`/`;` comments, `[sections]` ignored,
// whitespace trimmed) into one flat map.
func ParseINI(data []byte) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line == "" || strings.ContainsAny(line[:1], "#;[") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

// ReadCertbot lists the lineages under dir (renewal/*.conf), sorted by name.
func ReadCertbot(dir string) (out []Certbot, err error) {
	confs, _ := filepath.Glob(filepath.Join(dir, "renewal", "*.conf"))
	if len(confs) == 0 {
		return nil, fmt.Errorf("no certbot lineages found under %s/renewal", dir)
	}
	sort.Strings(confs)
	for _, c := range confs {
		cb := Certbot{Name: strings.TrimSuffix(filepath.Base(c), ".conf"), Conf: c}
		conf, e1 := os.ReadFile(c)
		cb.Authenticator = ParseINI(conf)["authenticator"]
		if cb.Domains, cb.Err = liveNames(filepath.Join(dir, "live", cb.Name, "cert.pem")); e1 != nil {
			cb.Err = e1
		}
		out = append(out, cb)
	}
	return out, nil
}

// liveNames returns the DNS SANs of the leaf in a PEM file.
func liveNames(file string) ([]string, error) {
	data, err := os.ReadFile(file)
	if b, _ := pem.Decode(data); err == nil && b != nil {
		c, perr := x509.ParseCertificate(b.Bytes)
		if perr != nil {
			return nil, perr
		}
		return c.DNSNames, nil
	} else if err == nil {
		err = fmt.Errorf("no certificate in %s", file)
	}
	return nil, err
}

// covers reports whether certificate name san serves host (a wildcard spans one label).
func covers(san, host string) bool {
	rest, wild := strings.CutPrefix(san, "*.")
	h, tail, found := strings.Cut(host, ".")
	return san == host || wild && found && h != "" && tail == rest
}

// Plan maps certbot lineages onto served targets. provider is the DNS provider
// of the supplied credential ("" when none); only, when set, limits adoption to
// that lineage. Refusals are "<name>: <reason and remediation>".
func Plan(cbs []Certbot, hosts map[string]string, provider, only string) (adopt []Lineage, refused []string) {
	owner := map[string]string{} // target -> lineage name
	for _, cb := range cbs {
		if only != "" && cb.Name != only {
			continue
		}
		prov, why := strings.TrimPrefix(cb.Authenticator, "dns-"), ""
		_, known := SecretNames(prov)
		switch {
		case cb.Err != nil:
			why = fmt.Sprintf("cannot read the certbot lineage (%v); run as the user that owns /etc/letsencrypt", cb.Err)
		case strings.HasPrefix(cb.Authenticator, "dns-") && !known:
			why = "DNS plugin " + cb.Authenticator + " is not supported (cloudflare, route53, digitalocean); issue a new certificate instead"
		case strings.HasPrefix(cb.Authenticator, "dns-") && provider != "" && provider != prov:
			why = fmt.Sprintf("the lineage uses %s but the credential file is for %s", prov, provider)
		case !strings.HasPrefix(cb.Authenticator, "dns-") && provider == "":
			why = "authenticator " + cb.Authenticator + " needs a stopped or shared port; pass --dns-credential-file to convert it to dns-01"
		case !strings.HasPrefix(cb.Authenticator, "dns-"):
			prov = provider
		}
		targets := targetsFor(cb.Domains, hosts)
		for _, t := range targets {
			if o, ok := owner[t]; ok && why == "" {
				why = fmt.Sprintf("target %s is already served by lineage %s", t, o)
			} else if !ValidTarget(t) && why == "" {
				why = fmt.Sprintf("a served conf points at %q, not certificates/<name>; give the lineage its own certificate directory first", t)
			}
		}
		if why == "" && len(targets) == 0 {
			why = "no served nginx conf references a certificate for " + strings.Join(cb.Domains, ", ")
		}
		if why != "" {
			refused = append(refused, cb.Name+": "+why)
			continue
		}
		for _, t := range targets {
			owner[t] = cb.Name
		}
		names, _ := SecretNames(prov)
		src := cb.Conf
		adopt = append(adopt, Lineage{Name: cb.Name, Domains: cb.Domains, Challenge: ChallengeDNS, DNSProvider: prov,
			CredentialSecrets: names, Targets: targets, KeyType: "ec256", AdoptedFrom: &src})
	}
	return adopt, refused
}

// targetsFor returns the certificate dirs (relative to the served ssl dir) of
// the served hosts that names cover, sorted and de-duplicated.
func targetsFor(names []string, hosts map[string]string) (out []string) {
	set, prefix := map[string]bool{}, nginxtopo.NginxSSLContainerPath+"/"
	for host, cert := range hosts {
		if p := path.Clean(cert); cert != "" && !strings.Contains(cert, "$") && strings.HasPrefix(p, prefix) {
			for _, n := range names {
				if covers(n, host) {
					set[path.Dir(strings.TrimPrefix(p, prefix))] = true
				}
			}
		}
	}
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
