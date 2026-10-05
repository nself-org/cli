package commands

// ssl_acme.go: `trust ssl setup|renew --acme`, CLI-owned ACME over DNS-01 (lego
// one-shot container, ADR 0026). Order: preflight (printed, no writes) -> lego
// -> atomic generation install -> nginx -t + reload -> served-fingerprint
// check, with rollback. Certbot paths are untouched; the flags are additive.
// acmeD holds the seams tests replace (docker, secrets, prober, exit).

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/nginxtopo"
	"github.com/nself-org/cli/internal/secrets"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ssl/acme"
	"github.com/spf13/cobra"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// acmeInput carries optional preflight seams (nil = real docker and PATH).
var acmeInput acme.Input

// acmeD holds the outside-world seams of the ACME commands.
var acmeD = struct {
	run    func(context.Context, docker.RunSpec) (string, string, error)
	exec   func(context.Context, string, []string) (string, string, error)
	probe  func(context.Context, string, string, time.Duration) (ssl.ServedCert, error)
	secGet func(root, env, key string) (string, error)
	secSet func(root, env, key, value string) error
	timer  func(workdir, keyPath string) error
	ask    func(prompt string) bool
	exit   func(int)
}{docker.RunOneShot, docker.ExecCapture, ssl.ProbeServed, secrets.Get, secrets.Set, installACMETimer, askTTY, os.Exit}

// acmeOnlyFlags mean nothing without --acme and are refused without it.
var acmeOnlyFlags = []string{"dry-run", "nginx-container", "agree-tos", "force", "quiet", "adopt-certbot", "dns-credential-file", "challenge", "lineage"}

func init() {
	for _, c := range []*cobra.Command{sslSetupCmd, sslRenewCmd} {
		f := c.Flags()
		f.Bool("acme", false, "Use the CLI's own ACME client (DNS-01, lego one-shot container) instead of certbot")
		f.Bool("dry-run", false, "With --acme: print the resolved stack, lineages and targets; write nothing")
		f.String("nginx-container", "", "With --acme: nginx container to reload (default: found by compose labels)")
		f.Bool("agree-tos", false, "With --acme: accept the ACME CA's terms of service on first account registration")
	}
	r, s := sslRenewCmd.Flags(), sslSetupCmd.Flags()
	r.String("email", "", "With --acme: ACME contact (default: stored contact, ACME_EMAIL, ADMIN_EMAIL)")
	r.Bool("staging", false, "With --acme: issue from the staging CA into .acme/staging; installs nothing")
	r.Bool("force", false, "With --acme: renew every lineage, not only those with 30 days or fewer left")
	r.Bool("quiet", false, "With --acme: suppress non-error output (for the renewal timer)")
	s.String("adopt-certbot", "", "With --acme: adopt certbot lineages from [dir] (default /etc/letsencrypt); never issues")
	s.Lookup("adopt-certbot").NoOptDefVal = "/etc/letsencrypt"
	s.String("dns-credential-file", "", "With --adopt-certbot: certbot DNS-plugin INI; its credential is stored by name")
	s.String("challenge", "dns-01", "With --adopt-certbot: challenge for converted lineages: dns-01 (a webroot lineage with no credential becomes http-01 when the served nginx answers the challenge location)")
	s.String("lineage", "", "With --adopt-certbot: adopt only this certbot lineage")
}

// acmeRun is one `--acme` invocation after preflight.
type acmeRun struct {
	cmd                 *cobra.Command
	out                 io.Writer
	workdir, secEnv     string
	cfg                 *config.Config
	res                 *acme.Resolution
	hooks               acme.Hooks
	file                acme.File
	dry, staging, agree bool
	unlock              func() // releases the run lock (a no-op under --dry-run)
}

func (r *acmeRun) say(format string, a ...any) { _, _ = fmt.Fprintf(r.out, format+"\n", a...) }

// rejectACMEFlags refuses ACME-only flags given without --acme.
func rejectACMEFlags(cmd *cobra.Command) error {
	names := acmeOnlyFlags
	if cmd.Name() == "renew" { // new on renew only: setup's own --staging and --email predate --acme
		names = append(append([]string{}, names...), "staging", "email")
	}
	for _, n := range names {
		if f := cmd.Flags().Lookup(n); f != nil && f.Changed {
			return e151(acmeRefuse("add --acme", "--%s only works with --acme", n))
		}
	}
	return nil
}

