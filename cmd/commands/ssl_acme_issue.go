package commands

// ssl_acme_issue.go: `trust ssl setup --acme`: adopt (--adopt-certbot), install the
// timer for existing lineages (--install-cron), or issue BASE_DOMAIN's certbot names.

import (
	"context"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ssl/acme"
	"github.com/spf13/cobra"
)

// runSSLSetupACME implements `trust ssl setup --acme`: adopt (--adopt-certbot),
// install the timer for existing lineages (--install-cron), or issue the
// certbot name set for BASE_DOMAIN.
func runSSLSetupACME(cmd *cobra.Command, args []string) error {
	r, err := prepareACME(cmd, false)
	if err != nil {
		return err
	}
	fl, ctx := cmd.Flags(), cmd.Context()
	cron, _ := fl.GetBool("install-cron")
	provider, _ := fl.GetString("provider")
	wild, _ := fl.GetBool("wildcard")
	secs, known := acme.SecretNames(provider)
	switch base := r.cfg.BaseDomain; {
	case fl.Changed("adopt-certbot"):
		return r.adopt(ctx, args)
	case len(r.file.Lineages) > 0 && cron:
		return r.timer()
	case len(r.file.Lineages) > 0:
		return e151(acmeRefuse("renew with `"+acmeCommand("renew")+" --force`, or add lineages with --adopt-certbot",
			"lineages are already managed in %s", acme.StateDir(r.res.SSLDir)))
	case !known:
		return e151(acmeRefuse("use cloudflare, route53 or digitalocean", "DNS provider %q is not supported by the ACME client", provider))
	case base == "" || base == "localhost":
		return e151(acmeRefuse("set BASE_DOMAIN to a real domain", "BASE_DOMAIN must be a real domain"))
	default:
		names, name := []string{base, "api." + base, "auth." + base}, ssl.DomainToDirName(base)
		if wild {
			names = []string{"*." + base, base}
		}
		l := acme.Lineage{Name: name, Domains: names, Challenge: acme.ChallengeDNS, DNSProvider: provider,
			CredentialSecrets: secs, Targets: []string{"certificates/" + name}, KeyType: "ec256"}
		return r.issueNew(ctx, l, cron)
	}
}

// issueNew prints l, then (unless --dry-run) issues, installs and records it.
// Like adoption it refuses while a certbot hook copies into the ssl dir and
// disables the certbot timer before issuing (D-0411).
func (r *acmeRun) issueNew(ctx context.Context, l acme.Lineage, cron bool) error {
	r.printLineage(l)
	if !r.staging {
		if err := r.refuseCertbotHooks(certbotDir); err != nil {
			return err
		}
	}
	if r.dry {
		r.say("dry run: nothing written")
		if cron {
			return r.timer()
		}
		if r.staging {
			return nil
		}
		_, err := r.retireCertbot()
		return err
	}
	if err := r.lock(); err != nil {
		return err
	}
	defer r.unlock()
	tos, err := r.acceptTOS()
	if err != nil {
		return err
	}
	var retired bool // before the first write: no certbot copy lands on a new generation
	if !r.staging {
		if retired, err = r.retireAndRecheck(false, certbotDir); err != nil {
			return err
		}
	}
	if err := r.issueInstall(ctx, &l, tos); err != nil || r.staging {
		r.retiredHint(retired && err != nil)
		return err
	}
	r.file.Contact = r.res.Contact
	r.file.Lineages = append(r.file.Lineages, l)
	if err := acme.Save(r.res.SSLDir, r.file); err != nil {
		r.retiredHint(retired)
		return e151(err)
	}
	if cron {
		return r.timerAfter(retired)
	}
	r.say("renew with `%s`; add --install-cron for the timer", acmeCommand("renew"))
	return nil
}
