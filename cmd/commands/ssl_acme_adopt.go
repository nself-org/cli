package commands

// ssl_acme_adopt.go: `trust ssl setup --acme --adopt-certbot [dir]`. Reads a
// certbot tree (never writes to it), maps each lineage to the served targets
// its names are served from, stores the DNS credential in the age store by
// name, records the lineages as dns-01 (a webroot lineage with no credential: http-01, when a probe
// token is answered by the served nginx) and never issues. A served target is
// replaced from certbot's live files only when it is missing or older.

import (
	"bytes"
	"context"
	"fmt"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ssl/acme"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// adopt implements the --adopt-certbot branch after preflight.
func (r *acmeRun) adopt(ctx context.Context, args []string) error {
	str := func(n string) string { v, _ := r.cmd.Flags().GetString(n); return v }
	dir := str("adopt-certbot")
	if len(args) > 0 {
		dir = args[0]
	}
	if c := str("challenge"); c != acme.ChallengeDNS && c != acme.ChallengeHTTP {
		return e151(acmeRefuse("use dns-01 or http-01", "challenge %q is not supported", c))
	}
	var provider string
	var secs map[string]string
	if cf := str("dns-credential-file"); cf != "" {
		var err error
		if provider, secs, err = parseDNSCredentialFile(cf); err != nil {
			return e151(err)
		}
	}
	cbs, err := acme.ReadCertbot(dir)
	if err != nil {
		return e151(acmeRefuse("pass the certbot directory: --adopt-certbot=<dir>", "%v", err))
	}
	hosts, _ := ssl.ServedHosts(r.res.NginxDir)
	// A webroot lineage with no credential is adopted as http-01 only when the served nginx answers
	// the challenge location for every name (a probe token file over local HTTP).
	probe := func(host string) error {
		env, _ := ssl.ServedEnv(r.res.Root, r.cfg.Env)
		return acmeProbeHTTP(ctx, r.res.SSLDir, httpProbeAddr(env), host, nil)
	}
	adopt, refused := acme.PlanWith(cbs, hosts, provider, str("lineage"), probe)
	for _, l := range adopt {
		r.printLineage(l)
	}
	for _, s := range refused {
		r.say("refused %s", s)
	}
	if len(refused) > 0 || len(adopt) == 0 {
		return e151(acmeRefuse("fix the lineages named above, or adopt selectively with --lineage <name>", "%d certbot lineage(s) cannot be adopted; nothing was changed", len(refused)))
	}
	if r.dry {
		r.say("dry run: nothing written")
		return nil
	}
	if err := r.lock(); err != nil {
		return err
	}
	defer r.unlock()
	for name, v := range secs {
		if err := acmeD.secSet(r.workdir, r.secEnv, name, v); err != nil {
			return e151(acmeRefuse("check the age key and that the value is a real credential", "storing %s in the secret store failed: %v", name, err))
		}
	}
	for _, l := range adopt { // saved per lineage: a later failure keeps what is already converted; re-run to finish
		if err := r.importLive(ctx, filepath.Join(dir, "live", l.Name), l); err != nil {
			return err
		}
		r.file.Lineages = append(slices.DeleteFunc(r.file.Lineages, func(x acme.Lineage) bool { return x.Name == l.Name }), l)
		if r.file.Contact = r.res.Contact; acme.Save(r.res.SSLDir, r.file) != nil {
			return e151(acmeRefuse("check permissions on the ssl/.acme directory", "saving lineages.json failed"))
		}
	}
	r.say("adopted %d lineage(s) (%s); certbot state was not touched", len(adopt), adoptedKinds(adopt))
	return nil
}

// adoptedKinds names the challenges of the adopted lineages, e.g. "1 dns-01, 1 http-01".
func adoptedKinds(ls []acme.Lineage) string {
	n := map[string]int{}
	for _, l := range ls {
		n[l.Challenge]++
	}
	var parts []string
	for _, c := range []string{acme.ChallengeDNS, acme.ChallengeHTTP} {
		if n[c] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n[c], c))
		}
	}
	return strings.Join(parts, ", ")
}

// importLive installs certbot's live pair into the targets of l that are
// missing or hold an older certificate; matching or newer targets stay as they are.
func (r *acmeRun) importLive(ctx context.Context, live string, l acme.Lineage) error {
	cert, e1 := os.ReadFile(filepath.Join(live, "fullchain.pem")) //nolint:gosec // certbot's live dir, read only
	key, e2 := os.ReadFile(filepath.Join(live, "privkey.pem"))    //nolint:gosec // certbot's live dir, read only
	liveCert, e3 := ssl.ReadDiskCert(filepath.Join(live, "fullchain.pem"))
	if e1 != nil || e2 != nil || e3 != nil {
		return e151(acmeRefuse("run as the user that owns /etc/letsencrypt", "cannot read the live files of %s", l.Name))
	}
	var stale []string
	for _, t := range l.Targets {
		p := filepath.Join(r.res.SSLDir, filepath.FromSlash(t), "fullchain.pem")
		if have, err := os.ReadFile(p); err == nil && bytes.Equal(have, cert) {
			continue
		}
		if c, err := ssl.ReadDiskCert(p); err == nil && !c.NotAfter.Before(liveCert.NotAfter) {
			continue
		}
		stale = append(stale, t)
	}
	if l.Targets = stale; len(stale) == 0 {
		return nil
	}
	return r.install(ctx, l, cert, key)
}