// prepareACME loads the project and runs and prints the preflight.
func prepareACME(cmd *cobra.Command, renew bool) (r *acmeRun, err error) {
	fl := cmd.Flags()
	str := func(n string) string { v, _ := fl.GetString(n); return v }
	r = &acmeRun{cmd: cmd, out: cmd.OutOrStdout(), secEnv: "dev", unlock: func() {}}
	r.dry, _ = fl.GetBool("dry-run")
	r.staging, _ = fl.GetBool("staging")
	r.agree, _ = fl.GetBool("agree-tos")
	if q, _ := fl.GetBool("quiet"); q {
		r.out = io.Discard
		slog.SetLogLoggerLevel(slog.LevelWarn) // the config loader's INFO lines are output too
	}
	cwd, _ := os.Getwd()
	if r.workdir, err = config.FindNSelfRoot(cwd); err != nil {
		return nil, e151(acmeRefuse("run from an nself project (`nself init`)", "no nself project found"))
	}
	if r.cfg, err = config.Load(r.workdir); err != nil {
		return nil, e151(acmeRefuse("fix the project's .env files", "loading config: %v", err))
	}
	r.secEnv = cmp.Or(r.cfg.Env, r.secEnv)
	if r.hooks, err = acme.HooksFromEnv(os.Getenv); err != nil {
		return nil, e151(err)
	}
	if d, derr := nginxtopo.ServedSSLDir(r.workdir, r.cfg.Nginx.FrontedBy); derr == nil {
		r.file, _ = acme.Load(d)
	}
	env, _ := ssl.ServedEnv(r.workdir, r.cfg.Env)
	stored := ""
	if renew {
		stored = r.file.Contact
	}
	in := acmeInput
	in.NoSecrets = cmd.Name() == "add" || renew && httpOnly(r.file) // HTTP-01 handles no credential: no age, no key
	in.ProjectDir, in.FrontedBy, in.ProjectName, in.NginxContainer = r.workdir, r.cfg.Nginx.FrontedBy, r.cfg.ProjectName, str("nginx-container")
	in.Contact = cmp.Or(str("email"), stored, env["ACME_EMAIL"], os.Getenv("ACME_EMAIL"), r.cfg.AdminEmail)
	home, _ := os.UserHomeDir()
	in.AgeKeyPath = cmp.Or(os.Getenv("SECRETS_AGE_KEY_PATH"), filepath.Join(home, ".config", "nself", "age-key.txt"))
	if r.res, err = acme.Resolve(cmd.Context(), in); err != nil {
		return nil, e151(err)
	}
	r.res.Print(r.out)
	return r, nil
}

// lock takes the run lock right before the first write (refusals and dry runs
// stay write-free) and re-reads lineages.json under it. Callers defer r.unlock().
func (r *acmeRun) lock() (err error) {
	if !r.dry {
		if r.unlock, err = acme.Lock(r.res.SSLDir, acmeLockWait); err != nil {
			return e151(err)
		}
		r.file, _ = acme.Load(r.res.SSLDir)
	}
	return nil
}

// acceptTOS reports whether lego needs --accept-tos (first account
// registration) and refuses when the terms were neither agreed nor confirmed.
func (r *acmeRun) acceptTOS() (bool, error) {
	if !acme.NeedsTOS(r.res.SSLDir, r.staging, r.res.Contact) {
		return false, nil
	}
	if r.agree || acmeD.ask("Accept the ACME CA's terms of service for "+r.res.Contact+"?") {
		return true, nil
	}
	return false, e151(acmeRefuse("pass --agree-tos, or run in a terminal and confirm", "the ACME CA's terms of service were not accepted"))
}

// printLineage shows a lineage and the state of each target.
func (r *acmeRun) printLineage(l acme.Lineage) {
	via := l.Challenge
	if l.DNSProvider != "" {
		via += " via " + l.DNSProvider
	}
	r.say("lineage %s: %s\n  domains: %s", l.Name, via, strings.Join(l.Domains, " "))
	for _, t := range l.Targets {
		state, p := "absent", filepath.Join(r.res.SSLDir, filepath.FromSlash(t))
		if dest, err := os.Readlink(p); err == nil {
			state = "generation link -> " + dest
		} else if _, err := os.Stat(p); err == nil {
			state = "directory, becomes a generation link on install"
		}
		r.say("  target:  %s [%s]", t, state)
	}
}

// probeHTTP checks that the served nginx answers the HTTP-01 challenge location
// for host, so a missing location fails here with a remediation, not at the CA.
func (r *acmeRun) probeHTTP(ctx context.Context, host string) error {
	env, _ := ssl.ServedEnv(r.res.Root, r.cfg.Env)
	if err := acmeProbeHTTP(ctx, r.res.SSLDir, httpProbeAddr(env), host, nil); err != nil {
		return e151(acmeRefuse("run `nself build` and restart nginx so the challenge location is served",
			"the served nginx does not answer the HTTP-01 challenge location for %s: %v", host, err))
	}
	return nil
}

