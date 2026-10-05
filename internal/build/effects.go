package build

// effects.go — the Effects seam: every action the pipeline takes outside the
// project tree (hosts file, trust store, certificate generation, plugin dir
// rewrites, plugin installs, secret persistence, snapshots, the build lock)
// goes through Effects.Do (P7-LIVE-21).
//
// Purpose: write mode runs the action as before; plan mode records it and
// returns nil, so a plan can list what apply would do without doing it.
// Inputs: a kind (one of the Effect* constants), a target, a human detail and
// the action itself.
// Outputs: Recorded() lists the effects a plan-mode build would perform.
// Constraints: secrets-persist details carry key names only, never values.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/ssl"
)

// Effect kinds, exactly as EPIC D2 names them. orphan-remove is recorded by
// the reconcile layer (P7-LIVE-03), not here; plugin-remove is reserved for
// the plugin lifecycle step.
const (
	EffectHosts            = "hosts"
	EffectTrustStore       = "trust-store"
	EffectCertificates     = "certificates"
	EffectPluginFragment   = "plugin-fragment"
	EffectPluginInstall    = "plugin-install"
	EffectPluginRemove     = "plugin-remove"
	EffectSecretsPersist   = "secrets-persist"
	EffectNginxSitesBackup = "nginx-sites-backup"
	EffectBuildLock        = "build-lock"
)

// PlannedEffect is one host effect a plan-mode build would perform.
type PlannedEffect struct {
	Kind   string
	Target string
	Detail string
}

// Effects performs or records the build's host effects.
type Effects interface {
	// Do runs act in write mode; in plan mode it records the effect instead.
	Do(kind, target, detail string, act func() error) error
	// Planning reports whether Do records instead of acting.
	Planning() bool
	// Recorded returns the effects recorded so far, in order.
	Recorded() []PlannedEffect
}

// writeEffects acts: the behaviour of every build before the seam existed.
type writeEffects struct{}

func (writeEffects) Do(_, _, _ string, act func() error) error {
	if act == nil {
		return nil
	}
	return act()
}
func (writeEffects) Planning() bool            { return false }
func (writeEffects) Recorded() []PlannedEffect { return nil }

// planEffects records and never acts.
type planEffects struct{ list []PlannedEffect }

func (p *planEffects) Do(kind, target, detail string, _ func() error) error {
	p.list = append(p.list, PlannedEffect{Kind: kind, Target: target, Detail: detail})
	return nil
}
func (p *planEffects) Planning() bool            { return true }
func (p *planEffects) Recorded() []PlannedEffect { return append([]PlannedEffect(nil), p.list...) }

// planSSL is the plan-mode stand-in for ssl.Generator.GenerateWithResult. It
// reads the disk (certificate presence and expiry) and PATH (is mkcert
// installed) but never runs mkcert or openssl, never touches the trust store
// or the hosts file: it records what a write-mode build would do and returns
// the domains whose certificate pair the build would create, so nginx can
// keep its TLS blocks (nginx.Generator.WithAssumedCerts).
func planSSL(fx Effects, cfg *config.Config, sslDir string, explicitHosts bool) (*ssl.GenerateResult, []string) {
	switch cfg.SSLMode {
	case "letsencrypt", "custom", "none":
		return &ssl.GenerateResult{Count: 0}, nil
	}
	gen := ssl.NewGenerator(cfg).WithExplicitHosts(explicitHosts)
	domains := gen.CollectDomains()
	dirName := ssl.DomainToDirName(cfg.BaseDomain)
	if dirName == "" {
		dirName = "localhost"
	}
	certDir := filepath.Join(sslDir, "certificates", dirName)
	if !validCertPair(certDir) {
		detail := "generate a certificate for " + strings.Join(domains, ", ")
		if validCertPair(sslDir) {
			detail = "copy the certificate from `nself trust` for " + strings.Join(domains, ", ")
		}
		_ = fx.Do(EffectCertificates, certDir, detail, nil)
	}
	if _, err := exec.LookPath("mkcert"); err == nil {
		_ = fx.Do(EffectTrustStore, "mkcert CA", "install the mkcert CA when it is not yet trusted", nil)
	}
	if entries := ssl.PlanHostsEntries(cfg.Env, cfg.BaseDomain, explicitHosts, domains); len(entries) > 0 {
		_ = fx.Do(EffectHosts, "/etc/hosts", "ensure entries: "+strings.Join(entries, ", "), nil)
	}
	return &ssl.GenerateResult{Count: 1}, domains
}

// validCertPair reports whether dir holds a certificate pair with 30 or more
// days left, the same test the generator uses to skip regeneration.
func validCertPair(dir string) bool {
	full := filepath.Join(dir, "fullchain.pem")
	for _, f := range []string{full, filepath.Join(dir, "privkey.pem")} {
		if _, err := os.Stat(f); err != nil {
			return false
		}
	}
	days, err := ssl.CheckCertExpiry(full)
	return err == nil && days >= 30
}
