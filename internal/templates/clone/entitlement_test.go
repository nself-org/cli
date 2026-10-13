package clone

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

// fixtureCatalog is a synthetic catalog: the Discord plugins are licensed and
// sit in the family and clawde Bundles, "orphan" is licensed but in no Bundle,
// "oddtier" has a tier value the catalog never writes, the rest are free, and
// "recording" is in no registry.
func fixtureCatalog() Lookup {
	m := map[string]PluginInfo{
		"chat":       {Slug: "chat", Tier: "licensed", Bundles: []string{"family"}},
		"realtime":   {Slug: "realtime", Tier: "licensed", Bundles: []string{"clawde", "claw"}},
		"moderation": {Slug: "moderation", Tier: "licensed", Bundles: []string{"family"}},
		"auth":       {Slug: "auth", Tier: "licensed", Bundles: []string{"clawde"}},
		"orphan":     {Slug: "orphan", Tier: "licensed"},
		"oddtier":    {Slug: "oddtier", Tier: "pro", Bundles: []string{"family"}},
		"livekit":    {Slug: "livekit", Tier: "free"},
		"notify":     {Slug: "notify", Tier: "free"},
		"photos":     {Slug: "photos", Tier: "free"},
	}
	return func(slug string) (PluginInfo, bool) { p, ok := m[slug]; return p, ok }
}

// spy records every Bundle the check asked about.
type spy struct {
	asked []string
	grant map[string]bool
	err   error
}

func (s *spy) lic(_ context.Context, bundle string) (bool, error) {
	s.asked = append(s.asked, bundle)
	return s.grant[bundle], s.err
}

func required(t *testing.T, template string) []string {
	t.Helper()
	m, err := GetManifest(template)
	if err != nil {
		t.Fatal(err)
	}
	return m.RequiredPlugins
}

func TestTemplateEntitlementE110NoWrite(t *testing.T) {
	s := &spy{err: errors.New("no license key configured")}
	rep := Check(context.Background(), "discord-clone", required(t, "discord-clone"), fixtureCatalog(), s.lic)
	if len(rep.MissingLicence) != 4 || len(rep.Unknown) != 0 || len(rep.Unchecked) != 0 {
		t.Fatalf("report = %+v, want chat, realtime, moderation and auth uncovered", rep)
	}
	err := rep.Err()
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E110" {
		t.Fatalf("Err() = %v, want E110", err)
	}
	if got := errs.ExitCodeFor(err); got != 3 {
		t.Errorf("exit = %d, want 3", got)
	}
	for _, want := range []string{"chat: included in the family Bundle", "realtime: included in the clawde, claw Bundle", "moderation: included in the family", "auth: included in the clawde", "nself.org/pricing", "licence check: no license key configured"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message missing %q:\n%s", want, err)
		}
	}
	// Each Bundle is asked once however many plugins list it.
	if got := strings.Join(s.asked, ","); got != "family,clawde,claw" {
		t.Errorf("bundles asked = %s, want family, clawde and claw once each", got)
	}
	// Check does no I/O: the scaffold target stays untouched.
	dir := t.TempDir()
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("target not empty: %v", entries)
	}
}

func TestTemplateEntitlementLicensed(t *testing.T) {
	// One covered Bundle per plugin is enough; a plugin in two Bundles needs one.
	s := &spy{grant: map[string]bool{"family": true, "claw": true}}
	rep := Check(context.Background(), "discord-clone", required(t, "discord-clone"), fixtureCatalog(), s.lic)
	if len(rep.MissingLicence) != 1 || rep.MissingLicence[0].Plugin != "auth" {
		t.Fatalf("report = %+v, want only auth (clawde only) uncovered", rep)
	}
	all := &spy{grant: map[string]bool{"family": true, "clawde": true}}
	if rep := Check(context.Background(), "discord-clone", required(t, "discord-clone"), fixtureCatalog(), all.lic); !rep.OK() || rep.Err() != nil {
		t.Errorf("covered template refused: %+v", rep)
	}
}

