package commands

// ssl_acme.go: `trust ssl setup|renew --acme`, CLI-owned ACME over DNS-01 (lego
// one-shot container, ADR 0026). Order: preflight (printed, no writes) -> lego
// -> atomic generation install -> nginx -t + reload -> served-fingerprint
// check, with rollback. Certbot paths are untouched; the flags are additive.
// acmeD holds the seams tests replace (docker, secrets, prober, exit).

import (
	"bufio"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/nginxtopo"
	"github.com/nself-org/cli/internal/secrets"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ssl/acme"
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
var acmeOnlyFlags = []string{"dry-run", "nginx-container", "agree-tos", "force", "quiet", "adopt-certbot", "dns-credential-file", "lineage"}

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
	s.String("challenge", "dns-01", "With --adopt-certbot: challenge for converted lineages (dns-01)")
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
}

// e151 turns an error into the CLI's E151 carrying its remediation.
func e151(err error) error {
	what, fix := "ACME: "+err.Error(), "re-run with --dry-run to see the plan"
	var ae *acme.Error
	if errors.As(err, &ae) {
		what, fix = ae.What, ae.Fix
	}
	e := errs.New("E151", what)
	e.Why, e.Fix, e.Wrapped = "", fix, err
	return e
}

func (r *acmeRun) say(format string, a ...any) { _, _ = fmt.Fprintf(r.out, format+"\n", a...) }

// rejectACMEFlags refuses ACME-only flags given without --acme.
func rejectACMEFlags(cmd *cobra.Command) error {
	for _, n := range acmeOnlyFlags {
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
	r = &acmeRun{cmd: cmd, out: cmd.OutOrStdout(), secEnv: "dev"}
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
	r.say("lineage %s: %s via %s\n  domains: %s", l.Name, l.Challenge, l.DNSProvider, strings.Join(l.Domains, " "))
	for _, t := range l.Targets {
		state, p := "absent", filepath.Join(r.res.SSLDir, filepath.FromSlash(t))
		if dest, err := os.Readlink(p); err == nil {
			state = "generation link -> " + dest
		} else if st, err := os.Stat(p); err == nil && st.IsDir() {
			state = "directory, becomes a generation link on install"
		}
		r.say("  target:  %s [%s]", t, state)
	}
}

// issueInstall issues l with lego and, unless staging, installs and verifies it.
func (r *acmeRun) issueInstall(ctx context.Context, l *acme.Lineage, tos bool) error {
	if err := acme.EnsureState(r.res.SSLDir); err != nil {
		return e151(err)
	}
	cert, key, err := acme.Issue(ctx, acme.IssueReq{SSLDir: r.res.SSLDir, Name: l.Name, Provider: l.DNSProvider,
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

// verifier returns the post-reload check that nginx serves cert for the
// lineage's first name (a wildcard is probed as check.<zone>).
func (r *acmeRun) verifier(l acme.Lineage, cert []byte) func(context.Context) error {
	b, _ := pem.Decode(cert)
	if b == nil {
		return func(context.Context) error { return errors.New("issued certificate is not PEM") }
	}
	sum, sni := sha256.Sum256(b.Bytes), strings.Replace(l.Domains[0], "*.", "check.", 1)
	env, _ := ssl.ServedEnv(r.res.Root, r.cfg.Env)
	addr := ssl.ServedAddr(env)
	return func(ctx context.Context) (err error) {
		// `nginx -s reload` returns before the old workers stop answering, so poll briefly.
		for i := 0; i < 20; i++ {
			got, perr := acmeD.probe(ctx, addr, sni, 10*time.Second)
			if err = perr; err == nil {
				if got.SHA256 == hex.EncodeToString(sum[:]) {
					return nil
				}
				err = fmt.Errorf("nginx at %s serves another certificate for %s than the one just installed", addr, sni)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(acmeVerifyWait):
			}
		}
		return err
	}
}

// acmeVerifyWait is the pause between served-fingerprint probes (tests shorten it).
var acmeVerifyWait = 500 * time.Millisecond

// askTTY prompts on a terminal and reports a yes; false when stdin is not a terminal.
func askTTY(prompt string) bool {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false
	}
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y")
}
