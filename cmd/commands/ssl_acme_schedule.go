package commands

// ssl_acme_schedule.go: `trust ssl renew --acme` and the renewal timer. The
// units set SECRETS_AGE_KEY_PATH and HOME explicitly: systemd's bare
// environment has neither, and the age key is how the DNS credential opens.

import (
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ssl/acme"
	"github.com/spf13/cobra"
	"path/filepath"
	"time"
)

// runSSLRenewACME renews the lineages that are due (30 days or fewer left), all
// of them with --force; --staging issues without installing (and is always due).
func runSSLRenewACME(cmd *cobra.Command, args []string) error {
	r, err := prepareACME(cmd, true)
	if err != nil {
		return err
	}
	if len(r.file.Lineages) == 0 {
		return e151(acmeRefuse("run `nself trust ssl setup --acme` or `--adopt-certbot` first", "no lineages are managed in %s", acme.StateDir(r.res.SSLDir)))
	}
	if err := r.lock(); err != nil {
		return err
	}
	defer r.unlock()
	force, _ := cmd.Flags().GetBool("force")
	var due []int
	matched := len(args) == 0
	for i, l := range r.file.Lineages {
		if len(args) > 0 && args[0] != l.Name {
			continue
		}
		if matched = true; len(l.Targets) == 0 || len(l.Domains) == 0 {
			return e151(acmeRefuse("repair or remove the lineage in lineages.json", "lineage %s has no domains or targets", l.Name))
		}
		c, cerr := ssl.ReadDiskCert(filepath.Join(r.res.SSLDir, filepath.FromSlash(l.Targets[0]), "fullchain.pem"))
		isDue := cerr != nil || acme.Due(c.NotAfter, time.Now(), force || r.staging)
		if !r.dry && !isDue {
			_ = acme.Repair(r.res.SSLDir, l.Targets)
		}
		r.printLineage(l)
		r.say("  due: %t", isDue)
		if isDue {
			due = append(due, i)
		}
	}
	if !matched {
		return e151(acmeRefuse("list them with `nself trust ssl renew --acme --dry-run`", "no lineage named %q", args[0]))
	}
	if len(due) == 0 || r.dry {
		return nil
	}
	tos, err := r.acceptTOS()
	if err != nil {
		return err
	}
	for _, i := range due {
		if err := r.issueInstall(cmd.Context(), &r.file.Lineages[i], tos); err != nil {
			return err
		}
		if tos = false; !r.staging {
			if err := acme.Save(r.res.SSLDir, r.file); err != nil {
				return e151(err)
			}
		}
	}
	return nil
}
