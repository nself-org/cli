package commands

// Purpose: Implements `nself ssl add <domain>` — issues a certbot cert for
// an additional custom domain and wires its nginx config, plus the small
// helper that makes a domain name safe to embed in a filename. Split out
// of ssl_setup.go (CLI-R12) to separate this subcommand from the main
// `nself ssl setup` flow (ssl_setup.go), the cert-install/nginx-config
// helpers (ssl_install.go), and the renewal-hook installers
// (ssl_renewal.go).
// Inputs: the cobra.Command + args (the domain to add) and flags shared
// with `ssl setup` (provider, email).
// With --acme the certificate comes from the CLI's own ACME client over HTTP-01
// (runSSLAddACME below); the certbot path below is untouched.
// Outputs: an issued certificate installed via installIssuedCert and a
// generated nginx server block via writeCustomDomainConf.
// Constraints: certificate and conf go to the served nginx/ssl trees
// (P7-LIVE-02, D-0045); the nginx test/reload run in the served stack's
// compose project. domainToFilesafe is also used by ssl_install.go and
// ssl_renewal.go.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginxtopo"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ssl/acme"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

func runSSLAdd(cmd *cobra.Command, args []string) error {
	if on, _ := cmd.Flags().GetBool("acme"); on {
		return runSSLAddACME(cmd, args)
	} else if err := rejectACMEFlags(cmd); err != nil {
		return err
	}
	domain := args[0]
	upstream, _ := cmd.Flags().GetString("upstream")

	ui.CommandHeader("nself ssl add", fmt.Sprintf("Provision certificate for %s", domain))

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	workdir, err := config.FindNSelfRoot(cwd)
	if err != nil {
		return fmt.Errorf("no nself project found: run 'nself init' first")
	}

	cfg, err := config.Load(workdir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if _, err := exec.LookPath("certbot"); err != nil {
		return fmt.Errorf("certbot not found in PATH")
	}

	email := cfg.AdminEmail
	if email == "" {
		return fmt.Errorf("ADMIN_EMAIL must be set in .env for certificate registration")
	}

	// Create the cert output directory so certbot can write into it.
	//
	// This must match the layout the nginx container actually sees. Compose
	// mounts "./ssl:/etc/nginx/ssl:ro" and internal/ssl writes certificates to
	// ssl/certificates/<dir>, where <dir> is the domain with dots replaced by
	// dashes. Writing to ssl/<dotted-domain> instead produced a cert on disk
	// that the generated server block could never reference, so `ssl add`
	// reported success while nginx kept serving the self-signed wildcard.
	//
	// Both targets are the SERVED tree (internal/nginxtopo, D-0045): a project
	// with NGINX_FRONTED_BY set has no nginx of its own, so its certificate and
	// custom-domain conf belong in the fronting stack's ssl/ and nginx/ dirs.
	// Resolved before certbot runs so an unconfirmed layout fails early.
	frontedBy := cfg.Nginx.FrontedBy
	domainSafe := domainToFilesafe(domain)
	certDir, err := servedCertDir(workdir, frontedBy, domain)
	if err != nil {
		return fmt.Errorf("locating the served ssl directory: %w", err)
	}
	servedRoot, err := nginxtopo.ServedRoot(workdir, frontedBy)
	if err != nil {
		return fmt.Errorf("locating the served nginx stack: %w", err)
	}
	if err := os.MkdirAll(certDir, 0750); err != nil {
		return fmt.Errorf("creating cert directory: %w", err)
	}

	// Use HTTP-01 challenge for single domain adds (simpler, no DNS provider needed).
	//
	// --cert-path/--key-path are deliberately NOT passed: certbot ignores them
	// for `certonly` (they apply to `install`), which is why earlier versions
	// appeared to place the certificate where nginx expected it while certbot
	// actually wrote to /etc/letsencrypt/live/<domain>/ and nothing ever bridged
	// the two. We copy explicitly below instead.
	certArgs := []string{
		"certonly",
		"--non-interactive",
		"--agree-tos",
		"--email", email,
		"--webroot",
		"--webroot-path", "/var/www/certbot",
		"-d", domain,
	}

	ui.Info(fmt.Sprintf("Provisioning certificate for %s...", domain))
	certCmd := exec.Command("certbot", certArgs...)
	certCmd.Stdout = os.Stdout
	certCmd.Stderr = os.Stderr
	if err := certCmd.Run(); err != nil {
		return fmt.Errorf("certbot failed: %w", err)
	}

	// Install the issued certificate into the tree nginx actually reads.
	// Without this the cert exists only under /etc/letsencrypt and nginx keeps
	// serving the self-signed wildcard, which is a silent no-op from the
	// operator's point of view.
	if err := installIssuedCert(domain, certDir); err != nil {
		return fmt.Errorf("installing certificate for %s: %w", domain, err)
	}

	// Write the nginx server block for this custom domain.
	if err := writeCustomDomainConfServed(workdir, frontedBy, domain, upstream); err != nil {
		return fmt.Errorf("writing nginx conf: %w", err)
	}

	// Validate nginx config before reloading.
	testCmd := exec.Command("docker", "compose", "exec", "nginx", "nginx", "-t")
	testCmd.Dir = servedRoot
	if out, testErr := testCmd.CombinedOutput(); testErr != nil {
		return fmt.Errorf("nginx config test failed: %s", string(out))
	}

	reloadCmd := exec.Command("docker", "compose", "exec", "nginx", "nginx", "-s", "reload")
	reloadCmd.Dir = servedRoot
	reloadCmd.Stdout = os.Stdout
	reloadCmd.Stderr = os.Stderr
	if err := reloadCmd.Run(); err != nil {
		ui.Warn(fmt.Sprintf("Nginx reload failed: %v", err))
	}

	confDir, err := servedConfDir(workdir, frontedBy)
	if err != nil {
		return fmt.Errorf("locating the served nginx directory: %w", err)
	}
	ui.Info(fmt.Sprintf("Custom domain conf written to %s",
		filepath.Join(confDir, fmt.Sprintf("custom-%s.conf", domainSafe))))
	ui.Success(fmt.Sprintf("Certificate provisioned for %s.", domain))
	return nil
}

// domainToFilesafe replaces dots and colons with dashes so a domain name is
// safe to embed in a filename. e.g. "my.custom.com" -> "my-custom-com".
func domainToFilesafe(domain string) string {
	r := strings.NewReplacer(".", "-", ":", "-")
	return r.Replace(domain)
}

func init() {
	f := sslAddCmd.Flags()
	f.Bool("acme", false, "Use the CLI's own ACME client over HTTP-01 (lego one-shot container) instead of certbot")
	f.Bool("dry-run", false, "With --acme: print the resolved stack, lineage and webroot; write nothing")
	f.String("nginx-container", "", "With --acme: nginx container to reload (default: found by compose labels)")
	f.Bool("agree-tos", false, "With --acme: accept the ACME CA's terms of service on first account registration")
}

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
func runSSLAddACME(cmd *cobra.Command, args []string) error {
	domain := strings.ToLower(strings.TrimSuffix(args[0], "."))
	if !acmeHostRE.MatchString(domain) {
		return e151(acmeRefuse("give one plain DNS name; a wildcard needs DNS-01 (`nself trust ssl setup --acme --wildcard`)",
			"%q cannot be issued over HTTP-01", args[0]))
	}
	r, err := prepareACME(cmd, false)
	if err != nil {
		return err
	}
	name := domainToFilesafe(domain)
	l := acme.Lineage{Name: name, Domains: []string{domain}, Challenge: acme.ChallengeHTTP, Targets: []string{"certificates/" + name}, KeyType: "ec256"}
	r.say("lineage %s: http-01 for %s\n  target:  %s\n  webroot: %s", name, domain, l.Targets[0], acme.WebrootDir(r.res.SSLDir))
	if r.dry {
		r.say("dry run: nothing written")
		return nil
	}
	if err := r.lock(); err != nil {
		return err
	}
	defer r.unlock()
	ctx := cmd.Context()
	hosts, _ := ssl.ServedHosts(r.res.NginxDir)
	if c := hosts[domain]; c != "" && !strings.Contains(c, "/"+name+"/") {
		return e151(acmeRefuse("use `nself trust ssl renew --acme` for the lineage that serves it", "%s is already served over HTTPS with %s", domain, c))
	}
	for _, x := range r.file.Lineages {
		if x.Name == name {
			return e151(acmeRefuse("renew it with `nself trust ssl renew --acme --force`", "lineage %s is already managed", name))
		}
	}
	env, _ := ssl.ServedEnv(r.res.Root, r.cfg.Env)
	if err := acmeProbeHTTP(ctx, r.res.SSLDir, httpProbeAddr(env), domain, nil); err != nil {
		return e151(acmeRefuse("run `nself build` and restart nginx so the challenge location is served",
			"the served nginx does not answer the HTTP-01 challenge location for %s: %v", domain, err))
	}
	tos, err := r.acceptTOS()
	if err != nil {
		return err
	}
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
		Reloader: reload, Fault: r.hooks.Fault, Exit: acmeD.exit})
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

// writeAndVerifyAdd writes the domain's conf (unless a served conf already uses
// the target), reloads nginx and checks the fingerprint it serves.
func writeAndVerifyAdd(ctx context.Context, r *acmeRun, l acme.Lineage, cert []byte, reload acme.NginxReloader) error {
	hosts, _ := ssl.ServedHosts(r.res.NginxDir)
	if !strings.Contains(hosts[l.Domains[0]], "/"+l.Name+"/") {
		upstream, _ := r.cmd.Flags().GetString("upstream")
		if err := writeCustomDomainConfServed(r.workdir, r.cfg.Nginx.FrontedBy, l.Domains[0], upstream); err != nil {
			return e151(acmeRefuse("the certificate is installed; fix the conf directory and re-run", "writing nginx conf: %v", err))
		}
	}
	if err := reload.Reload(ctx); err != nil {
		return e151(acmeRefuse("fix the nginx error above; the certificate is installed", "%v", err))
	}
	if err := r.verifier(l, cert)(ctx); err != nil {
		return e151(acmeRefuse("check that nginx serves the domain on its HTTPS port", "served certificate check failed: %v", err))
	}
	r.say("lineage %s: nginx reloaded, served certificate verified", l.Name)
	return nil
}
