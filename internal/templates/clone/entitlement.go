package clone

// entitlement.go — which required plugins of a clone template the operator
// may not use yet.
//
// Purpose: `nself init --template <x>-clone` scaffolds a project that needs
// several plugins (manifest.json required_plugins). Before any file is
// written, Check resolves each one against the plugin catalog and the current
// licence, so an unentitled operator is told up front instead of ending with a
// half-working project (P7-PLUG-25).
// Inputs: the template's plugin slugs, a catalog lookup and the licence
// checks. All are injected; this file does no network or disk I/O of its own.
// Outputs: a Report (licensed plugins the licence does not cover, plugins in
// no registry) and the E110 / E137 errors built from it.
// Constraints: a free plugin never triggers an entitlement call, so a
// free-only template makes no licence call. Each bundle is asked at most once
// per Check. A licensed plugin that is in no Bundle cannot be decided here (the
// licence server answers per Bundle, and a refusal would also block ɳSelf+
// holders): it is listed as Unchecked and left to the install-time check. An entitlement error counts as "not entitled" and
// is kept as the report's Reason: with no licence key that is the common case
// and must read as E110, not as an unrelated failure.

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
)

// PricingURL is where an operator buys the Bundles named in a report.
const PricingURL = "https://nself.org/pricing"

// v14Timeout bounds the whole check in v1.4 mode, where it is advisory: a slow
// or unreachable network must not hold up a scaffold that never depended on it.
const v14Timeout = 8 * time.Second

// licensedTier is the catalog tier value of a plugin that needs a licence.
const licensedTier = "licensed"

// PluginInfo is the part of a catalog entry Check reads.
type PluginInfo struct {
	Slug    string
	Tier    string   // "free" or "licensed"
	Bundles []string // Bundles that include the plugin
}

// Lookup finds a plugin by slug in the catalog (both tiers).
type Lookup func(slug string) (PluginInfo, bool)

// Entitled reports whether the operator's licence covers one Bundle. It is the
// hardened license.BundleEntitled in production: signed, bound, fail closed.
type Entitled func(ctx context.Context, bundle string) (bool, error)

// Missing is one licensed plugin the licence does not cover.
type Missing struct {
	Plugin  string
	Bundles []string
}

// Report is the outcome of Check.
type Report struct {
	Template       string
	MissingLicence []Missing
	Unknown        []string
	// Unchecked are licensed plugins in no Bundle: not refused here.
	Unchecked []string
	// Reason is the first entitlement error seen, "" when every check answered.
	Reason string
}

// OK reports whether the template may be scaffolded.
func (r Report) OK() bool { return len(r.MissingLicence) == 0 && len(r.Unknown) == 0 }

// Check resolves each plugin. It never returns an error for a missing
// licence: that is data in the Report.
func Check(ctx context.Context, template string, plugins []string, find Lookup, entitled Entitled) Report {
	rep := Report{Template: template}
	asked := map[string]bool{}
	note := func(ok bool, err error) bool {
		if err != nil && rep.Reason == "" {
			rep.Reason = err.Error()
		}
		return ok && err == nil
	}
	covered := func(bundle string) bool {
		if ok, seen := asked[bundle]; seen {
			return ok
		}
		asked[bundle] = note(entitled(ctx, bundle))
		return asked[bundle]
	}
	for _, slug := range plugins {
		info, found := find(slug)
		switch {
		case !found:
			rep.Unknown = append(rep.Unknown, slug)
		case info.Tier != licensedTier:
			// free: nothing to check
		default:
			if len(info.Bundles) == 0 {
				rep.Unchecked = append(rep.Unchecked, slug)
			} else if !anyCovered(info.Bundles, covered) {
				rep.MissingLicence = append(rep.MissingLicence, Missing{Plugin: slug, Bundles: info.Bundles})
			}
		}
	}
	return rep
}

func anyCovered(bundles []string, covered func(string) bool) bool {
	for _, b := range bundles {
		if covered(b) {
			return true
		}
	}
	return false
}

// Err is the error that refuses the scaffold, nil when the Report is OK.
// A plugin in no registry (E137, exit 1) outranks a missing licence (E110,
// exit 3): buying a Bundle does not make the template work.
func (r Report) Err() error {
	switch {
	case len(r.Unknown) > 0:
		return errs.Newf("E137", "template %q requires %s, which no plugin registry publishes%s",
			r.Template, strings.Join(r.Unknown, ", "), r.licenceTail())
	case len(r.MissingLicence) > 0:
		return errs.Newf("E110", "template %q needs licensed plugins your licence does not cover.\n%s",
			r.Template, r.Lines()).
			WithFix("Buy a Bundle that includes them at " + PricingURL + ", or run: nself license set <key>")
	}
	return nil
}

// licenceTail appends the licence problem to the E137 message when both exist.
func (r Report) licenceTail() string {
	if len(r.MissingLicence) == 0 {
		return ""
	}
	return ".\nAlso not covered by your licence:\n" + r.Lines()
}

// Lines lists each uncovered plugin with the Bundles that include it.
func (r Report) Lines() string {
	var b strings.Builder
	for _, m := range r.MissingLicence {
		fmt.Fprintf(&b, "  %s: %s\n", m.Plugin, bundleText(m.Bundles))
	}
	if r.Reason != "" {
		fmt.Fprintf(&b, "  licence check: %s\n", r.Reason)
	}
	return strings.TrimRight(b.String(), "\n")
}

func bundleText(bundles []string) string {
	return "included in the " + strings.Join(bundles, ", ") + " Bundle (or ɳSelf+)"
}

// Source supplies the catalog lookup and the entitlement check. Its error is
// the catalog's (E115 with no catalog and no cache, E116 for a broken one).
type Source func(ctx context.Context) (Lookup, Entitled, error)

// Enforce is the init gate. It runs before any file or directory is written;
// a template with no required plugins makes no catalog or licence call. From
// v1.5 an uncovered plugin (E110), a plugin in no registry (E137) or no
// catalog at all (E115) refuses. In v1.4 the E110/E137 report is printed to w
// as a warning and the scaffold proceeds; a catalog failure stays silent
// there, because the served catalog may not exist yet.
func Enforce(w io.Writer, template string, plugins []string, src Source) error {
	if len(plugins) == 0 {
		return nil
	}
	// compat.V15(P7-PLUG-25): clone template scaffolds without checking its plugins -> refused (E110, E137, or E115 with no catalog) before any file is written
	refuse := compat.V15()
	timeout := 60 * time.Second
	if !refuse {
		timeout = v14Timeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	find, entitled, err := src(ctx)
	report := false
	if err == nil {
		rep := Check(ctx, template, plugins, find, entitled)
		if err, report = rep.Err(), true; err == nil && len(rep.Unchecked) > 0 {
			_, _ = fmt.Fprintf(w, "note: %s are licensed but in no Bundle; their licence is checked when you install them\n", strings.Join(rep.Unchecked, ", "))
		}
	}
	if err == nil {
		return nil
	}
	if refuse {
		return err
	}
	if report && ctx.Err() == nil { // a timed-out check proves nothing: no false "not covered" warning
		_, _ = fmt.Fprintf(w, "warning: %s\n", err)
	}
	return nil
}
