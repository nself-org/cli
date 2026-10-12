package commands

// Purpose: where `nself update --plan|--yes` gets its candidates and how it
// keeps the project consistent when an apply fails (P7-LIVE-10).
// Inputs: the project lock, the release lookup, the plugin registry, the CLI
// image lock, the bundle ledger.
// Outputs: a plan.Plan, a strict registry, a ledger refresh, a lock restore.
// Constraints: a source that cannot answer is an error, never an empty or stale
// answer (the registry here has no cache and no fallback host); a pin a source
// does not offer is a row with a reason. An image lock key is resolved to the
// variant of the pinned repository (postgres vs pgvector, the functions
// runtimes), and a plugin to the tier plugin.Update would install.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/httptimeout"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/upgrade/plan"
)

// gatherPlan resolves every candidate and diffs it against the lock.
func gatherPlan(ctx context.Context, d updateDeps, l lock.Lock, target string) (plan.Plan, error) {
	c := plan.Candidates{CLICurrent: d.runningVersion(), Images: map[string]plan.Candidate{}, Plugins: map[string]plan.Candidate{}}
	tag, _, err := d.latestRelease(target)
	switch {
	case err == nil:
		c.CLI = plan.Candidate{Value: tag}
	case strings.Contains(err.Error(), "status 404"):
		c.CLI = plan.Candidate{Reason: "no such release (HTTP 404)"}
	default:
		return plan.Plan{}, sourceFailure("the CLI release lookup", err)
	}
	for k, pin := range l.Pins() {
		if cand, ok := imageCandidate(k, pin); ok {
			c.Images[k] = cand
		}
	}
	if len(l.Plugins) > 0 {
		reg, err := d.registry(ctx)
		if err != nil {
			return plan.Plan{}, sourceFailure("the plugin registry", err)
		}
		for slug, pin := range l.Plugins {
			m, _, err := plugin.ResolvePluginTier(ctx, reg, slug, "", pin.Tier, d.entitled)
			switch {
			case errors.Is(err, errs.ErrPluginNotFound):
			case err != nil:
				c.Plugins[slug] = plan.Candidate{Reason: err.Error()}
			default:
				c.Plugins[slug] = plan.Candidate{Value: m.Version}
			}
		}
	}
	return plan.BuildPlan(l, c), nil
}

// repoNoTag is a reference's repository, canonical and without tag or digest.
func repoNoTag(ref string) string {
	r := compose.CanonicalRef(ref)
	if i := strings.LastIndex(r, ":"); i > strings.LastIndex(r, "/") {
		r = r[:i]
	}
	return r
}

// imageCandidate is what the CLI image lock offers for a lock key. A service
// that renders several lock entries (postgres: postgres or pgvector;
// functions: node, deno or python) offers the entry of the pinned repository,
// and nothing when the pin is another variant. ok is false for an unknown key.
func imageCandidate(key, pin string) (plan.Candidate, bool) {
	var entries []string
	for _, row := range compose.ImageInputs() {
		if row.Service == key {
			entries = append(entries, row.Entry)
		}
	}
	if len(entries) == 0 {
		entries = []string{key}
	}
	var refs []compose.Ref
	for _, e := range entries {
		if r, ok := compose.LockedRef(e); ok {
			refs = append(refs, r)
		}
	}
	switch {
	case len(refs) == 0:
		return plan.Candidate{}, false
	case len(refs) == 1:
		return plan.Candidate{Value: refs[0].String()}, true
	}
	for _, r := range refs {
		if repoNoTag(r.Repository) == repoNoTag(pin) {
			return plan.Candidate{Value: r.String()}, true
		}
	}
	return plan.Candidate{Reason: fmt.Sprintf("pinned repository %s is not one of the %d variants the CLI image lock offers for %s", repoNoTag(pin), len(refs), key)}, true
}

// strictRegistry fetches the plugin registry from its primary URL only: no
// cache, no stale copy and no fallback host for a failed fetch. A non-200 answer is an error that
// carries the status. The body is parsed by the plugin package through a
// private one-file cache directory that is removed afterwards.
func strictRegistry(ctx context.Context) (*plugin.Registry, error) {
	url := plugin.DefaultRegistryURL
	if env := os.Getenv("NSELF_PLUGIN_REGISTRY"); env != "" {
		url = env
	}
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://localhost") && !strings.HasPrefix(url, "http://127.0.0.1") {
		return nil, fmt.Errorf("registry URL %q is not https", url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "nself-cli")
	resp, err := httptimeout.WithTimeout(15 * time.Second).Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP GET %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "nself-registry-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := os.WriteFile(filepath.Join(tmp, "registry.json"), body, 0o600); err != nil {
		return nil, err
	}
	return plugin.FetchRegistry(ctx, url, tmp) // fresh file in tmp: parsed, no network
}

var sha256RE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// refreshBundleLedger brings the version, checksum and tier of an updated
// plugin's ledger record in step with what is installed, so the rebuilt lock
// does not keep the old pin. Ownership (installed_by, explicit) is untouched; a
// plugin without a record, or a project without a ledger, is left alone.
func refreshBundleLedger(pluginDir, slug string) error {
	store := bundle.LedgerStore(pluginDir)
	if _, err := os.Stat(store.Path()); err != nil {
		return nil
	}
	facts, err := bundle.InstalledFacts(pluginDir, []string{slug})
	if err != nil || len(facts) == 0 {
		return err
	}
	f := facts[0]
	return store.Update(func(l *ledger.Ledger) error {
		key := strings.ToLower(f.Name)
		rec, ok := l.Plugins[key]
		if !ok {
			return nil
		}
		if f.Version != "" {
			rec.Version = f.Version
		}
		rec.Checksum = strings.TrimPrefix(strings.ToLower(f.Checksum), "sha256:")
		if !sha256RE.MatchString(rec.Checksum) {
			rec.Checksum = ""
		}
		if f.Tier == ledger.TierFree || f.Tier == ledger.TierLicensed {
			rec.Tier = f.Tier
		}
		l.Plugins[key] = rec
		return nil
	})
}

// restoreLock writes the lock bytes taken before an apply back, atomically, and
// adds that to err, so a failed apply leaves the pins as they were and a retry
// sees the same diff and rebuilds.
func restoreLock(dir string, orig []byte, err error) error {
	path := lock.Path(dir)
	tmp := path + ".tmp"
	if werr := os.WriteFile(tmp, orig, 0o644); werr == nil {
		werr = os.Rename(tmp, path)
		if werr == nil {
			return fmt.Errorf("%w (the previous pins in %s were restored)", err, lock.FileName)
		}
		_ = os.Remove(tmp)
	}
	return fmt.Errorf("%w (and %s could not be restored: edit it back or run nself build --write-lock)", err, lock.FileName)
}