// TestTemplateEntitlementUnchecked: a licensed plugin in no Bundle cannot be
// decided from the Bundle answer, so it is listed and not refused; an unknown
// tier value never reads as free.
func TestTemplateEntitlementUnchecked(t *testing.T) {
	s := &spy{}
	rep := Check(context.Background(), "x", []string{"orphan", "notify"}, fixtureCatalog(), s.lic)
	if !rep.OK() || len(rep.Unchecked) != 1 || rep.Unchecked[0] != "orphan" || len(s.asked) != 0 {
		t.Errorf("report = %+v asked %v, want orphan unchecked with no licence call", rep, s.asked)
	}
	odd := Check(context.Background(), "x", []string{"oddtier"}, fixtureCatalog(), s.lic)
	if len(odd.MissingLicence) != 1 {
		t.Errorf("a tier other than free must be checked as licensed: %+v", odd)
	}
}

func TestTemplateEntitlementE137Recording(t *testing.T) {
	s := &spy{}
	base := fixtureCatalog()
	find := func(slug string) (PluginInfo, bool) { // notify is licensed here, so E110 competes with E137
		if slug == "notify" {
			return PluginInfo{Slug: slug, Tier: "licensed", Bundles: []string{"media"}}, true
		}
		return base(slug)
	}
	rep := Check(context.Background(), "zoom-clone", required(t, "zoom-clone"), find, s.lic)
	if len(rep.Unknown) != 1 || rep.Unknown[0] != "recording" {
		t.Fatalf("Unknown = %v, want recording (and auth is licensed, not unknown)", rep.Unknown)
	}
	err := rep.Err()
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E137" {
		t.Fatalf("Err() = %v, want E137 ahead of E110", err)
	}
	if errs.ExitCodeFor(err) != 1 {
		t.Errorf("exit = %d, want 1", errs.ExitCodeFor(err))
	}
	if !strings.Contains(err.Error(), "recording") || !strings.Contains(err.Error(), "notify:") {
		t.Errorf("message must name recording and the uncovered notify:\n%s", err)
	}
}

func TestTemplateFreeNoLicenceCall(t *testing.T) {
	s := &spy{}
	rep := Check(context.Background(), "airbnb-clone", []string{"notify", "photos", "livekit"}, fixtureCatalog(), s.lic)
	if !rep.OK() || rep.Err() != nil {
		t.Fatalf("free template refused: %+v", rep)
	}
	if len(s.asked) != 0 {
		t.Errorf("free-only template made licence calls: %v", s.asked)
	}
}

func TestEnforceModes(t *testing.T) {
	src := func(s *spy) Source {
		return func(context.Context) (Lookup, Entitled, error) { return fixtureCatalog(), s.lic, nil }
	}
	down := func(context.Context) (Lookup, Entitled, error) {
		return nil, nil, errs.New("E115", "plugin catalog unavailable: test")
	}
	compattest.Both(t, func(t *testing.T) {
		v15 := compat.V15()
		var w bytes.Buffer
		err := Enforce(&w, "discord-clone", required(t, "discord-clone"), src(&spy{}))
		if v15 != (err != nil) || (!v15 && !strings.Contains(w.String(), "warning: [E110]")) {
			t.Errorf("v15=%v: err=%v warning=%q", v15, err, w.String())
		}
		w.Reset()
		err = Enforce(&w, "discord-clone", required(t, "discord-clone"), down)
		var ce *errs.CLIError
		if v15 && (!errors.As(err, &ce) || ce.Code != "E115") {
			t.Errorf("v1.5 with no catalog: %v, want E115", err)
		}
		if !v15 && (err != nil || w.Len() != 0) {
			t.Errorf("v1.4 with no catalog must stay silent: err=%v out=%q", err, w.String())
		}
		spyCalls := &spy{}
		if err := Enforce(&w, "x", nil, src(spyCalls)); err != nil || len(spyCalls.asked) != 0 {
			t.Errorf("template without plugins must not consult anything: %v", err)
		}
	})
}