// issueInstall issues l with lego (DNS-01, or HTTP-01 for an http-01 lineage) and, unless staging, installs and verifies it.
func (r *acmeRun) issueInstall(ctx context.Context, l *acme.Lineage, tos bool) error {
	issue := acme.Issue
	if l.Challenge == acme.ChallengeHTTP { // no provider, no secret: the CA fetches a token from the served nginx
		issue = acme.IssueHTTP01
		for _, d := range l.Domains {
			if err := r.probeHTTP(ctx, d); err != nil {
				return err
			}
		}
	}
	cert, key, err := issue(ctx, acme.IssueReq{SSLDir: r.res.SSLDir, Name: l.Name, Provider: l.DNSProvider,
		Contact: r.res.Contact, Domains: l.Domains, Staging: r.staging, AcceptTOS: tos, Hooks: r.hooks, Run: acmeD.run,
		Secret: func(n string) (string, error) { return acmeD.secGet(r.workdir, r.secEnv, n) }})
	if err != nil {
		return e151(err)
	}
	if r.staging {
		r.say("lineage %s: staging certificate written to %s; nothing installed", l.Name, cert)
		return nil
	}
	c, e1 := os.ReadFile(cert) //nolint:gosec // path built from the served ssl dir
	k, e2 := os.ReadFile(key)  //nolint:gosec // path built from the served ssl dir
	if err := errors.Join(e1, e2); err != nil {
		return e151(err)
	}
	if err := r.install(ctx, *l, c, k); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	l.LastIssued = &now
	return nil
}

// install switches l's targets to the certificate, reloads nginx and verifies
// the fingerprint nginx serves; a failure restores the previous generation.
func (r *acmeRun) install(ctx context.Context, l acme.Lineage, cert, key []byte) error {
	gens, err := acme.Install(ctx, acme.InstallReq{SSLDir: r.res.SSLDir, Targets: l.Targets, Cert: cert, Key: key,
		Reloader: acme.NginxReloader{Container: r.res.Container, Exec: acmeD.exec}, Verify: r.verifier(l, cert),
		Fault: r.hooks.Fault, Exit: acmeD.exit})
	if err != nil {
		return e151(err)
	}
	for _, t := range l.Targets {
		r.say("lineage %s: installed %s as generation %d, nginx reloaded, served certificate verified", l.Name, t, gens[t])
	}
	return nil
}

// verifier returns the post-reload check that nginx serves cert on every target.
func (r *acmeRun) verifier(l acme.Lineage, cert []byte) func(context.Context) error {
	b, _ := pem.Decode(cert)
	if b == nil {
		return func(context.Context) error { return errors.New("issued certificate is not PEM") }
	}
	sum := sha256.Sum256(b.Bytes)
	env, _ := ssl.ServedEnv(r.res.Root, r.cfg.Env)
	addr, snis := ssl.ServedAddr(env), r.probeHosts(l)
	return func(ctx context.Context) error {
		for _, sni := range snis {
			if err := pollServed(ctx, addr, sni, hex.EncodeToString(sum[:])); err != nil {
				return err
			}
		}
		return nil
	}
}

// probeHosts picks one served host per target (the first by name whose conf uses
// that target), else the lineage's first name; a wildcard is probed as check.<zone>.
func (r *acmeRun) probeHosts(l acme.Lineage) (out []string) {
	hosts, _ := ssl.ServedHosts(r.res.NginxDir)
	for _, t := range l.Targets {
		pick := ""
		for h, c := range hosts {
			if strings.Contains(c, "/"+t+"/") && (pick == "" || h < pick) {
				pick = h
			}
		}
		out = append(out, cmp.Or(pick, strings.Replace(l.Domains[0], "*.", "check.", 1)))
	}
	return out
}

// acmeLockWait is how long a run waits for another --acme run (tests shorten it).
var acmeLockWait = 10 * time.Second

// acmeVerifyWait is the pause between served-fingerprint probes (tests shorten it).
var acmeVerifyWait = 500 * time.Millisecond

// httpOnly reports whether f has lineages and every one is http-01.
func httpOnly(f acme.File) bool {
	for _, l := range f.Lineages {
		if l.Challenge != acme.ChallengeHTTP {
			return false
		}
	}
	return len(f.Lineages) > 0
}
