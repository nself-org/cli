package commands

// Purpose: issue and install custom-domain HTTP-01 ACME certificates.
// Inputs: add command flags, served nginx topology and ACME dependencies.
// Outputs: a generation, nginx configuration and a recorded lineage.
// Constraints: refuses certbot writers before issuance; holds the ACME lock.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ssl/acme"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

// acmeHostRE is a plain DNS name with at least one dot: no wildcard, no port.
var acmeHostRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

// acmeProbeHTTP is the HTTP-01 reachability probe (tests replace it).
var acmeProbeHTTP = acme.ProbeWebroot

// httpProbeAddr is where the served nginx answers plain HTTP from this host.
func httpProbeAddr(env map[string]string) string {
	port, host := "80", strings.Trim(strings.TrimSpace(env["NGINX_BIND_IP"]), "[]")
	if v := strings.TrimSpace(env["NGINX_HTTP_PORT"]); v != "" {
		port = v
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// runSSLAddACME implements `trust ssl add <domain> --acme`: issue over HTTP-01
// into <served ssl>/.acme-webroot (nginx answers the challenge from the
// default server or the domain's own port-80 block), install the certificate
// as a generation of certificates/<domain-safe>, write the custom-domain conf,
// reload, verify the served fingerprint and record the lineage.
func runSSLAddACME(cmd *cobra.Command, args []string) (retErr error) {
	domain := strings.ToLower(strings.TrimSuffix(args[0], "."))
	if !acmeHostRE.MatchString(domain) {
		return e151(acmeRefuse("give one plain DNS name; a wildcard needs DNS-01 (`"+acmeCommand("setup")+" --wildcard`)",
			"%q cannot be issued over HTTP-01", args[0]))
	}
	r, err := prepareACME(cmd, false)
	if err != nil {
		return err
	}
	name := domainToFilesafe(domain)
	l := acme.Lineage{Name: name, Domains: []string{domain}, Challenge: acme.ChallengeHTTP, Targets: []string{"certificates/" + name}, KeyType: "ec256"}
	r.say("lineage %s: http-01 for %s\n  target:  %s\n  webroot: %s", name, domain, l.Targets[0], acme.WebrootDir(r.res.SSLDir))
	if err := r.refuseCertbotHooks(certbotDir); err != nil {
		return err
	}
	if r.dry {
		r.say("dry run: nothing written")
		_, err := r.retireCertbot()
		return err
	}
	if err := r.lock(); err != nil {
		return err
	}
	defer r.unlock()
	ctx := cmd.Context()
	hosts, _ := ssl.ServedHosts(r.res.NginxDir)
	if c := hosts[domain]; c != "" && !strings.Contains(c, "/"+name+"/") {
		return e151(acmeRefuse("use `"+acmeCommand("renew")+"` for the lineage that serves it", "%s is already served over HTTPS with %s", domain, c))
	}
	for _, x := range r.file.Lineages {
		if x.Name == name {
			return e151(acmeRefuse("renew it with `"+acmeCommand("renew")+" --force`", "lineage %s is already managed", name))
		}
	}
	if err := r.probeHTTP(ctx, domain); err != nil {
		return err
	}
	tos, err := r.acceptTOS()
	if err != nil {
		return err
	}
	// Stop certbot before issuance and recheck hooks saved by an in-flight renewal.
	retired, err := r.retireAndRecheck(false, certbotDir)
	if err != nil {
		return err
	}
	defer func() { r.retiredHint(retired && retErr != nil) }()
	cert, key, err := acme.IssueHTTP01(ctx, acme.IssueReq{SSLDir: r.res.SSLDir, Name: name, Contact: r.res.Contact,
		Domains: l.Domains, AcceptTOS: tos, Hooks: r.hooks, Run: acmeD.run})
	if err != nil {
		return e151(err)
	}
	c, e1 := os.ReadFile(cert) //nolint:gosec // path built from the served ssl dir
	k, e2 := os.ReadFile(key)  //nolint:gosec // path built from the served ssl dir
	if err := errors.Join(e1, e2); err != nil {
		return e151(err)
	}
	reload := acme.NginxReloader{Container: r.res.Container, Exec: acmeD.exec}
	gens, err := acme.Install(ctx, acme.InstallReq{SSLDir: r.res.SSLDir, Targets: l.Targets, Cert: c, Key: k,
		Access: r.installAccess(), Reloader: reload, Fault: r.hooks.Fault, Exit: acmeD.exit})
	if err != nil {
		return e151(err)
	}
	r.say("lineage %s: installed %s as generation %d", name, l.Targets[0], gens[l.Targets[0]])
	if err := writeAndVerifyAdd(ctx, r, l, c, reload); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	l.LastIssued = &now
	r.file.Contact = r.res.Contact
	r.file.Lineages = append(r.file.Lineages, l)
	if err := acme.Save(r.res.SSLDir, r.file); err != nil {
		return e151(err)
	}
	ui.Success(fmt.Sprintf("Certificate provisioned for %s over HTTP-01.", domain))
	return nil
}

// installFailed tags err as an after-install failure (E471 in v1.5). The chain
// keeps the *acme.Error, so e151 still reads its What and Fix; the message text
// is unchanged (the %.0w verb adds no output).
func installFailed(err error) error { return fmt.Errorf("%w%.0w", err, acme.ErrInstall) }

// writeAndVerifyAdd writes the domain's conf (unless a served conf already uses
// the target), reloads nginx and checks the fingerprint it serves.
func writeAndVerifyAdd(ctx context.Context, r *acmeRun, l acme.Lineage, cert []byte, reload acme.NginxReloader) error {
	hosts, _ := ssl.ServedHosts(r.res.NginxDir)
	if !strings.Contains(hosts[l.Domains[0]], "/"+l.Name+"/") {
		upstream, _ := r.cmd.Flags().GetString("upstream")
		if err := writeCustomDomainConfServed(r.workdir, r.cfg.Nginx.FrontedBy, l.Domains[0], upstream); err != nil {
			return e151(installFailed(acmeRefuse("the certificate is installed; fix the conf directory and re-run", "writing nginx conf: %v", err)))
		}
	}
	if err := reload.Reload(ctx); err != nil {
		return e151(installFailed(acmeRefuse("fix the nginx error above; the certificate is installed", "%v", err)))
	}
	if err := r.verifier(l, cert)(ctx); err != nil {
		return e151(installFailed(acmeRefuse("check that nginx serves the domain on its HTTPS port", "served certificate check failed: %v", err)))
	}
	r.say("lineage %s: nginx reloaded, served certificate verified", l.Name)
	return nil
}
